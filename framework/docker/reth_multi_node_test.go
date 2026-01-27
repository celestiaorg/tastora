package docker

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	"github.com/celestiaorg/tastora/framework/docker/evstack/reth"
	"github.com/celestiaorg/tastora/framework/testutil/deploy"
	"github.com/stretchr/testify/require"
)

func TestRethNodes_IndependentInstances(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	testCfg := setupDockerTest(t)

	reth0, err := reth.NewNodeBuilderWithTestName(t, testCfg.TestName).
		WithName("primary").
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithGenesis([]byte(reth.DefaultEvolveGenesisJSON())).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = reth0.Stop(testCfg.Ctx)
		_ = reth0.Remove(testCfg.Ctx)
	})

	reth1, err := reth.NewNodeBuilderWithTestName(t, testCfg.TestName).
		WithName("secondary").
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithGenesis([]byte(reth.DefaultEvolveGenesisJSON())).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = reth1.Stop(testCfg.Ctx)
		_ = reth1.Remove(testCfg.Ctx)
	})

	require.NotEqual(t, reth0.HostName(), reth1.HostName(), "hostnames should be unique")
	require.NotEqual(t, reth0.Name(), reth1.Name(), "container names should be unique")

	require.Contains(t, reth0.Name(), "primary", "reth0 name should contain 'primary'")
	require.Contains(t, reth1.Name(), "secondary", "reth1 name should contain 'secondary'")
}

func TestMultipleRethEvmSinglePairs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	testCfg := setupDockerTest(t)
	ctx := testCfg.Ctx

	celestia, daNetwork, err := deploy.CelestiaWithDA(ctx, testCfg.ChainBuilder, testCfg.DANetworkBuilder)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = celestia.Stop(ctx)
	})

	bridge := daNetwork.GetBridgeNodes()[0]
	bridgeNodeNetworkInfo, err := bridge.GetNetworkInfo(ctx)
	require.NoError(t, err)
	daAddress := fmt.Sprintf("http://%s:%s", bridgeNodeNetworkInfo.Internal.IP, bridgeNodeNetworkInfo.Internal.Ports.RPC)

	rethPrimary, evmPrimary := deployRethEvmPair(t, testCfg, "primary", daAddress)
	rethSecondary, evmSecondary := deployRethEvmPair(t, testCfg, "secondary", daAddress)

	require.NotEqual(t, rethPrimary.Name(), rethSecondary.Name(), "reth names should be unique")
	require.Contains(t, rethPrimary.Name(), "primary")
	require.Contains(t, rethSecondary.Name(), "secondary")

	evmPrimaryNodes := evmPrimary.Nodes()
	evmSecondaryNodes := evmSecondary.Nodes()
	require.Len(t, evmPrimaryNodes, 1)
	require.Len(t, evmSecondaryNodes, 1)

	require.NotEqual(t, evmPrimaryNodes[0].Name(), evmSecondaryNodes[0].Name(), "evmsingle names should be unique")
	require.Contains(t, evmPrimaryNodes[0].Name(), "primary")
	require.Contains(t, evmSecondaryNodes[0].Name(), "secondary")

	assertEvmSingleHealthy(t, ctx, evmPrimaryNodes[0])
	assertEvmSingleHealthy(t, ctx, evmSecondaryNodes[0])
}

func deployRethEvmPair(t *testing.T, testCfg *TestSetupConfig, name, daAddress string) (*reth.Node, *evmsingle.Chain) {
	t.Helper()
	ctx := testCfg.Ctx

	rethNode, err := reth.NewNodeBuilderWithTestName(t, testCfg.TestName).
		WithName(name).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithGenesis([]byte(reth.DefaultEvolveGenesisJSON())).
		Build(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = rethNode.Stop(ctx)
		_ = rethNode.Remove(ctx)
	})

	require.NoError(t, rethNode.Start(ctx))

	rni, err := rethNode.GetNetworkInfo(ctx)
	require.NoError(t, err)

	evmEthURL := fmt.Sprintf("http://%s:%s", rni.Internal.Hostname, rni.Internal.Ports.RPC)
	evmEngineURL := fmt.Sprintf("http://%s:%s", rni.Internal.Hostname, rni.Internal.Ports.Engine)
	rGenesisHash, err := rethNode.GenesisHash(ctx)
	require.NoError(t, err)

	evNodeCfg := evmsingle.NewNodeConfigBuilder().
		WithEVMEngineURL(evmEngineURL).
		WithEVMETHURL(evmEthURL).
		WithEVMJWTSecret(rethNode.JWTSecretHex()).
		WithEVMSignerPassphrase("secret").
		WithEVMBlockTime("1s").
		WithEVMGenesisHash(rGenesisHash).
		WithDAAddress(daAddress).
		Build()

	evmChain, err := evmsingle.NewChainBuilderWithTestName(t, testCfg.TestName).
		WithName(name).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithNodes(evNodeCfg).
		Build(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = evmChain.Stop(ctx)
		_ = evmChain.Remove(ctx)
	})

	require.NoError(t, evmChain.Start(ctx))

	return rethNode, evmChain
}

func assertEvmSingleHealthy(t *testing.T, ctx context.Context, node *evmsingle.Node) {
	t.Helper()
	networkInfo, err := node.GetNetworkInfo(ctx)
	require.NoError(t, err)

	healthURL := fmt.Sprintf("http://0.0.0.0:%s/health/ready", networkInfo.External.Ports.RPC)
	require.Eventually(t, func() bool {
		req, _ := http.NewRequest(http.MethodGet, healthURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode == http.StatusOK
	}, 60*time.Second, 2*time.Second, "evm-single %s did not become healthy", node.Name())
}
