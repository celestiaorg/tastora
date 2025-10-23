package cosmos

import (
	"context"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"go.uber.org/zap"
)

type ChainConfig struct {
	Logger *zap.Logger
	// DockerClient is a Docker client instance used for the tests.
	DockerClient types.TastoraDockerClient
	// DockerNetworkID is the ID of the docker network the nodes are deployed to.
	DockerNetworkID string

	// Chain configuration fields (previously in ChainConfig)
	// Chain name, e.g. celestia.
	Name string
	// How many validators and how many full nodes to use when instantiating the chain.
	NumValidators, NumFullNodes *int
	// Chain ID, e.g. cosmoshub-4
	ChainID string
	// Docker image for running chain nodes.
	Image container.Image
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
	// PostInit defines a set of functions executed after initializing a chain node, allowing custom setups or configurations.
	PostInit []func(ctx context.Context, chainNode *ChainNode) error
	// Non-nil will override the encoding config, used for cosmos chains only.
	EncodingConfig *testutil.TestEncodingConfig
	// Additional start command arguments
	AdditionalStartArgs []string
	// Environment variables for chain nodes
	Env []string
	// GenesisFileBz contains the raw bytes of the genesis file that will be written to config/gensis.json
	GenesisFileBz []byte
}
