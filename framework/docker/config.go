package docker

import (
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
}

type ChainConfig struct {
	// Chain type, e.g. cosmos.
	Type string `yaml:"type"`
	// Chain name, e.g. cosmoshub.
	Name string `yaml:"name"`
	// Version of the docker image to use.
	// Must be set.
	Version string
	// How many validators and how many full nodes to use when instantiating the chain.
	NumValidators, NumFullNodes *int
	// Chain ID, e.g. cosmoshub-4
	ChainID string
	// Docker images required for running chain nodes.
	Images []DockerImage
	// Binary to execute for the chain node daemon.
	Bin string `yaml:"bin"`
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
	// Do not use docker host mount.
	NoHostMount bool
	// When provided, genesis file contents will be altered before sharing for genesis.
	ModifyGenesis func(Config, []byte) ([]byte, error)
	// Override config parameters for files at filepath.
	ConfigFileOverrides map[string]any
	// Non-nil will override the encoding config, used for cosmos chains only.
	EncodingConfig *testutil.TestEncodingConfig
	// To avoid port binding conflicts, ports are only exposed on the 0th validator.
	HostPortOverride map[int]int
	// ExposeAdditionalPorts exposes each port id to the host on a random port. ex: "8080/tcp"
	// Access the address with ChainNode.GetHostAddress
	ExposeAdditionalPorts []string
	// Additional start command arguments
	AdditionalStartArgs []string
	// Environment variables for chain nodes
	Env []string
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
}
