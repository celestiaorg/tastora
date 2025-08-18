package docker

import (
	"cosmossdk.io/math"
	wallet "github.com/celestiaorg/tastora/framework/testutil/wallet"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestCreateAndFundWallet tests wallet creation and funding.
func TestCreateAndFundWallet(t *testing.T) {
	t.Parallel()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	chain, err := testCfg.Builder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	amount := math.NewInt(1000000)
	sendAmount := sdk.NewCoins(sdk.NewCoin("utia", amount))
	testWallet, err := wallet.CreateAndFund(testCfg.Ctx, "test", sendAmount, chain)
	require.NoError(t, err)
	require.NotNil(t, testWallet)
	require.NotEmpty(t, testWallet.GetFormattedAddress())
	require.NotEmpty(t, testWallet.GetKeyName())
	require.NotEmpty(t, testWallet.GetBech32Prefix())
	t.Logf("Created and funded wallet with address: %s", testWallet.GetFormattedAddress())
}
