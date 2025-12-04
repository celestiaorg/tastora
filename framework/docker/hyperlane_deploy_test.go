package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"github.com/celestiaorg/tastora/framework/types"
	commonpkg "github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// TestHyperlaneDeployer_Bootstrap starts celestia-app, a DA bridge, and a reth node,
// then initializes and optionally runs the Hyperlane deployer steps.
func TestHyperlaneDeployer_Bootstrap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	testCfg := setupDockerTest(t)
	ctx, cancel := context.WithTimeout(testCfg.Ctx, 5*time.Minute)
	defer cancel()

	// 1) Start a Celestia App chain (needed for DA network)
	chain, err := testCfg.ChainBuilder.Build(ctx)
	require.NoError(t, err)
	require.NoError(t, chain.Start(ctx))
	t.Cleanup(func() { _ = chain.Remove(context.Background()) })

	// 2) Start only the DA bridge node (minimal for this test) with custom network settings
	chainID := chain.GetChainID()
	chainNI, err := chain.GetNodes()[0].GetNetworkInfo(ctx)
	require.NoError(t, err)
	coreHost := chainNI.Internal.Hostname
	genesisHash, err := getGenesisHash(ctx, chain)
	require.NoError(t, err)

	danet, err := testCfg.DANetworkBuilder.WithChainID(chainID).Build(ctx)
	require.NoError(t, err)
	bridge := danet.GetBridgeNodes()[0]
	require.NoError(t, bridge.Start(ctx,
		da.WithChainID(chainID),
		da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", coreHost, "--rpc.addr", "0.0.0.0"),
		da.WithEnvironmentVariables(map[string]string{
			"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
			"P2P_NETWORK":     chainID,
		}),
	))
	t.Cleanup(func() { _ = bridge.Remove(context.Background()) })

	// compute DA address for evm-single (use bridge internal RPC)
	bridgeNI, err := bridge.GetNetworkInfo(ctx)
	require.NoError(t, err)
	daAddress := fmt.Sprintf("http://%s:%s", bridgeNI.Internal.IP, bridgeNI.Internal.Ports.RPC)

	// 3) Start a single reth node
	rnode, err := testCfg.RethBuilder.Build(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rnode.Remove(context.Background()) })
	require.NoError(t, rnode.Start(ctx))

	// Wait until reth RPC becomes responsive
	require.Eventually(t, func() bool {
		ec, err := rnode.GetEthClient(ctx)
		if err != nil {
			return false
		}
		_, err = ec.BlockNumber(ctx)
		return err == nil
	}, 45*time.Second, 1*time.Second)

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
	}, 45*time.Second, 1*time.Second)

	// Build URLs for evm-single
	rethNetworkInfo, err := rnode.GetNetworkInfo(ctx)
	require.NoError(t, err)
	evmEthURL := fmt.Sprintf("http://%s:%s", rethNetworkInfo.Internal.Hostname, rethNetworkInfo.Internal.Ports.RPC)
	evmEngineURL := fmt.Sprintf("http://%s:%s", rethNetworkInfo.Internal.Hostname, rethNetworkInfo.Internal.Ports.Engine)
	// Fetch genesis block hash from reth
	evmGenesisHash, err := rnode.GenesisHash(ctx)
	require.NoError(t, err)

	// Start evm-single linked to reth and DA
	ebuilder := testCfg.EVMSingleChainBuilder.
		WithNode(
			evmsingle.NewNodeConfigBuilder().
				WithEVMEngineURL(evmEngineURL).
				WithEVMETHURL(evmEthURL).
				WithEVMJWTSecret(rnode.JWTSecretHex()).
				WithEVMSignerPassphrase("secret").
				WithEVMBlockTime("1s").
				WithEVMGenesisHash(evmGenesisHash).
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

	// Health check via evnode health endpoint
	enodes := echain.Nodes()
	require.Len(t, enodes, 1)
	eni, err := enodes[0].GetNetworkInfo(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, eni.External.Ports.RPC)
	healthURL := fmt.Sprintf("http://0.0.0.0:%s/health/ready", eni.External.Ports.RPC)
	require.Eventually(t, func() bool {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode == http.StatusOK
	}, 60*time.Second, 2*time.Second)

	// 4) Initialize the Hyperlane deployer with the reth node as a provider
	// Select a hyperlane image. Allow override via HYPERLANE_IMAGE (format repo:tag)
	hlImage := hyperlane.DefaultHyperlaneImage()

	d, err := hyperlane.NewHyperlane(
		ctx,
		hyperlane.Config{
			Logger:          testCfg.Logger,
			DockerClient:    testCfg.DockerClient,
			DockerNetworkID: testCfg.NetworkID,
			HyperlaneImage:  hlImage,
		},
		testCfg.TestName,
		[]hyperlane.ChainConfigProvider{rnode, chain},
	)
	require.NoError(t, err)

	// 5) Validate that init wrote basic config files
	// Read relayer-config.json and ensure it contains the reth chain
	relayerBytes, err := d.ReadFile(ctx, "relayer-config.json")
	require.NoError(t, err)
	var relayerCfg hyperlane.RelayerConfig
	require.NoError(t, json.Unmarshal(relayerBytes, &relayerCfg))
	require.NotEmpty(t, relayerCfg.Chains["rethlocal"]) // name from reth provider
	// also expect the cosmos chain entry by its configured name
	require.NotEmpty(t, relayerCfg.Chains[chain.Config.Name])

	// Registry metadata for reth should exist
	_, err = d.ReadFile(ctx, filepath.Join("registry", "chains", "rethlocal", "metadata.yaml"))
	require.NoError(t, err)
	// Registry paths for reth and cosmos exist
	_, err = d.ReadFile(ctx, filepath.Join("registry", "chains", "rethlocal", "addresses.yaml"))
	require.NoError(t, err)
	_, err = d.ReadFile(ctx, filepath.Join("registry", "chains", chain.Config.Name, "metadata.yaml"))
	require.NoError(t, err)

	// 6) Execute the Hyperlane deploy steps (must succeed)
	require.NoError(t, d.Deploy(ctx))

	// 7) Verify core contracts are deployed on reth by reading registry addresses
	onDiskSchema, err := d.GetOnDiskSchema(ctx)
	require.NoError(t, err)
	addrs := onDiskSchema.Registry.Chains["rethlocal"].Addresses

	// pick a few critical contracts to verify
	critical := map[string]string{
		"Mailbox":        addrs.Mailbox,
		"ProxyAdmin":     addrs.ProxyAdmin,
		"MerkleTreeHook": addrs.MerkleTreeHook,
	}
	for name, hex := range critical {
		require.NotEmpty(t, hex, "%s address should be present", name)
	}

	// Query code at those addresses via reth RPC to ensure contracts exist
	ec, err := rnode.GetEthClient(ctx)
	require.NoError(t, err)
	for name, hex := range critical {
		code, err := ec.CodeAt(ctx, commonpkg.HexToAddress(hex), nil)
		require.NoErrorf(t, err, "failed to fetch code for %s", name)
		require.Greaterf(t, len(code), 0, "%s should have non-empty code", name)
	}
}
