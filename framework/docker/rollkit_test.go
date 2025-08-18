package docker

import (
	"fmt"
	"testing"

	"cosmossdk.io/math"
	sdkacc "github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"
)

func TestRollkit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	provider := testCfg.CreateDockerProvider(t)
	chain, err := testCfg.Builder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	daNetwork, err := provider.GetDataAvailabilityNetwork(testCfg.Ctx)
	require.NoError(t, err)

	genesisHash, err := getGenesisHash(testCfg.Ctx, chain)
	require.NoError(t, err)

	hostname, err := chain.GetNodes()[0].GetInternalHostName(testCfg.Ctx)
	require.NoError(t, err, "failed to get internal hostname")

	bridgeNode := daNetwork.GetBridgeNodes()[0]
	chainID := chain.GetChainID()

	t.Run("bridge node can be started", func(t *testing.T) {
		err = bridgeNode.Start(testCfg.Ctx,
			types.WithChainID(chainID),
			types.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			types.WithEnvironmentVariables(
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

	// Fund the rollkit node wallet with coins
	bankSend := banktypes.NewMsgSend(fromAddress, toAddress, sdk.NewCoins(sdk.NewCoin("utia", math.NewInt(100_000_000_00))))
	_, err = chain.BroadcastMessages(testCfg.Ctx, chain.GetFaucetWallet(), bankSend)
	require.NoError(t, err)

	rollkit, err := provider.GetRollkitChain(testCfg.Ctx)
	require.NoError(t, err)

	nodes := rollkit.GetNodes()
	require.Len(t, nodes, 1)
	aggregatorNode := nodes[0]

	err = aggregatorNode.Init(testCfg.Ctx)
	require.NoError(t, err)

	authToken, err := bridgeNode.GetAuthToken()
	require.NoError(t, err)

	// Use the configured RPC port instead of hardcoded 26658
	bridgeRPCAddress, err := bridgeNode.GetInternalRPCAddress()
	require.NoError(t, err)
	daAddress := fmt.Sprintf("http://%s", bridgeRPCAddress)
	err = aggregatorNode.Start(testCfg.Ctx,
		"--rollkit.da.address", daAddress,
		"--rollkit.da.gas_price", "0.025",
		"--rollkit.da.auth_token", authToken,
		"--rollkit.rpc.address", "0.0.0.0:7331", // bind to 0.0.0.0 so rpc is reachable from test host.
		"--rollkit.da.header_namespace", "ev-header",
		"--rollkit.da.data_namespace", "ev-data",
	)
	require.NoError(t, err)
}
