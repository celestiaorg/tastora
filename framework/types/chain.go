package types

import (
	"context"

	"github.com/celestiaorg/go-square/v2/share"

	rpcclient "github.com/cometbft/cometbft/rpc/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ChainRelayerConfig contains all values required to populate a relayer config.
type ChainRelayerConfig struct {
	ChainID      string
	Denom        string
	GasPrices    string
	Bech32Prefix string
	RPCAddress   string
	GRPCAddress  string
}

type Chain interface {
	NetworkInfoProvider
	// Height returns the current height of the chain.
	Height(ctx context.Context) (int64, error)
	// Start starts the chain.
	Start(ctx context.Context) error
	// Stop stops the chain.
	Stop(ctx context.Context) error
	// GetVolumeName is a docker specific field, it is the name of the docker volume the chain nodes are mounted to.
	GetVolumeName() string
	// GetNodes returns a slice of ChainNodes.
	GetNodes() []ChainNode
	// CreateWallet creates a new wallet with the specified keyName and returns the Wallet instance or an error.
	CreateWallet(ctx context.Context, keyName string) (*Wallet, error)
	// BroadcastMessages sends multiple messages to the blockchain network using the signingWallet, returning a transaction response.
	BroadcastMessages(ctx context.Context, signingWallet *Wallet, msgs ...sdk.Msg) (sdk.TxResponse, error)
	// BroadcastBlobMessage broadcasts a transaction that includes a message and associated blobs to the blockchain.
	BroadcastBlobMessage(ctx context.Context, signingWallet *Wallet, msg sdk.Msg, blobs ...*share.Blob) (sdk.TxResponse, error)
	// UpgradeVersion upgrades the chain to the specified version.
	UpgradeVersion(ctx context.Context, version string) error
	// GetFaucetWallet returns the faucet wallet.
	GetFaucetWallet() *Wallet
	// GetChainID returns the chain ID.
	GetChainID() string
	// GetRelayerConfig returns the chain configuration.
	GetRelayerConfig() ChainRelayerConfig
}

type ChainNode interface {
	NetworkInfoProvider
	// GetType returns if the node is a fullnode or a validator. NodeTypeConsensusFull or NodeTypeValidator
	GetType() ConsensusNodeType
	// GetRPCClient retrieves the RPC client associated with the chain node, returning the client instance or an error.
	GetRPCClient() (rpcclient.Client, error)
	// ReadFile reads the contents of a file specified by a relative filePath and returns its byte data or an error on failure.
	ReadFile(ctx context.Context, filePath string) ([]byte, error)
	// WriteFile writes the provided byte data to the specified relative filePath. An error is returned if the write operation fails.
	WriteFile(ctx context.Context, filePath string, data []byte) error
	// GetKeyring returns the keyring for this chain node.
	GetKeyring() (keyring.Keyring, error)
	// Exec executes a command in the specified context with the given environment variables, returning stdout, stderr, and an error.
	Exec(ctx context.Context, cmd []string, env []string) ([]byte, []byte, error)
}
