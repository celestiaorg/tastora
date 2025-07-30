package docker

import (
	"context"
	"github.com/celestiaorg/tastora/framework/docker/ibc"
	"github.com/celestiaorg/tastora/framework/docker/ibc/relayer"
	"github.com/celestiaorg/tastora/framework/types"
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

	err = r.Init(ctx, s.chain, chainB)
	s.Require().NoError(err)

	err = r.SetupWallets(ctx, s.chain, chainB)
	s.Require().NoError(err)

	connection, channel := s.setupIBCConnection(ctx, r, s.chain, chainB)
	
	s.T().Logf("Created IBC connection: %s <-> %s", connection.ConnectionID, connection.CounterpartyID)
	s.T().Logf("Created IBC channel: %s <-> %s", channel.ChannelID, channel.CounterpartyID)

}

// setupIBCConnection is a helper function that establishes a complete IBC connection and channel
func (s *DockerTestSuite) setupIBCConnection(ctx context.Context, r *relayer.Hermes, chainA, chainB types.Chain) (ibc.Connection, *ibc.Channel) {
	// Create clients
	err := r.CreateClients(ctx, chainA, chainB)
	s.Require().NoError(err)

	// Create connections
	connection, err := r.CreateConnections(ctx, chainA, chainB)
	s.Require().NoError(err)
	s.Require().NotEmpty(connection.ConnectionID, "Connection ID should not be empty")

	// Create an ICS20 channel for token transfers
	channelOpts := ibc.CreateChannelOptions{
		SourcePortName: "transfer",
		DestPortName:   "transfer", 
		Order:          ibc.OrderUnordered,
		Version:        "ics20-1",
	}
	
	channel, err := r.CreateChannel(ctx, chainA, chainB, connection, channelOpts)
	s.Require().NoError(err)
	s.Require().NotNil(channel)
	s.Require().NotEmpty(channel.ChannelID, "Channel ID should not be empty")

	return connection, channel
}
