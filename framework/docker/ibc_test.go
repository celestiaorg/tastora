package docker

import (
	"context"
	"github.com/celestiaorg/tastora/framework/docker/ibc"
	"github.com/celestiaorg/tastora/framework/docker/ibc/relayer"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sync/errgroup"
)

// TestCreateAndFundWallet tests wallet creation and funding.
func (s *DockerTestSuite) TestIBC() {
	var err error
	ctx := context.TODO()

	s.chain, err = s.builder.Build(s.ctx)
	s.Require().NoError(err)

	s.builder = s.builder.WithChainID("chain-b")

	chainB, err := s.builder.Build(s.ctx)
	s.Require().NoError(err)

	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return s.chain.Start(egCtx)
	})
	eg.Go(func() error {
		return chainB.Start(egCtx)
	})

	s.Require().NoError(eg.Wait())

	r, err := relayer.NewHermes(ctx, s.dockerClient, s.T().Name(), s.networkID, zaptest.NewLogger(s.T()))
	s.Require().NoError(err)
	
	connector := ibc.NewConnector(s.chain, chainB, r)
	err = connector.SetupRelayerWallets(ctx)
	s.Require().NoError(err)

	err = connector.Connect(ctx)
	s.Require().NoError(err)

}
