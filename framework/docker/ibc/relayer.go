package ibc

import (
	"context"

	"github.com/celestiaorg/tastora/framework/types"
)

// Relayer interface defines the operations for an IBC relayer.
type Relayer interface {
	// Start starts the relayer.
	Start(ctx context.Context, args ...string) error

	// Stop stops the relayer.
	Stop(ctx context.Context) error

	// Init initializes the relayer configuration
	Init(ctx context.Context, chainA, chainB types.Chain) error

	// SetupWallets creates and funds relayer wallets on both chains
	SetupWallets(ctx context.Context, chainA, chainB types.Chain) error

	// CreateClients creates IBC clients on both chains.
	CreateClients(ctx context.Context, chainA, chainB types.Chain) error

	// CreateConnections creates IBC connections between the chains.
	CreateConnections(ctx context.Context, chainA, chainB types.Chain) (Connection, error)

	// CreateChannel creates an IBC channel between the chains.
	CreateChannel(ctx context.Context, chainA, chainB types.Chain, connection Connection, opts CreateChannelOptions) (*Channel, error)

	// AddWallet adds a wallet for the specified chain ID to the relayer configuration.
	AddWallet(chainID string, wallet types.Wallet) error

	// AddChain adds a chain to the relayer configuration.
	AddChain(chain types.Chain) error
}

// CreateChannelOptions defines options for creating an IBC channel.
type CreateChannelOptions struct {
	SourcePortName string
	DestPortName   string
	Order          ChannelOrder
	Version        string
}

// ChannelOrder represents the ordering of an IBC channel.
type ChannelOrder string

const (
	OrderOrdered   ChannelOrder = "ordered"
	OrderUnordered ChannelOrder = "unordered"
)