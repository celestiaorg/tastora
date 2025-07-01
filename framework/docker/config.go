package docker

import (
	"context"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

type Config struct {
	Logger *zap.Logger
	// DockerClient is a Docker client instance used for the tests.
	DockerClient *client.Client
	// DockerNetworkID is the ID of the docker network the nodes are deployed to.
	DockerNetworkID string
	// ChainConfig defines configuration specific to the app chain.
	ChainConfig *ChainConfig
	// DataAvailabilityNetworkConfig defines the configuration for the data availability network settings.
	DataAvailabilityNetworkConfig *DataAvailabilityNetworkConfig
	// RollkitChainConfig defines configuration settings specific to a Rollkit-based chain.
	RollkitChainConfig *RollkitChainConfig
}

type ChainConfig struct {
	// Chain name, e.g. celestia.
	Name string
	// How many validators and how many full nodes to use when instantiating the chain.
	NumValidators, NumFullNodes *int
	// Chain ID, e.g. cosmoshub-4
	ChainID string
	// Docker image for running chain nodes.
	Image DockerImage
	// Binary to execute for the chain node daemon.
	Bin string
	// Bech32 prefix for chain addresses, e.g. cosmos.
	Bech32Prefix string
	// Denomination of native currency, e.g. uatom.
	Denom string
	// Coin type
	CoinType string
	// Minimum gas prices for sending transactions, in native currency denom.
	GasPrices string
	// Adjustment multiplier for gas fees.
	GasAdjustment float64
	// Default gas limit for transactions. May be empty, "auto", or a number.
	Gas string
	// Trusting period of the chain.
	TrustingPeriod string
	PostInit       []func(ctx context.Context, chainNode *ChainNode) error
	// Non-nil will override the encoding config, used for cosmos chains only.
	EncodingConfig *testutil.TestEncodingConfig
	// Additional start command arguments
	AdditionalStartArgs []string
	// Environment variables for chain nodes
	Env           []string
	GenesisFileBz []byte
}

// DataAvailabilityNetworkConfig defines the configuration for the data availability network, including node counts and image settings.
type DataAvailabilityNetworkConfig struct {
	// FullNodeCount specifies the number of full nodes to deploy in the data availability network.
	FullNodeCount int
	// BridgeNodeCount specifies the number of bridge nodes to deploy in the data availability network.
	BridgeNodeCount int
	// LightNodeCount specifies the number of light nodes to deploy in the data availability network.
	LightNodeCount int
	// Image specifies the Docker image used for nodes in the data availability network.
	Image DockerImage
	// BridgeNodeConfigs allows per-bridge-node configuration overrides, keyed by bridge node index
	BridgeNodeConfigs map[int]*DANodeConfig
	// FullNodeConfigs allows per-full-node configuration overrides, keyed by full node index
	FullNodeConfigs map[int]*DANodeConfig
	// LightNodeConfigs allows per-light-node configuration overrides, keyed by light node index
	LightNodeConfigs map[int]*DANodeConfig

	// NEW: Default port configuration for all DA nodes
	DefaultRPCPort      string // Default internal RPC port (default: "26658")
	DefaultP2PPort      string // Default internal P2P port (default: "2121")
	DefaultCoreRPCPort  string // Default core RPC port to connect to (default: "26657")
	DefaultCoreGRPCPort string // Default core GRPC port to connect to (default: "9090")
}

// DANodeConfig provides per-node configuration that can override DataAvailabilityNetworkConfig defaults
type DANodeConfig struct {
	// Image overrides the network-level Image for this specific node
	Image *DockerImage

	RPCPort      string // Internal RPC port (overrides default)
	P2PPort      string // Internal P2P port (overrides default)
	CoreRPCPort  string // Port to connect to celestia-app RPC (overrides default)
	CoreGRPCPort string // Port to connect to celestia-app GRPC (overrides default)
}

// RollkitChainConfig defines the configuration for a Rollkit-based chain
// including node counts, image settings, and chainID.
type RollkitChainConfig struct {
	// ChainID, e.g. test-rollkit
	ChainID string
	// Environment variables for chain nodes
	Env []string
	// Binary to execute for the rollkit chain.
	Bin string
	// AggregatorPassphrase is the passphrase used when a node is an aggregator.
	AggregatorPassphrase string
	// NumNodes
	NumNodes int
	// Image specifies the Docker image used for the rollkit nodes.
	Image DockerImage
}
