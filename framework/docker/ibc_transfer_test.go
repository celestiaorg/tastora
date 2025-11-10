package docker

import (
	"context"
	sdkmath "cosmossdk.io/math"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	"github.com/celestiaorg/tastora/framework/docker/ibc"
	"github.com/celestiaorg/tastora/framework/docker/ibc/relayer"
	"github.com/celestiaorg/tastora/framework/testutil/query"
	"github.com/celestiaorg/tastora/framework/testutil/random"
	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/ibc-go/v8/modules/apps/transfer"
	ibctransfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
	clienttypes "github.com/cosmos/ibc-go/v8/modules/core/02-client/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"golang.org/x/sync/errgroup"
	"testing"
	"time"
)

// IBCTestSetupConfig contains all the components needed for a Docker IBC test
type IBCTestSetupConfig struct {
	TestSetupConfig

	// IBC-specific components
	chainA     types.Chain // celestia-app chain
	chainB     types.Chain // simapp chain
	relayer    *relayer.Hermes
	connection ibc.Connection
	channel    ibc.Channel
}

func setupIBCDockerTest(t *testing.T) *IBCTestSetupConfig {
	configureBech32PrefixOnce()

	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	encConfig := testutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{}, transfer.AppModuleBasic{})

	// Generate unique test name for parallel execution
	uniqueTestName := fmt.Sprintf("%s-%s", t.Name(), random.LowerCaseLetterString(8))

	dockerClient, networkID := Setup(t)

	// Create celestia-app chain (chain A)
	chainA, err := createCelestiaChain(t, ctx, dockerClient, networkID, encConfig, uniqueTestName)
	require.NoError(t, err, "failed to create celestia chain")

	// Create simapp chain (chain B)
	chainB, err := createSimappChain(t, ctx, dockerClient, networkID, encConfig, uniqueTestName)
	require.NoError(t, err, "failed to create simapp chain")

	// Start both chains in parallel
	eg, egCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return chainA.Start(egCtx)
	})
	eg.Go(func() error {
		return chainB.Start(egCtx)
	})
	require.NoError(t, eg.Wait(), "failed to start chains")

	// Create and initialize relayer (but don't start it)
	hermes, err := relayer.NewHermes(ctx, dockerClient, uniqueTestName, networkID, 0, logger)
	require.NoError(t, err, "failed to create hermes relayer")

	err = hermes.Init(ctx, []types.Chain{chainA, chainB}, func(config *relayer.HermesConfig) {
		// apply a modification that is not the default.
		config.Chains[0].ClockDrift = "6s"
		config.Chains[1].ClockDrift = "6s"
	})

	require.NoError(t, err, "failed to initialize relayer")

	cfg, err := hermes.GetConfig(ctx)
	require.NoError(t, err, "failed to get relayer config")

	require.Equal(t, cfg.Chains[0].ClockDrift, "6s", "clock drift should be 6s")
	require.Equal(t, cfg.Chains[1].ClockDrift, "6s", "clock drift should be 6s")

	// Setup IBC connection and channel
	connection, channel := setupIBCConnection(t, ctx, chainA, chainB, hermes)

	ibcCfg := &IBCTestSetupConfig{
		TestSetupConfig: TestSetupConfig{
			DockerClient: dockerClient,
			NetworkID:    networkID,
			TestName:     uniqueTestName,
			Logger:       logger,
			EncConfig:    encConfig,
			Ctx:          ctx,
		},
		chainA:     chainA,
		chainB:     chainB,
		relayer:    hermes,
		connection: connection,
		channel:    channel,
	}

	return ibcCfg
}

