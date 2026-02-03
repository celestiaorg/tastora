package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	"github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	"github.com/celestiaorg/tastora/framework/docker/evstack/reth"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"github.com/celestiaorg/tastora/framework/testutil/deploy"
	"github.com/celestiaorg/tastora/framework/testutil/evm"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

var (
	reth0ChainName = "reth0"
	reth1ChainName = "reth1"
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

	reth0, _ := BuildEvolveEVM(t, ctx, testCfg, daAddress, reth0ChainName, 1234)
	reth1, _ := BuildEvolveEVM(t, ctx, testCfg, daAddress, reth1ChainName, 1235)

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
	require.NotEmpty(t, relayerCfg.Chains[celestia.Config.Name])

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

	cosmosEntry, ok := schema.Registry.Chains[celestia.Config.Name]
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
