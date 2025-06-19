package docker

import (
	"cosmossdk.io/math"
	wallet "github.com/celestiaorg/tastora/framework/testutil/wallet"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// TestCreateAndFundWallet tests wallet creation and funding.
func (s *DockerTestSuite) TestCreateAndFundWallet() {
	var err error
	s.provider = s.CreateDockerProvider()
	s.chain, err = s.provider.GetChain(s.ctx)
	s.Require().NoError(err)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	amount := math.NewInt(1000000)
	sendAmount := sdk.NewCoins(sdk.NewCoin("utia", amount))
	testWallet, err := wallet.CreateAndFund(s.ctx, "test", sendAmount, s.chain)
	s.Require().NoError(err)
	s.Require().NotNil(testWallet)
	s.Require().NotEmpty(testWallet.GetFormattedAddress())
	s.Require().NotEmpty(testWallet.GetKeyName())
	s.Require().NotEmpty(testWallet.GetBech32Prefix())
	s.T().Logf("Created and funded wallet with address: %s", testWallet.GetFormattedAddress())
}
