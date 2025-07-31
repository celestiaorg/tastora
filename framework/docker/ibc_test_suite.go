package docker

import (
	"context"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/ibc"
	"github.com/celestiaorg/tastora/framework/docker/ibc/relayer"
	"github.com/celestiaorg/tastora/framework/testutil/config"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/moby/moby/client"
	"golang.org/x/sync/errgroup"

	servercfg "github.com/cosmos/cosmos-sdk/server/config"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/ibc-go/v8/modules/apps/transfer"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// IBCTestSuite is a test suite for IBC functionality between two chains
type IBCTestSuite struct {
	suite.Suite
	ctx          context.Context
	dockerClient *client.Client
	networkID    string
	logger       *zap.Logger
	encConfig    testutil.TestEncodingConfig

	// IBC-specific components
	chainA     types.Chain // celestia-app chain
	chainB     types.Chain // simapp chain
	relayer    *relayer.Hermes
	connection ibc.Connection
	channel    ibc.Channel
}

// SetupSuite runs once before all tests in the suite
func (s *IBCTestSuite) SetupSuite() {
	s.ctx = context.Background()
	s.logger = zaptest.NewLogger(s.T())
	s.encConfig = testutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{}, transfer.AppModuleBasic{})
}

// SetupTest sets up IBC infrastructure for each test
func (s *IBCTestSuite) SetupTest() {
	s.dockerClient, s.networkID = DockerSetup(s.T())

	// Create celestia-app chain (chain A)
	var err error
	s.chainA, err = s.createCelestiaChain()
	s.Require().NoError(err, "failed to create celestia chain")

	// Create simapp chain (chain B)
	s.chainB, err = s.createSimappChain()
	s.Require().NoError(err, "failed to create simapp chain")

	// Start both chains in parallel
	eg, egCtx := errgroup.WithContext(s.ctx)
	eg.Go(func() error {
		return s.chainA.Start(egCtx)
	})
	eg.Go(func() error {
		return s.chainB.Start(egCtx)
	})
	s.Require().NoError(eg.Wait(), "failed to start chains")

	// Create and initialize relayer (but don't start it)
	s.relayer, err = relayer.NewHermes(s.ctx, s.dockerClient, s.T().Name(), s.networkID, 0, s.logger)
	s.Require().NoError(err, "failed to create hermes relayer")

	err = s.relayer.Init(s.ctx, s.chainA, s.chainB)
	s.Require().NoError(err, "failed to initialize relayer")

	// Setup IBC connection and channel
	s.connection, s.channel = s.setupIBCConnection()
}

// createCelestiaChain creates a celestia-app chain for IBC testing
func (s *IBCTestSuite) createCelestiaChain() (types.Chain, error) {
	// Configure global SDK for celestia prefix during chain setup
	sdkConf := sdk.GetConfig()
	sdkConf.SetBech32PrefixForAccount("celestia", "celestiapub")

	builder := NewChainBuilder(s.T()).
		WithDockerClient(s.dockerClient).
		WithDockerNetworkID(s.networkID).
		WithChainID("chain-a").
		WithName("celestia").
		WithImage(container.NewImage("ghcr.io/celestiaorg/celestia-app", "v5.0.1-rc1", "10001:10001")).
		WithBinaryName("celestia-appd").
		WithBech32Prefix("celestia").
		WithDenom("utia").
		WithGasPrices("0.000001utia").
		WithEncodingConfig(&s.encConfig).
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
			"--timeout-commit", "1s",
		).
		WithPostInit(func(ctx context.Context, node *ChainNode) error {
			return config.Modify(ctx, node, "config/app.toml", func(cfg *servercfg.Config) {
				cfg.MinGasPrices = "0.000001utia"
				cfg.GRPC.Enable = true
				cfg.GRPC.Address = "0.0.0.0:9090"
				cfg.API.Enable = true
				cfg.MinGasPrices = "0.0utia"
			})
		}).
		WithNode(NewChainNodeConfigBuilder().Build())

	return builder.Build(s.ctx)
}

// createSimappChain creates an IBC-Go simapp chain for IBC testing
func (s *IBCTestSuite) createSimappChain() (types.Chain, error) {
	builder := NewChainBuilder(s.T()).
		WithDockerClient(s.dockerClient).
		WithDockerNetworkID(s.networkID).
		WithChainID("chain-b").
		WithName("simapp").
		// use the simapp from ibc-go as a simple app with basic wiring and no token filters.
		WithImage(container.NewImage("ghcr.io/cosmos/ibc-go-simd", "v8.5.0", "1000:1000")).
		WithBinaryName("simd").
		WithBech32Prefix("cosmos").
		WithDenom("stake").
		WithGasPrices("0.000001stake").
		WithEncodingConfig(&s.encConfig).
		WithPostInit(func(ctx context.Context, node *ChainNode) error {
			return config.Modify(ctx, node, "config/app.toml", func(cfg *servercfg.Config) {
				cfg.MinGasPrices = "0.000001stake"
				cfg.GRPC.Enable = true
				cfg.API.Enable = true
				cfg.API.EnableUnsafeCORS = true
			})
		}).
		WithNode(NewChainNodeConfigBuilder().Build())

	return builder.Build(s.ctx)
}

// setupIBCConnection establishes a complete IBC connection and channel
func (s *IBCTestSuite) setupIBCConnection() (ibc.Connection, ibc.Channel) {
	// create clients
	err := s.relayer.CreateClients(s.ctx, s.chainA, s.chainB)
	s.Require().NoError(err)

	// create connections
	connection, err := s.relayer.CreateConnections(s.ctx, s.chainA, s.chainB)
	s.Require().NoError(err)
	s.Require().NotEmpty(connection.ConnectionID, "Connection ID should not be empty")

	// Create an ICS20 channel for token transfers
	channelOpts := ibc.CreateChannelOptions{
		SourcePortName: "transfer",
		DestPortName:   "transfer",
		Order:          ibc.OrderUnordered,
		Version:        "ics20-1",
	}

	channel, err := s.relayer.CreateChannel(s.ctx, s.chainA, connection, channelOpts)
	s.Require().NoError(err)
	s.Require().NotNil(channel)
	s.Require().NotEmpty(channel.ChannelID, "Channel ID should not be empty")

	s.T().Logf("Created IBC connection: %s <-> %s", connection.ConnectionID, connection.CounterpartyID)
	s.T().Logf("Created IBC channel: %s <-> %s", channel.ChannelID, channel.CounterpartyID)

	return connection, channel
}

// TearDownTest cleans up docker resources
func (s *IBCTestSuite) TearDownTest() {
	DockerCleanup(s.T(), s.dockerClient)()
}
