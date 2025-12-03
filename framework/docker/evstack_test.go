package docker

import (
	"fmt"
	"testing"

	"cosmossdk.io/math"
	"github.com/celestiaorg/tastora/framework/docker/container"
	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	"github.com/celestiaorg/tastora/framework/docker/evstack"
	sdkacc "github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
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

	chain, err := testCfg.ChainBuilder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	// Create DA network using builder pattern
	celestiaImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-node",
		Version:    "v0.26.4",
		UIDGID:     "10001:10001",
	}

	bridgeNodeConfig := da.NewNodeBuilder().
		WithNodeType(types.BridgeNode).
		Build()

	daNetwork, err := testCfg.DANetworkBuilder.
		WithChainID(chain.GetChainID()).
		WithImage(celestiaImage).
		WithNodes(bridgeNodeConfig).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	genesisHash, err := getGenesisHash(testCfg.Ctx, chain)
	require.NoError(t, err)

	networkInfo, err := chain.GetNodes()[0].GetNetworkInfo(testCfg.Ctx)
	require.NoError(t, err, "failed to get network info")
	hostname := networkInfo.Internal.Hostname

	bridgeNode := daNetwork.GetBridgeNodes()[0]
	chainID := chain.GetChainID()

	t.Run("bridge node can be started", func(t *testing.T) {
		err = bridgeNode.Start(testCfg.Ctx,
			da.WithChainID(chainID),
			da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			da.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		require.NoError(t, err)
	})

	daWallet, err := bridgeNode.GetWallet()
	require.NoError(t, err)
	t.Logf("da node celestia address: %s", daWallet.GetFormattedAddress())

	// Fund the da node address
	fromAddress, err := sdkacc.AddressFromWallet(chain.GetFaucetWallet())
	require.NoError(t, err)

	toAddress, err := sdk.AccAddressFromBech32(daWallet.GetFormattedAddress())
	require.NoError(t, err)

	// Fund the ev node wallet with coins
	bankSend := banktypes.NewMsgSend(fromAddress, toAddress, sdk.NewCoins(sdk.NewCoin("utia", math.NewInt(100_000_000_00))))
	_, err = chain.BroadcastMessages(testCfg.Ctx, chain.GetFaucetWallet(), bankSend)
	require.NoError(t, err)

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
