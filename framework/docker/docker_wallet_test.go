package docker

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/stretchr/testify/require"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// TestCreateWalletOnSpecificNode tests wallet creation on specific nodes.
func TestCreateWalletOnSpecificNode(t *testing.T) {
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	// create chain with 2 validators and 1 full node
	builder := testCfg.Builder.WithNodes(
		NewChainNodeConfigBuilder().WithNodeType(types.NodeTypeValidator).Build(),
		NewChainNodeConfigBuilder().WithNodeType(types.NodeTypeValidator).Build(),
		NewChainNodeConfigBuilder().WithNodeType(types.NodeTypeConsensusFull).Build(),
	)

	chain, err := builder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	// test creating wallet on validator[0]
	wallet1, err := chain.Validators[0].CreateWallet(testCfg.Ctx, "test-key-1", chain.cfg.ChainConfig.Bech32Prefix)
	require.NoError(t, err)
	require.NotNil(t, wallet1)
	require.Equal(t, "test-key-1", wallet1.GetKeyName())
	require.NotEmpty(t, wallet1.GetFormattedAddress())

	// test creating wallet on validator[1]
	wallet2, err := chain.Validators[1].CreateWallet(testCfg.Ctx, "test-key-2", chain.cfg.ChainConfig.Bech32Prefix)
	require.NoError(t, err)
	require.NotNil(t, wallet2)
	require.Equal(t, "test-key-2", wallet2.GetKeyName())
	require.NotEmpty(t, wallet2.GetFormattedAddress())

	// wallets should have different addresses
	require.NotEqual(t, wallet1.GetFormattedAddress(), wallet2.GetFormattedAddress())

	// test creating wallet on full node
	wallet3, err := chain.FullNodes[0].CreateWallet(testCfg.Ctx, "test-key-3", chain.cfg.ChainConfig.Bech32Prefix)
	require.NoError(t, err)
	require.NotNil(t, wallet3)
	require.Equal(t, "test-key-3", wallet3.GetKeyName())
	require.NotEmpty(t, wallet3.GetFormattedAddress())
}

// TestGetFaucetWalletOnNodes tests faucet wallet accessibility on all nodes.
func TestGetFaucetWalletOnNodes(t *testing.T) {
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	// create chain with 2 validators
	builder := testCfg.Builder.WithNodes(
		NewChainNodeConfigBuilder().WithNodeType(types.NodeTypeValidator).Build(),
		NewChainNodeConfigBuilder().WithNodeType(types.NodeTypeValidator).Build(),
	)

	chain, err := builder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	// faucet wallet should be available on all validators
	faucetWallet1 := chain.Validators[0].GetFaucetWallet()
	require.NotNil(t, faucetWallet1)
	require.NotEmpty(t, faucetWallet1.GetFormattedAddress())

	faucetWallet2 := chain.Validators[1].GetFaucetWallet()
	require.NotNil(t, faucetWallet2)
	require.Equal(t, faucetWallet1.GetFormattedAddress(), faucetWallet2.GetFormattedAddress())

	// chain's GetFaucetWallet should return same as validator[0]
	chainFaucetWallet := chain.GetFaucetWallet()
	require.Equal(t, faucetWallet1.GetFormattedAddress(), chainFaucetWallet.GetFormattedAddress())

	// verify faucet wallet can be used for broadcasting transactions on each node
	// create a test recipient wallet
	testWallet, err := chain.Validators[0].CreateWallet(testCfg.Ctx, "test-recipient", chain.cfg.ChainConfig.Bech32Prefix)
	require.NoError(t, err)

	// test broadcasting from validator[0] using faucet wallet
	err = testFaucetBroadcast(chain, chain.Validators[0], faucetWallet1, testWallet)
	require.NoError(t, err)

	// test broadcasting from validator[1] using faucet wallet
	err = testFaucetBroadcast(chain, chain.Validators[1], faucetWallet2, testWallet)
	require.NoError(t, err)
}

// TestCreateWalletBackwardCompatibility tests that existing Chain.CreateWallet still works.
func TestCreateWalletBackwardCompatibility(t *testing.T) {
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	chain, err := testCfg.Builder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	// existing Chain.CreateWallet should still work
	wallet, err := chain.CreateWallet(testCfg.Ctx, "test-wallet")
	require.NoError(t, err)
	require.NotNil(t, wallet)
	require.Equal(t, "test-wallet", wallet.GetKeyName())
	require.NotEmpty(t, wallet.GetFormattedAddress())
}

// testFaucetBroadcast helper function to test broadcasting a bank send transaction using the faucet wallet on a specific node
func testFaucetBroadcast(chain *Chain, node *ChainNode, faucetWallet, recipientWallet *types.Wallet) error {
	// create a bank send message
	sendAmount := sdk.NewCoins(sdk.NewCoin(chain.cfg.ChainConfig.Denom, math.NewInt(1000)))
	msg := &banktypes.MsgSend{
		FromAddress: faucetWallet.GetFormattedAddress(),
		ToAddress:   recipientWallet.GetFormattedAddress(),
		Amount:      sendAmount,
	}

	// get a broadcaster that will broadcast through the specific node
	broadcaster := node.GetBroadcaster(chain)

	// broadcast the transaction using the node-specific broadcaster
	_, err := broadcaster.BroadcastMessages(context.Background(), faucetWallet, msg)
	return err
}
