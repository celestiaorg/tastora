package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	warptypes "github.com/bcp-innovations/hyperlane-cosmos/x/warp/types"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	"github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	"github.com/celestiaorg/tastora/framework/docker/evstack/reth"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"github.com/celestiaorg/tastora/framework/testutil/deploy"
	"github.com/celestiaorg/tastora/framework/testutil/evm"
	query "github.com/celestiaorg/tastora/framework/testutil/query"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	reth0ChainName = "reth0"
	reth1ChainName = "reth1"
	reth0ChainID   = 1234
	reth1ChainID   = 1235
)

func TestHyperlaneDeployer_MultiEVMChains(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	testCfg := setupDockerTest(t)
	ctx, cancel := context.WithTimeout(testCfg.Ctx, 12*time.Minute)
	defer cancel()

	testCfg.ChainBuilder = testCfg.ChainBuilder.WithImage(container.NewImage("ghcr.io/celestiaorg/celestia-app-standalone", "feature-zk-execution-ism", "10001:10001"))

	celestia, daNetwork, err := deploy.CelestiaWithDA(ctx, testCfg.ChainBuilder, testCfg.DANetworkBuilder)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = celestia.Stop(ctx)
	})

	bridge := daNetwork.GetBridgeNodes()[0]
	bridgeNodeNetworkInfo, err := bridge.GetNetworkInfo(ctx)
	require.NoError(t, err)
	daAddress := fmt.Sprintf("http://%s:%s", bridgeNodeNetworkInfo.Internal.IP, bridgeNodeNetworkInfo.Internal.Ports.RPC)

	reth0, _ := BuildEvolveEVM(t, ctx, testCfg, daAddress, reth0ChainName, reth0ChainID)
	reth1, _ := BuildEvolveEVM(t, ctx, testCfg, daAddress, reth1ChainName, reth1ChainID)

	d, err := hyperlane.NewDeployer(
		ctx,
		hyperlane.Config{
			Logger:          testCfg.Logger,
			DockerClient:    testCfg.DockerClient,
			DockerNetworkID: testCfg.NetworkID,
			HyperlaneImage:  hyperlane.DefaultDeployerImage(),
		},
		testCfg.TestName,
		[]hyperlane.ChainConfigProvider{reth0, reth1, celestia},
	)
	require.NoError(t, err)

	relayerBytes, err := d.ReadFile(ctx, "relayer-config.json")
	require.NoError(t, err)

	var relayerCfg hyperlane.RelayerConfig
	require.NoError(t, json.Unmarshal(relayerBytes, &relayerCfg))
	require.NotEmpty(t, relayerCfg.Chains[reth0ChainName])
	require.NotEmpty(t, relayerCfg.Chains[reth1ChainName])
	require.NotEmpty(t, relayerCfg.Chains[HypChainName])

	require.NoError(t, d.Deploy(ctx))

	schema, err := d.GetOnDiskSchema(ctx)
	require.NoError(t, err)

	assertMailbox(t, ctx, schema, reth0, reth0ChainName)
	assertMailbox(t, ctx, schema, reth1, reth1ChainName)

	broadcaster := cosmos.NewBroadcaster(celestia)
	faucet := celestia.GetNode().GetFaucetWallet()

	config, err := d.DeployCosmosNoopISM(ctx, broadcaster, faucet)
	require.NoError(t, err)
	require.NotNil(t, config)

	cosmosEntry, ok := schema.Registry.Chains[HypChainName]
	require.True(t, ok, "missing registry entry for %s", celestia.Config.Name)
	cosmosDomain := cosmosEntry.Metadata.DomainID

	networkInfo, err := celestia.GetNetworkInfo(ctx)
	require.NoError(t, err)

	warpTokens, err := queryWarpTokens(ctx, networkInfo.External.GRPCAddress())
	require.NoError(t, err)
	require.NotEmpty(t, warpTokens)
	routerHex := warpTokens[0].Id

	tokenRouter, err := d.GetEVMWarpTokenAddress()
	require.NoError(t, err)

	enrollRemote := func(chainName string, node *reth.Node) {
		t.Helper()

		networkInfo, err := node.GetNetworkInfo(ctx)
		require.NoError(t, err)

		rpcURL := fmt.Sprintf("http://%s", networkInfo.External.RPCAddress())

		txHash, err := d.EnrollRemoteRouter(ctx, tokenRouter.Hex(), cosmosDomain, routerHex, chainName, rpcURL)
		require.NoError(t, err)
		t.Logf("Enrolled remote router for %s: %s", chainName, txHash.Hex())

		entry, ok := schema.Registry.Chains[chainName]
		require.True(t, ok, "missing registry entry for %s", chainName)
		evmDomain := entry.Metadata.DomainID

		remoteTokenRouter := evm.PadAddress(tokenRouter) // leftpad to bytes32
		require.NoError(t, d.EnrollRemoteRouterOnCosmos(ctx, broadcaster, faucet, config.TokenID, evmDomain, remoteTokenRouter.String()))
	}

	enrollRemote(reth0ChainName, reth0)
	enrollRemote(reth1ChainName, reth1)

	ci, err := celestia.GetNetworkInfo(ctx)
	require.NoError(t, err)
	celestiaGRPC := ci.External.GRPCAddress()
	cconn, err := grpc.NewClient(celestiaGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() {
		_ = cconn.Close()
	}()

	warpModuleAddr := authtypes.NewModuleAddress(warptypes.ModuleName).String()
	beforeEscrow, err := query.Balance(ctx, cconn, warpModuleAddr, celestia.Config.Denom)
	require.NoError(t, err)

	sendAmount0 := sdkmath.NewInt(1000)
	sendAmount1 := sdkmath.NewInt(2000)
	receiver := gethcommon.HexToAddress("0xaF9053bB6c4346381C77C2FeD279B17ABAfCDf4d")

	reth0Entry := schema.Registry.Chains[reth0ChainName]
	reth1Entry := schema.Registry.Chains[reth1ChainName]

	txMsg0 := &warptypes.MsgRemoteTransfer{
		Sender:            faucet.GetFormattedAddress(),
		TokenId:           config.TokenID,
		DestinationDomain: reth0Entry.Metadata.DomainID,
		Recipient:         evm.PadAddress(receiver),
		Amount:            sendAmount0,
	}
	resp, err := broadcaster.BroadcastMessages(ctx, faucet, txMsg0)
	require.NoError(t, err)
	require.Equal(t, resp.Code, uint32(0), "reth0 transfer tx should succeed: code=%d, log=%s", resp.Code, resp.RawLog)
	require.NoError(t, wait.ForBlocks(ctx, 3, celestia))

	txMsg1 := &warptypes.MsgRemoteTransfer{
		Sender:            faucet.GetFormattedAddress(),
		TokenId:           config.TokenID,
		DestinationDomain: reth1Entry.Metadata.DomainID,
		Recipient:         evm.PadAddress(receiver),
		Amount:            sendAmount1,
	}
	resp, err = broadcaster.BroadcastMessages(ctx, faucet, txMsg1)
	require.NoError(t, err)
	require.Equal(t, resp.Code, uint32(0), "reth1 transfer tx should succeed: code=%d, log=%s", resp.Code, resp.RawLog)
	require.NoError(t, wait.ForBlocks(ctx, 3, celestia))

	expectedEscrow := beforeEscrow.Add(sendAmount0).Add(sendAmount1)
	afterEscrow, err := query.Balance(ctx, cconn, warpModuleAddr, celestia.Config.Denom)
	require.NoError(t, err)
	require.Truef(t, afterEscrow.Equal(expectedEscrow), "escrow should increase by transfers (%s)", celestia.Config.Denom)

	agentCfg := hyperlane.Config{
		Logger:          testCfg.Logger,
		DockerClient:    testCfg.DockerClient,
		DockerNetworkID: testCfg.NetworkID,
		HyperlaneImage:  container.NewImage("damiannolan/hyperlane-agent", "test", "1000:1000"),
	}

	agent, err := hyperlane.NewAgent(ctx, agentCfg, testCfg.TestName, hyperlane.AgentTypeRelayer, d)
	require.NoError(t, err)
	require.NoError(t, agent.Start(ctx))
	t.Cleanup(func() {
		_ = agent.Stop(ctx)
		_ = agent.Remove(ctx)
	})

	ec0, err := reth0.GetEthClient(ctx)
	require.NoError(t, err)
	ec1, err := reth1.GetEthClient(ctx)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		balance, err := evm.GetERC20Balance(ctx, ec0, tokenRouter, receiver)
		if err != nil {
			t.Logf("reth0 balance query failed: %v", err)
			return false
		}
		t.Logf("reth0 recipient warp token balance: %s", balance.String())
		return balance.Cmp(sendAmount0.BigInt()) == 0
	}, 2*time.Minute, 5*time.Second, "reth0 recipient should receive minted warp tokens")

	require.Eventually(t, func() bool {
		balance, err := evm.GetERC20Balance(ctx, ec1, tokenRouter, receiver)
		if err != nil {
			t.Logf("reth1 balance query failed: %v", err)
			return false
		}
		t.Logf("reth1 recipient warp token balance: %s", balance.String())
		return balance.Cmp(sendAmount1.BigInt()) == 0
	}, 2*time.Minute, 5*time.Second, "reth1 recipient should receive minted warp tokens")
}