// TestIBCTransfer tests a complete IBC token transfer between celestia-app and simapp
func TestIBCTransfer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	// Setup IBC environment
	ibcCfg := setupIBCDockerTest(t)
	ctx := ibcCfg.Ctx

	// send from faucet wallet on chain A
	senderWallet := ibcCfg.chainA.GetFaucetWallet()

	// receive on faucet wallet on chain B
	receiverWallet := ibcCfg.chainB.GetFaucetWallet()

	t.Logf("IBC sender wallet: %s", senderWallet.GetFormattedAddress())
	t.Logf("IBC receiver wallet: %s", receiverWallet.GetFormattedAddress())

	receiverAddr, err := sdkacc.AddressFromWallet(receiverWallet)
	require.NoError(t, err)

	senderAddr, err := sdkacc.AddressFromWallet(senderWallet)
	require.NoError(t, err)

	// Calculate the IBC denom for chainA's token on chainB
	ibcDenom := calculateIBCDenom(ibcCfg.channel.CounterpartyPort, ibcCfg.channel.CounterpartyID, ibcCfg.chainA.GetRelayerConfig().Denom)
	initialReceiverBalance := getBalance(t, ctx, ibcCfg.chainB, receiverAddr, ibcDenom)
	t.Logf("Receiver initial IBC balance: %s %s", initialReceiverBalance.String(), ibcDenom)

	// Send IBC transfer
	transferAmount := sdkmath.NewInt(100000) // 0.1 tokens
	t.Logf("Sending IBC transfer: %s %s from %s to %s", transferAmount.String(), ibcCfg.chainA.GetRelayerConfig().Denom, ibcCfg.chainA.GetChainID(), ibcCfg.chainB.GetChainID())

	t.Logf("Starting Hermes relayer...")
	err = ibcCfg.relayer.Start(ctx)
	require.NoError(t, err)

	ibcTransfer := ibctransfertypes.NewMsgTransfer(
		ibcCfg.channel.PortID,
		ibcCfg.channel.ChannelID,
		sdk.NewCoin(ibcCfg.chainA.GetRelayerConfig().Denom, transferAmount),
		senderWallet.GetFormattedAddress(),
		receiverAddr.String(),
		clienttypes.ZeroHeight(),
		uint64(time.Now().Add(time.Hour).UnixNano()),
		"",
	)

	resp, err := ibcCfg.chainA.BroadcastMessages(ctx, senderWallet, ibcTransfer)
	require.NoError(t, err)
	require.Equal(t, uint32(0), resp.Code, "IBC transfer failed: %s", resp.RawLog)

	// Wait a moment for the escrow transaction to be reflected in balances
	t.Logf("Waiting for balance updates...")
	err = wait.ForBlocks(ctx, 5, ibcCfg.chainA, ibcCfg.chainB)
	require.NoError(t, err)

	intermediateReceiverBalance := getBalance(t, ctx, ibcCfg.chainB, receiverAddr, ibcDenom)
	t.Logf("Receiver balance before relay: %s %s", intermediateReceiverBalance.String(), ibcDenom)

	// Wait for relayer to process transfer
	t.Logf("Waiting for relayer to process transfer...")
	err = wait.ForBlocks(ctx, 10, ibcCfg.chainA, ibcCfg.chainB)
	require.NoError(t, err)

	// Check final balances
	t.Logf("Checking final balances...")
	finalSenderBalance := getBalance(t, ctx, ibcCfg.chainA, senderAddr, ibcCfg.chainA.GetRelayerConfig().Denom)
	finalReceiverBalance := getBalance(t, ctx, ibcCfg.chainB, receiverAddr, ibcDenom)

	t.Logf("Sender final balance: %s %s", finalSenderBalance.String(), ibcCfg.chainA.GetRelayerConfig().Denom)
	t.Logf("Receiver final IBC balance: %s %s", finalReceiverBalance.String(), ibcDenom)

	// Verify final balances
	// Receiver should have received the transferred tokens
	expectedReceiverBalance := initialReceiverBalance.Add(transferAmount)
	require.True(t, finalReceiverBalance.Equal(expectedReceiverBalance),
		"Receiver balance mismatch: expected %s, got %s", expectedReceiverBalance.String(), finalReceiverBalance.String())

	t.Logf("IBC transfer completed successfully!")
}

// getBalance queries the balance of an address for a specific denom
func getBalance(t *testing.T, ctx context.Context, chain types.Chain, address sdk.AccAddress, denom string) sdkmath.Int {
	// Get the first node to create a client context
	dockerChain, ok := chain.(*cosmos.Chain)
	if !ok {
		t.Logf("Chain is not a docker Chain, returning zero balance")
		return sdkmath.ZeroInt()
	}

	node := dockerChain.GetNode()
	amount, err := query.Balance(ctx, node.GrpcConn, address.String(), denom)
	if err != nil {
		t.Logf("Failed to query balance for %s denom %s: %v", address.String(), denom, err)
		return sdkmath.ZeroInt()
	}
	return amount
}

// calculateIBCDenom calculates the IBC denomination for a token transferred over IBC
func calculateIBCDenom(portID, channelID, baseDenom string) string {
	prefixedDenom := ibctransfertypes.GetPrefixedDenom(
		portID,
		channelID,
		baseDenom,
	)
	return ibctransfertypes.ParseDenomTrace(prefixedDenom).IBCDenom()
}
