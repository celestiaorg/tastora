package docker

import (
	"cosmossdk.io/math"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// TestCreateWalletOnSpecificNode tests wallet creation on specific nodes.
func (s *DockerTestSuite) TestCreateWalletOnSpecificNode() {
	// create chain with 2 validators and 1 full node
	s.builder = s.builder.WithNodes(
		NewChainNodeConfigBuilder().WithNodeType(ValidatorNodeType).Build(),
		NewChainNodeConfigBuilder().WithNodeType(ValidatorNodeType).Build(),
		NewChainNodeConfigBuilder().WithNodeType(FullNodeType).Build(),
	)

	var err error
	s.chain, err = s.builder.Build(s.ctx)
	s.Require().NoError(err)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	// test creating wallet on validator[0]
	wallet1, err := s.chain.Validators[0].CreateWallet(s.ctx, "test-key-1", s.chain.cfg.ChainConfig.Bech32Prefix)
	s.Require().NoError(err)
	s.Require().NotNil(wallet1)
	s.Require().Equal("test-key-1", wallet1.GetKeyName())
	s.Require().NotEmpty(wallet1.GetFormattedAddress())

	// test creating wallet on validator[1]
	wallet2, err := s.chain.Validators[1].CreateWallet(s.ctx, "test-key-2", s.chain.cfg.ChainConfig.Bech32Prefix)
	s.Require().NoError(err)
	s.Require().NotNil(wallet2)
	s.Require().Equal("test-key-2", wallet2.GetKeyName())
	s.Require().NotEmpty(wallet2.GetFormattedAddress())

	// wallets should have different addresses
	s.Require().NotEqual(wallet1.GetFormattedAddress(), wallet2.GetFormattedAddress())

	// test creating wallet on full node
	wallet3, err := s.chain.FullNodes[0].CreateWallet(s.ctx, "test-key-3", s.chain.cfg.ChainConfig.Bech32Prefix)
	s.Require().NoError(err)
	s.Require().NotNil(wallet3)
	s.Require().Equal("test-key-3", wallet3.GetKeyName())
	s.Require().NotEmpty(wallet3.GetFormattedAddress())
}

// TestGetFaucetWalletOnNodes tests faucet wallet accessibility on all nodes.
func (s *DockerTestSuite) TestGetFaucetWalletOnNodes() {
	// create chain with 2 validators
	s.builder = s.builder.WithNodes(
		NewChainNodeConfigBuilder().WithNodeType(ValidatorNodeType).Build(),
		NewChainNodeConfigBuilder().WithNodeType(ValidatorNodeType).Build(),
	)

	var err error
	s.chain, err = s.builder.Build(s.ctx)
	s.Require().NoError(err)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	// faucet wallet should be available on all validators
	faucetWallet1 := s.chain.Validators[0].GetFaucetWallet()
	s.Require().NotNil(faucetWallet1)
	s.Require().NotEmpty(faucetWallet1.GetFormattedAddress())

	faucetWallet2 := s.chain.Validators[1].GetFaucetWallet()
	s.Require().NotNil(faucetWallet2)
	s.Require().Equal(faucetWallet1.GetFormattedAddress(), faucetWallet2.GetFormattedAddress())

	// chain's GetFaucetWallet should return same as validator[0]
	chainFaucetWallet := s.chain.GetFaucetWallet()
	s.Require().Equal(faucetWallet1.GetFormattedAddress(), chainFaucetWallet.GetFormattedAddress())

	// verify faucet wallet can be used for broadcasting transactions on each node
	// create a test recipient wallet
	testWallet, err := s.chain.Validators[0].CreateWallet(s.ctx, "test-recipient", s.chain.cfg.ChainConfig.Bech32Prefix)
	s.Require().NoError(err)

	// test broadcasting from validator[0] using faucet wallet
	err = s.testFaucetBroadcast(s.chain.Validators[0], faucetWallet1, testWallet)
	s.Require().NoError(err)

	// test broadcasting from validator[1] using faucet wallet
	err = s.testFaucetBroadcast(s.chain.Validators[1], faucetWallet2, testWallet)
	s.Require().NoError(err)
}

// TestCreateWalletBackwardCompatibility tests that existing Chain.CreateWallet still works.
func (s *DockerTestSuite) TestCreateWalletBackwardCompatibility() {
	var err error
	s.chain, err = s.builder.Build(s.ctx)
	s.Require().NoError(err)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	// existing Chain.CreateWallet should still work
	wallet, err := s.chain.CreateWallet(s.ctx, "test-wallet")
	s.Require().NoError(err)
	s.Require().NotNil(wallet)
	s.Require().Equal("test-wallet", wallet.GetKeyName())
	s.Require().NotEmpty(wallet.GetFormattedAddress())
}

// testFaucetBroadcast helper function to test broadcasting a bank send transaction using the faucet wallet on a specific node
func (s *DockerTestSuite) testFaucetBroadcast(node *ChainNode, faucetWallet, recipientWallet types.Wallet) error {
	// create a bank send message
	sendAmount := sdk.NewCoins(sdk.NewCoin(s.chain.cfg.ChainConfig.Denom, math.NewInt(1000)))
	msg := &banktypes.MsgSend{
		FromAddress: faucetWallet.GetFormattedAddress(),
		ToAddress:   recipientWallet.GetFormattedAddress(),
		Amount:      sendAmount,
	}

	// get a broadcaster that will broadcast through the specific node
	broadcaster := node.GetBroadcaster(s.chain)

	// broadcast the transaction using the node-specific broadcaster
	_, err := broadcaster.BroadcastMessages(s.ctx, faucetWallet, msg)
	return err
}