func BuildEvolveEVM(t *testing.T, ctx context.Context, testCfg *TestSetupConfig, daAddress, chainName string, chainID int) (*reth.Node, *evmsingle.Chain) {
	t.Helper()

	rethNode, err := reth.NewNodeBuilderWithTestName(t, testCfg.TestName).
		WithName(chainName).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithGenesis([]byte(reth.DefaultEvolveGenesisJSON(reth.WithChainID(chainID)))).
		WithHyperlaneChainName(chainName).
		WithHyperlaneChainID(uint64(chainID)).
		WithHyperlaneDomainID(uint32(chainID)).
		Build(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = rethNode.Stop(ctx)
		_ = rethNode.Remove(ctx)
	})

	require.NoError(t, rethNode.Start(ctx))

	networkInfo, err := rethNode.GetNetworkInfo(ctx)
	require.NoError(t, err)

	genesisHash, err := rethNode.GenesisHash(ctx)
	require.NoError(t, err)

	evmEthURL := fmt.Sprintf("http://%s:%s", networkInfo.Internal.Hostname, networkInfo.Internal.Ports.RPC)
	evmEngineURL := fmt.Sprintf("http://%s:%s", networkInfo.Internal.Hostname, networkInfo.Internal.Ports.Engine)

	evNodeCfg := evmsingle.NewNodeConfigBuilder().
		WithEVMEngineURL(evmEngineURL).
		WithEVMETHURL(evmEthURL).
		WithEVMJWTSecret(rethNode.JWTSecretHex()).
		WithEVMSignerPassphrase("secret").
		WithEVMBlockTime("1s").
		WithEVMGenesisHash(genesisHash).
		WithDAAddress(daAddress).
		Build()

	seqNode, err := evmsingle.NewChainBuilderWithTestName(t, testCfg.TestName).
		WithName(chainName).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithNodes(evNodeCfg).
		Build(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = seqNode.Stop(ctx)
		_ = seqNode.Remove(ctx)
	})

	require.NoError(t, seqNode.Start(ctx))

	evmNodes := seqNode.Nodes()
	require.Len(t, evmNodes, 1)
	assertEvmSingleHealthy(t, ctx, evmNodes[0])

	return rethNode, seqNode
}

func assertMailbox(t *testing.T, ctx context.Context, schema *hyperlane.Schema, node *reth.Node, chainName string) {
	t.Helper()

	entry, ok := schema.Registry.Chains[chainName]
	require.True(t, ok, "missing registry entry for %s", chainName)

	mailbox := string(entry.Addresses.Mailbox)

	ethClient, err := node.GetEthClient(ctx)
	require.NoError(t, err)

	code, err := ethClient.CodeAt(ctx, gethcommon.HexToAddress(mailbox), nil)
	require.NoError(t, err, "failed to fetch code for mailbox")
	require.Greaterf(t, len(code), 0, "should have non-empty code")
}
