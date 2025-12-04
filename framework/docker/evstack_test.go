package docker

import (
	"fmt"
	"testing"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/evstack"
	"github.com/stretchr/testify/require"
)

func TestEvstack(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	_, daNetwork, err := DeployCelestiaWithDABridgeNode(t, testCfg)
	require.NoError(t, err)

	bridgeNode := daNetwork.GetBridgeNodes()[0]

	evstackImage := container.Image{
		Repository: "ghcr.io/evstack/ev-node",
		Version:    "v1.0.0-beta.8",
		UIDGID:     "10001:10001",
	}

	aggregatorNodeConfig := evstack.NewNodeBuilder().
		WithAggregator(true).
		Build()

	evstackChain, err := evstack.NewChainBuilder(t).
		WithChainID("test").
		WithBinaryName("testapp").
		WithAggregatorPassphrase("12345678").
		WithImage(evstackImage).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithNode(aggregatorNodeConfig).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	nodes := evstackChain.GetNodes()
	require.Len(t, nodes, 1)
	aggregatorNode := nodes[0]

	err = aggregatorNode.Init(testCfg.Ctx)
	require.NoError(t, err)

	authToken, err := bridgeNode.GetAuthToken()
	require.NoError(t, err)

	// Use the configured RPC port instead of hardcoded 26658
	bridgeNetworkInfo, err := bridgeNode.GetNetworkInfo(testCfg.Ctx)
	require.NoError(t, err)
	bridgeRPCAddress := bridgeNetworkInfo.Internal.RPCAddress()
	daAddress := fmt.Sprintf("http://%s", bridgeRPCAddress)
	err = aggregatorNode.Start(testCfg.Ctx,
		"--evnode.da.address", daAddress,
		"--evnode.da.gas_price", "0.025",
		"--evnode.da.auth_token", authToken,
		"--evnode.rpc.address", "0.0.0.0:7331", // bind to 0.0.0.0 so rpc is reachable from test host.
		"--evnode.da.namespace", "ev-header",
		"--evnode.da.data_namespace", "ev-data",
	)
	require.NoError(t, err)
}
