package types

import (
	"context"
	"github.com/celestiaorg/go-square/v2/share"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type Chain interface {
	// Height returns the current height of the chain.
	Height(ctx context.Context) (int64, error)
	// Start starts the chain.
	Start(ctx context.Context) error
	// Stop stops the chain.
	Stop(ctx context.Context) error
	// GetHostRPCAddress returns the RPC address of the chain resolvable by the test runner.
	GetHostRPCAddress() string
	// GetGRPCAddress returns the internal GRPC address.
	GetGRPCAddress() string
	// GetVolumeName is a docker specific field, it is the name of the docker volume the chain nodes are mounted to.
	GetVolumeName() string // TODO: this should be removed and is a temporary function for docker only PoC.
	// GetNodes returns a slice of ChainNodes.
	GetNodes() []ChainNode
	// AddNode adds a full node to the chain. overrides can be provided to make modifications to any config files before starting.
	AddNode(ctx context.Context, overrides map[string]any) error // TODO: use options pattern to allow for overrides.
	// CreateWallet creates a new wallet with the specified keyName and returns the Wallet instance or an error.
	CreateWallet(ctx context.Context, keyName string) (Wallet, error)
	BroadcastMessages(ctx context.Context, signingWallet Wallet, msgs ...sdk.Msg) (sdk.TxResponse, error)
	BroadcastBlobMessage(ctx context.Context, signingWallet Wallet, msg sdk.Msg, blobs ...*share.Blob) (sdk.TxResponse, error)
}

type ChainNode interface {
	// GetType returns if the node is a fullnode or a validator. "fn" or a "val"
	GetType() string
	GetRPCClient() (rpcclient.Client, error)
	// GetInternalPeerAddress returns the peer address resolvable within the network.
	GetInternalPeerAddress(ctx context.Context) (string, error)
	// GetInternalRPCAddress returns the rpc address resolvable within the network.
	GetInternalRPCAddress(ctx context.Context) (string, error)
	// GetInternalHostName returns the hostname resolvable within the network.
	GetInternalHostName(ctx context.Context) (string, error)
}
