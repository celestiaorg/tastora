package ibc

import (
	"context"
	sdkmath "cosmossdk.io/math"
	"fmt"

	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// Connector orchestrates IBC connections between two chains.
type Connector struct {
	chainA  types.Chain
	chainB  types.Chain
	relayer Relayer

	// Relayer wallets (created during setup)
	relayerWalletA types.Wallet
	relayerWalletB types.Wallet

	// Connection state
	connected bool
	channels  map[string]*Channel
}

// NewConnector creates a new Connector for connecting two chains via a relayer.
func NewConnector(chainA, chainB types.Chain, relayer Relayer) *Connector {
	return &Connector{
		chainA:   chainA,
		chainB:   chainB,
		relayer:  relayer,
		channels: make(map[string]*Channel),
	}
}

// SetupRelayerWallets creates and funds relayer wallets on both chains.
func (c *Connector) SetupRelayerWallets(ctx context.Context) error {
	// Create relayer wallet on chain A
	walletNameA := fmt.Sprintf("relayer-%s", c.chainA.GetChainID())
	relayerWalletA, err := c.chainA.CreateWallet(ctx, walletNameA)
	if err != nil {
		return fmt.Errorf("failed to create relayer wallet on chain A: %w", err)
	}

	// Create relayer wallet on chain B
	walletNameB := fmt.Sprintf("relayer-%s", c.chainB.GetChainID())
	relayerWalletB, err := c.chainB.CreateWallet(ctx, walletNameB)
	if err != nil {
		return fmt.Errorf("failed to create relayer wallet on chain B: %w", err)
	}

	// Fund both wallets from faucets
	err = c.fundRelayerWallet(ctx, c.chainA, relayerWalletA)
	if err != nil {
		return fmt.Errorf("failed to fund relayer wallet on chain A: %w", err)
	}

	err = c.fundRelayerWallet(ctx, c.chainB, relayerWalletB)
	if err != nil {
		return fmt.Errorf("failed to fund relayer wallet on chain B: %w", err)
	}

	// Configure relayer with wallets and chains
	if err := c.relayer.AddChain(c.chainA); err != nil {
		return fmt.Errorf("failed to add chain A to relayer: %w", err)
	}

	if err := c.relayer.AddChain(c.chainB); err != nil {
		return fmt.Errorf("failed to add chain B to relayer: %w", err)
	}

	if err := c.relayer.AddWallet(c.chainA.GetChainID(), relayerWalletA); err != nil {
		return fmt.Errorf("failed to add chain A wallet to relayer: %w", err)
	}

	if err := c.relayer.AddWallet(c.chainB.GetChainID(), relayerWalletB); err != nil {
		return fmt.Errorf("failed to add chain B wallet to relayer: %w", err)
	}
	
	// Store for later use
	c.relayerWalletA = relayerWalletA
	c.relayerWalletB = relayerWalletB

	return nil
}

// Connect establishes IBC connection between the two chains.
func (c *Connector) Connect(ctx context.Context) error {
	if c.connected {
		return nil
	}

	// Create clients
	if err := c.relayer.CreateClients(ctx, c.chainA, c.chainB); err != nil {
		return fmt.Errorf("failed to create IBC clients: %w", err)
	}

	// Create connections
	if err := c.relayer.CreateConnections(ctx, c.chainA, c.chainB); err != nil {
		return fmt.Errorf("failed to create IBC connections: %w", err)
	}

	c.connected = true
	return nil
}

// CreateChannel creates an IBC channel between the chains.
func (c *Connector) CreateChannel(ctx context.Context, opts CreateChannelOptions) (*Channel, error) {
	if !c.connected {
		return nil, fmt.Errorf("chains must be connected before creating channels")
	}

	channel, err := c.relayer.CreateChannel(ctx, c.chainA, c.chainB, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create IBC channel: %w", err)
	}

	// Store channel for later reference
	channelKey := fmt.Sprintf("%s-%s", opts.SourcePortName, opts.DestPortName)
	c.channels[channelKey] = channel

	return channel, nil
}

// fundRelayerWallet funds a relayer wallet from a faucet wallet.
func (c *Connector) fundRelayerWallet(ctx context.Context, chain types.Chain, relayerWallet types.Wallet) error {
	// Get the chain's faucet wallet and config
	faucet := chain.GetFaucetWallet()
	chainConfig := chain.GetChainConfig()

	// Get addresses from wallets
	fromAddr, err := sdkacc.AddressFromWallet(faucet)
	if err != nil {
		return fmt.Errorf("failed to get faucet address: %w", err)
	}

	toAddr, err := sdkacc.AddressFromWallet(relayerWallet)
	if err != nil {
		return fmt.Errorf("failed to get relayer wallet address: %w", err)
	}

	// Define amount to fund the relayer wallet (enough for relayer operations)
	// Use the chain's native denom from the config
	fundAmount := sdk.NewCoins(sdk.NewCoin(chainConfig.Denom, sdkmath.NewInt(10000000))) // 10 tokens

	// Create bank send message
	bankSend := banktypes.NewMsgSend(fromAddr, toAddr, fundAmount)

	// Broadcast the funding transaction
	resp, err := chain.BroadcastMessages(ctx, faucet, bankSend)
	if err != nil {
		return fmt.Errorf("failed to broadcast funding transaction: %w", err)
	}

	if resp.Code != 0 {
		return fmt.Errorf("funding transaction failed: %s", resp.RawLog)
	}

	return nil
}
