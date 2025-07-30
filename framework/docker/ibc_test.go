package docker

import (
	"context"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/testutil/config"
	servercfg "github.com/cosmos/cosmos-sdk/server/config"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/celestiaorg/tastora/framework/docker/ibc"
	"github.com/celestiaorg/tastora/framework/docker/ibc/relayer"
	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sync/errgroup"
)

// TestCreateAndFundWallet tests wallet creation and funding.
func (s *DockerTestSuite) TestIBC() {
	var err error
	ctx := context.TODO()

	s.builder = s.builder.WithImage(container.NewImage("ghcr.io/celestiaorg/celestia-app", "v5.0.1-rc1", "10001")).
		WithGasPrices("0.000001utia").
		WithPostInit(func(ctx context.Context, node *ChainNode) error {
			return config.Modify(ctx, node, "config/app.toml", func(cfg *servercfg.Config) {
				cfg.MinGasPrices = "0.000001utia"
			})
		})

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

	// Test IBC token transfer (relayer will be started after escrow check)
	s.testIBCTransfer(ctx, r, s.chain, chainB, channel)

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

// testIBCTransfer tests sending tokens from chainA to chainB via IBC
func (s *DockerTestSuite) testIBCTransfer(ctx context.Context, relayer *relayer.Hermes, chainA, chainB types.Chain, channel *ibc.Channel) {
	// Create a test wallet on chainA to send tokens from
	senderWallet, err := chainA.CreateWallet(ctx, "ibc-sender")
	s.Require().NoError(err)

	s.T().Logf("Created IBC sender wallet: %s", senderWallet.GetFormattedAddress())

	// Fund the sender wallet
	faucetA := chainA.GetFaucetWallet()
	chainAConfig := chainA.GetChainConfig()

	fromAddr, err := sdkacc.AddressFromWallet(faucetA)
	s.Require().NoError(err)

	senderAddr, err := sdkacc.AddressFromWallet(senderWallet)
	s.Require().NoError(err)

	// Fund sender with tokens to transfer
	fundAmount := sdk.NewCoins(sdk.NewCoin(chainAConfig.Denom, sdkmath.NewInt(1000000))) // 1 token
	bankSend := banktypes.NewMsgSend(fromAddr, senderAddr, fundAmount)

	resp, err := chainA.BroadcastMessages(ctx, faucetA, bankSend)
	s.Require().NoError(err)
	s.Require().Equal(uint32(0), resp.Code, "funding transaction failed: %s", resp.RawLog)

	// Create receiver wallet on chainB
	receiverWallet, err := chainB.CreateWallet(ctx, "ibc-receiver")
	s.Require().NoError(err)

	s.T().Logf("Created IBC receiver wallet: %s", receiverWallet.GetFormattedAddress())

	receiverAddr, err := sdkacc.AddressFromWallet(receiverWallet)
	s.Require().NoError(err)

	// Check initial balances
	s.T().Logf("Checking initial balances...")
	initialSenderBalance := s.getBalance(ctx, chainA, senderAddr, chainAConfig.Denom)
	s.T().Logf("Sender initial balance: %s %s", initialSenderBalance.String(), chainAConfig.Denom)

	// Calculate the IBC denom for chainA's token on chainB
	ibcDenom := s.calculateIBCDenom(channel.CounterpartyPort, channel.CounterpartyID, chainAConfig.Denom)
	initialReceiverBalance := s.getBalance(ctx, chainB, receiverAddr, ibcDenom)
	s.T().Logf("Receiver initial IBC balance: %s %s", initialReceiverBalance.String(), ibcDenom)

	// Send IBC transfer
	transferAmount := sdkmath.NewInt(100000) // 0.1 tokens
	s.T().Logf("Sending IBC transfer: %s %s from %s to %s", transferAmount.String(), chainAConfig.Denom, chainA.GetChainID(), chainB.GetChainID())

	// Now start the relayer to process the packet
	s.T().Logf("Starting Hermes relayer to process packets...")
	err = relayer.Start(ctx)
	s.Require().NoError(err)

	ibcTransfer := ibctransfertypes.NewMsgTransfer(
		channel.PortID,
		channel.ChannelID,
		sdk.NewCoin(chainAConfig.Denom, transferAmount),
		senderAddr.String(),
		receiverAddr.String(),
		clienttypes.NewHeight(0, 0),                  // timeout height (0 = no timeout)
		uint64(time.Now().Add(time.Hour).UnixNano()), // timeout timestamp
		"", // memo
	)

	resp, err = chainA.BroadcastMessages(ctx, senderWallet, ibcTransfer)
	s.Require().NoError(err)
	s.Require().Equal(uint32(0), resp.Code, "IBC transfer failed: %s", resp.RawLog)
	s.T().Log(resp.RawLog)
	// Wait a moment for the escrow transaction to be reflected in balances
	s.T().Logf("Waiting for balance updates...")
	err = wait.ForBlocks(ctx, 5, chainA)
	s.Require().NoError(err)

	intermediateReceiverBalance := s.getBalance(ctx, chainB, receiverAddr, ibcDenom)
	s.T().Logf("Receiver balance before relay: %s %s", intermediateReceiverBalance.String(), ibcDenom)

	time.Sleep(time.Hour)

	// Wait for relayer to process transfer
	s.T().Logf("Waiting for relayer to process transfer...")
	err = wait.ForBlocks(ctx, 30, chainA, chainB)
	s.Require().NoError(err)

	// Check final balances
	s.T().Logf("Checking final balances...")
	finalSenderBalance := s.getBalance(ctx, chainA, senderAddr, chainAConfig.Denom)
	finalReceiverBalance := s.getBalance(ctx, chainB, receiverAddr, ibcDenom)

	s.T().Logf("Sender final balance: %s %s", finalSenderBalance.String(), chainAConfig.Denom)
	s.T().Logf("Receiver final IBC balance: %s %s", finalReceiverBalance.String(), ibcDenom)

	// Verify final balances
	// Receiver should have received the transferred tokens
	expectedReceiverBalance := initialReceiverBalance.Add(transferAmount)
	s.Require().True(finalReceiverBalance.Equal(expectedReceiverBalance),
		"Receiver balance mismatch: expected %s, got %s", expectedReceiverBalance.String(), finalReceiverBalance.String())

	s.T().Logf("✅ IBC transfer completed successfully!")
	s.T().Logf("   Deducted %s %s from %s", transferAmount.String(), chainAConfig.Denom, chainA.GetChainID())
	s.T().Logf("   Minted %s %s on %s", transferAmount.String(), ibcDenom, chainB.GetChainID())
}

// getBalance queries the balance of an address for a specific denom
func (s *DockerTestSuite) getBalance(ctx context.Context, chain types.Chain, address sdk.AccAddress, denom string) sdkmath.Int {
	// Get the first node to create a client context
	dockerChain, ok := chain.(*Chain)
	if !ok {
		s.T().Logf("Chain is not a docker Chain, returning zero balance")
		return sdkmath.ZeroInt()
	}

	node := dockerChain.GetNode()
	clientCtx := node.CliContext()

	// Create bank query client
	bankClient := banktypes.NewQueryClient(clientCtx)

	// Query the balance
	balanceReq := &banktypes.QueryBalanceRequest{
		Address: address.String(),
		Denom:   denom,
	}

	resp, err := bankClient.Balance(ctx, balanceReq)
	if err != nil {
		s.T().Logf("Failed to query balance for %s denom %s: %v", address.String(), denom, err)
		return sdkmath.ZeroInt()
	}

	if resp.Balance == nil {
		return sdkmath.ZeroInt()
	}

	return resp.Balance.Amount
}

// calculateIBCDenom calculates the IBC denomination for a token transferred over IBC
func (s *DockerTestSuite) calculateIBCDenom(portID, channelID, baseDenom string) string {
	prefixedDenom := ibctransfertypes.GetPrefixedDenom(
		portID,
		channelID,
		baseDenom,
	)
	return ibctransfertypes.ParseDenomTrace(prefixedDenom).IBCDenom()
}

/*
	chainBIBCToken := testsuite.GetIBCToken(chainADenom, channelA.Counterparty.PortID, channelA.Counterparty.ChannelID)

	t.Run("packets relayed", func(t *testing.T) {
		s.AssertPacketRelayed(ctx, chainA, channelA.PortID, channelA.ChannelID, 1)
		actualBalance, err := query.Balance(ctx, chainB, chainBAddress, chainBIBCToken.IBCDenom())

		s.Require().NoError(err)
		s.Require().Equal(testvalues.IBCTransferAmount, actualBalance.Int64())
	})
*/
