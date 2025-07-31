package docker

import (
	"context"
	sdkmath "cosmossdk.io/math"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	"github.com/stretchr/testify/suite"
	"testing"
	"time"
)

// TestIBCTransfer tests a complete IBC token transfer between celestia-app and simapp
func (s *IBCTestSuite) TestIBCTransfer() {
	ctx := s.ctx
	// send from faucet wallet on chain A
	senderWallet := s.chainA.GetFaucetWallet()

	// receive on faucet wallet on chain B
	receiverWallet := s.chainB.GetFaucetWallet()

	s.T().Logf("IBC sender wallet: %s", senderWallet.GetFormattedAddress())
	s.T().Logf("IBC receiver wallet: %s", receiverWallet.GetFormattedAddress())

	receiverAddr, err := sdkacc.AddressFromWallet(receiverWallet)
	s.Require().NoError(err)

	senderAddr, err := sdkacc.AddressFromWallet(senderWallet)
	s.Require().NoError(err)

	// Calculate the IBC denom for chainA's token on chainB
	ibcDenom := s.calculateIBCDenom(s.channel.CounterpartyPort, s.channel.CounterpartyID, s.chainA.GetRelayerConfig().Denom)
	initialReceiverBalance := s.getBalance(ctx, s.chainB, receiverAddr, ibcDenom)
	s.T().Logf("Receiver initial IBC balance: %s %s", initialReceiverBalance.String(), ibcDenom)

	// Send IBC transfer
	transferAmount := sdkmath.NewInt(100000) // 0.1 tokens
	s.T().Logf("Sending IBC transfer: %s %s from %s to %s", transferAmount.String(), s.chainA.GetRelayerConfig().Denom, s.chainA.GetChainID(), s.chainB.GetChainID())

	s.T().Logf("Starting Hermes relayer...")
	err = s.relayer.Start(ctx)
	s.Require().NoError(err)

	ibcTransfer := ibctransfertypes.NewMsgTransfer(
		s.channel.PortID,
		s.channel.ChannelID,
		sdk.NewCoin(s.chainA.GetRelayerConfig().Denom, transferAmount),
		senderWallet.GetFormattedAddress(),
		receiverAddr.String(),
		clienttypes.ZeroHeight(),
		uint64(time.Now().Add(time.Hour).UnixNano()),
		"",
	)

	resp, err := s.chainA.BroadcastMessages(ctx, senderWallet, ibcTransfer)
	s.Require().NoError(err)
	s.Require().Equal(uint32(0), resp.Code, "IBC transfer failed: %s", resp.RawLog)

	// Wait a moment for the escrow transaction to be reflected in balances
	s.T().Logf("Waiting for balance updates...")
	err = wait.ForBlocks(ctx, 5, s.chainA, s.chainB)
	s.Require().NoError(err)

	intermediateReceiverBalance := s.getBalance(ctx, s.chainB, receiverAddr, ibcDenom)
	s.T().Logf("Receiver balance before relay: %s %s", intermediateReceiverBalance.String(), ibcDenom)

	// Wait for relayer to process transfer
	s.T().Logf("Waiting for relayer to process transfer...")
	err = wait.ForBlocks(ctx, 10, s.chainA, s.chainB)
	s.Require().NoError(err)

	// Check final balances
	s.T().Logf("Checking final balances...")
	finalSenderBalance := s.getBalance(ctx, s.chainA, senderAddr, s.chainA.GetRelayerConfig().Denom)
	finalReceiverBalance := s.getBalance(ctx, s.chainB, receiverAddr, ibcDenom)

	s.T().Logf("Sender final balance: %s %s", finalSenderBalance.String(), s.chainA.GetRelayerConfig().Denom)
	s.T().Logf("Receiver final IBC balance: %s %s", finalReceiverBalance.String(), ibcDenom)

	// Verify final balances
	// Receiver should have received the transferred tokens
	expectedReceiverBalance := initialReceiverBalance.Add(transferAmount)
	s.Require().True(finalReceiverBalance.Equal(expectedReceiverBalance),
		"Receiver balance mismatch: expected %s, got %s", expectedReceiverBalance.String(), finalReceiverBalance.String())

	s.T().Logf("IBC transfer completed successfully!")
}

// getBalance queries the balance of an address for a specific denom
func (s *IBCTestSuite) getBalance(ctx context.Context, chain types.Chain, address sdk.AccAddress, denom string) sdkmath.Int {
	// Get the first node to create a client context
	dockerChain, ok := chain.(*Chain)
	if !ok {
		s.T().Logf("Chain is not a docker Chain, returning zero balance")
		return sdkmath.ZeroInt()
	}

	// Get chain config for bech32 prefix
	chainConfig := chain.GetRelayerConfig()

	reset := internal.TemporarilyModifySDKConfigPrefix(chainConfig.Bech32Prefix)
	defer reset()

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
func (s *IBCTestSuite) calculateIBCDenom(portID, channelID, baseDenom string) string {
	prefixedDenom := ibctransfertypes.GetPrefixedDenom(
		portID,
		channelID,
		baseDenom,
	)
	return ibctransfertypes.ParseDenomTrace(prefixedDenom).IBCDenom()
}

func TestIBCSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	suite.Run(t, new(IBCTestSuite))
}
