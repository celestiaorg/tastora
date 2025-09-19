package docker

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	sdkacc "github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
)

// TestEvmSingle_WithReth starts a single reth node and an ev-node-evm-single
// configured to talk to it via Engine/RPC, then checks basic liveness.
func TestEvmSingle_WithReth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	testCfg := setupDockerTest(t)

	// 1) Start a Celestia App chain (required for DA network)
	chain, err := testCfg.ChainBuilder.Build(testCfg.Ctx)
	require.NoError(t, err)
	require.NoError(t, chain.Start(testCfg.Ctx))
	chainID := chain.GetChainID()
	chainNI, err := chain.GetNodes()[0].GetNetworkInfo(testCfg.Ctx)
	require.NoError(t, err)
	coreHost := chainNI.Internal.Hostname
	genesisHash, err := getGenesisHash(testCfg.Ctx, chain)
	require.NoError(t, err)

	// 2) Start Celestia DA network (bridge, full, light)
	danet, err := testCfg.DANetworkBuilder.
		WithChainID(chainID).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(testCfg.Ctx)
	defer cancel()

	t.Cleanup(func() {
		_ = chain.Remove(ctx)
		nodes := danet.GetNodes()
		for _, n := range nodes {
			_ = n.Remove(ctx)
		}
	})

	// Start DA Bridge
	bridge := danet.GetBridgeNodes()[0]
	require.NoError(t, bridge.Start(ctx,
		da.WithChainID(chainID),
		da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", coreHost, "--rpc.addr", "0.0.0.0"),
		da.WithEnvironmentVariables(map[string]string{
			"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
			"P2P_NETWORK":     chainID,
		}),
	))

	// Fund DA bridge wallet on the Celestia app chain so DA submissions succeed
	daWallet, err := bridge.GetWallet()
	require.NoError(t, err)
	fromAddress, err := sdkacc.AddressFromWallet(chain.GetFaucetWallet())
	require.NoError(t, err)
	toAddress, err := sdk.AccAddressFromBech32(daWallet.GetFormattedAddress())
	require.NoError(t, err)
	bankSend := banktypes.NewMsgSend(fromAddress, toAddress, sdk.NewCoins(sdk.NewCoin("utia", sdkmath.NewInt(100_000_000_00))))
	_, err = chain.BroadcastMessages(testCfg.Ctx, chain.GetFaucetWallet(), bankSend)
	require.NoError(t, err)

	// Start DA Full (peer to bridge)
	full := danet.GetFullNodes()[0]
	p2pInfo, err := bridge.GetP2PInfo(ctx)
	require.NoError(t, err)
	p2pAddr, err := p2pInfo.GetP2PAddress()
	require.NoError(t, err)
	require.NoError(t, full.Start(ctx,
		da.WithChainID(chainID),
		da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", coreHost, "--rpc.addr", "0.0.0.0"),
		da.WithEnvironmentVariables(map[string]string{
			"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, p2pAddr),
			"P2P_NETWORK":     chainID,
		}),
	))

	// Start DA Light (peer to full)
	light := danet.GetLightNodes()[0]
	p2pInfoFull, err := full.GetP2PInfo(ctx)
	require.NoError(t, err)
	p2pAddrFull, err := p2pInfoFull.GetP2PAddress()
	require.NoError(t, err)
	require.NoError(t, light.Start(ctx,
		da.WithChainID(chainID),
		da.WithAdditionalStartArguments("--p2p.network", chainID, "--rpc.addr", "0.0.0.0"),
		da.WithEnvironmentVariables(map[string]string{
			"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, p2pAddrFull),
			"P2P_NETWORK":     chainID,
		}),
	))

	// compute DA address for evm-single (use bridge internal RPC)
	bridgeNI, err := bridge.GetNetworkInfo(ctx)
	require.NoError(t, err)
	daAddress := fmt.Sprintf("http://%s:%s", bridgeNI.Internal.IP, bridgeNI.Internal.Ports.RPC)

	// 3) Build a single reth node using the pre-configured builder
	rnode, err := testCfg.RethBuilder.Build(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = rnode.Remove(ctx)
	})

	require.NoError(t, rnode.Start(ctx))

	// Wait until reth JSON-RPC is ready using ethclient (fetch latest block number)
	require.Eventually(t, func() bool {
		ec, err := rnode.GetEthClient(ctx)
		if err != nil {
			return false
		}
		if _, err := ec.BlockNumber(ctx); err != nil {
			return false
		}
		return true
	}, 45*time.Second, 1*time.Second, "reth JSON-RPC (ethclient) did not become ready")

	// Fetch genesis block hash from reth via helper
	genesisHash, err = rnode.GenesisHash(ctx)
	require.NoError(t, err)

	// Ensure Engine (authrpc) TCP port is open on host mapping before starting evm-single
	rni2, err := rnode.GetNetworkInfo(ctx)
	require.NoError(t, err)
	engineHost := fmt.Sprintf("0.0.0.0:%s", rni2.External.Ports.Engine)
	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", engineHost, 2*time.Second)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}, 45*time.Second, 1*time.Second, "reth Engine port did not open")

	rethNetworkInfo, err := rnode.GetNetworkInfo(ctx)
	require.NoError(t, err)
	evmEthURL := fmt.Sprintf("http://%s:%s", rethNetworkInfo.Internal.Hostname, rethNetworkInfo.Internal.Ports.RPC)
	evmEngineURL := fmt.Sprintf("http://%s:%s", rethNetworkInfo.Internal.Hostname, rethNetworkInfo.Internal.Ports.Engine)

	// 4) Build an evm-single chain linked to reth and DA (explicit config)
	ebuilder := testCfg.EVMSingleChainBuilder.
		WithNode(
			evmsingle.NewNodeConfigBuilder().
				WithEVMEngineURL(evmEngineURL).
				WithEVMETHURL(evmEthURL).
				WithEVMJWTSecret(rnode.JWTSecretHex()).
				WithEVMSignerPassphrase("secret").
				WithEVMBlockTime("1s").
				WithEVMGenesisHash(genesisHash).
				WithDAAddress(daAddress).
				Build(),
		)

	echain, err := ebuilder.Build(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = echain.Stop(ctx)
		_ = echain.Remove(ctx)
	})

	require.NoError(t, echain.Start(ctx))

	enodes := echain.Nodes()
	require.Len(t, enodes, 1)

	eni, err := enodes[0].GetNetworkInfo(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, eni.External.Ports.RPC)

	// Health check via evnode health endpoint
	healthURL := fmt.Sprintf("http://0.0.0.0:%s/evnode.v1.HealthService/Livez", eni.External.Ports.RPC)
	require.Eventually(t, func() bool {
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, healthURL, bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode == http.StatusOK
	}, 60*time.Second, 2*time.Second, "evm-single did not become healthy")
}
