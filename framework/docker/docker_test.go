package docker

import (
	"context"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/moby/moby/client"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// DockerTestSuite is a test suite which can be used to perform tests which spin up a docker network.
type DockerTestSuite struct {
	suite.Suite
	ctx          context.Context
	dockerClient *client.Client
	networkID    string
	logger       *zap.Logger
	encConfig    testutil.TestEncodingConfig
	provider     *Provider
	chain        types.Chain
	builder      *ChainBuilder
}

// SetupSuite runs once before all tests in the suite.
func (s *DockerTestSuite) SetupSuite() {
	s.ctx = context.Background()

	// configure Bech32 prefix, this needs to be set as account.String() uses the global config.
	sdkConf := sdk.GetConfig()
	sdkConf.SetBech32PrefixForAccount("celestia", "celestiapub")
	sdkConf.Seal()

	s.logger = zaptest.NewLogger(s.T())
	s.encConfig = testutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{})
}

func (s *DockerTestSuite) SetupTest() {
	s.dockerClient, s.networkID = DockerSetup(s.T())

	defaultImage := DockerImage{
		Repository: "ghcr.io/celestiaorg/celestia-app",
		Version:    "v4.0.0-rc6",
		UIDGID:     "10001:10001",
	}

	// create a build with a default set of args, any individual test can override/modify.
	s.builder = NewChainBuilder(s.T()).
		WithDockerClient(s.dockerClient).
		WithDockerNetworkID(s.networkID).
		WithImage(defaultImage).
		WithEncodingConfig(&s.encConfig).
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--grpc.enable",
			"--grpc.address", "0.0.0.0:9090",
			"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
			"--timeout-commit", "1s",
		).
		WithNode(NewChainNodeConfigBuilder().Build())
}

// TearDownTest removes docker resources.
func (s *DockerTestSuite) TearDownTest() {
	DockerCleanup(s.T(), s.dockerClient)()
}

// CreateDockerProvider returns a provider with configuration options applied to the default Celestia config.
func (s *DockerTestSuite) CreateDockerProvider(opts ...ConfigOption) *Provider {
	cfg := Config{
		Logger:          s.logger,
		DockerClient:    s.dockerClient,
		DockerNetworkID: s.networkID,
		DataAvailabilityNetworkConfig: &DataAvailabilityNetworkConfig{
			FullNodeCount:   1,
			BridgeNodeCount: 1,
			LightNodeCount:  1,
			Image: DockerImage{
				Repository: "ghcr.io/celestiaorg/celestia-node",
				Version:    "pr-4283", // TODO: use tag that includes changes from https://github.com/celestiaorg/celestia-node/pull/4283.
				UIDGID:     "10001:10001",
			},
		},
		RollkitChainConfig: &RollkitChainConfig{
			ChainID:              "test",
			Bin:                  "testapp",
			AggregatorPassphrase: "12345678",
			NumNodes:             1,
			Image: DockerImage{
				Repository: "ghcr.io/rollkit/rollkit",
				Version:    "main",
				UIDGID:     "2000",
			},
		},
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return NewProvider(cfg, s.T())
}

// getGenesisHash returns the genesis hash of the given chain node.
func (s *DockerTestSuite) getGenesisHash(ctx context.Context) string {
	node := s.chain.GetNodes()[0]
	c, err := node.GetRPCClient()
	s.Require().NoError(err, "failed to get node client")

	first := int64(1)
	block, err := c.Block(ctx, &first)
	s.Require().NoError(err, "failed to get block")

	genesisHash := block.Block.Header.Hash().String()
	s.Require().NotEmpty(genesisHash, "genesis hash is empty")
	return genesisHash
}

// TestPerNodeDifferentImages tests that nodes can be deployed with different Docker images
func (s *DockerTestSuite) TestPerNodeDifferentImages() {
	alternativeImage := DockerImage{
		Repository: "ghcr.io/celestiaorg/celestia-app",
		Version:    "v4.0.0-rc5", // different version from default
		UIDGID:     "10001:10001",
	}

	// create node configs with different images and settings
	validator0Config := NewChainNodeConfigBuilder().
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--grpc.enable",
			"--grpc.address", "0.0.0.0:9090",
			"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
			"--timeout-commit", "1s",
			"--log_level", "info",
		).
		Build() // uses default image

	validator1Config := NewChainNodeConfigBuilder().
		WithImage(alternativeImage). // override with different image
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--grpc.enable",
			"--grpc.address", "0.0.0.0:9090",
			"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
			"--timeout-commit", "1s",
			"--log_level", "debug",
		).
		Build()

	// Use builder directly - tests can modify as needed before calling Build
	var err error
	s.chain, err = s.builder.
		WithNodes(validator0Config, validator1Config).
		Build(s.ctx)
	s.Require().NoError(err)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	validatorNodes := s.chain.GetNodes()
	s.Require().Len(validatorNodes, 2, "expected 2 validators")

	// verify both validators are accessible
	for i, node := range validatorNodes {
		client, err := node.GetRPCClient()
		s.Require().NoError(err, "node %d should have accessible RPC client", i)

		status, err := client.Status(s.ctx)
		s.Require().NoError(err, "node %d should return status", i)
		s.Require().NotNil(status, "node %d status should not be nil", i)

		s.T().Logf("Node %d is running with chain ID: %s", i, status.NodeInfo.Network)
	}
}

// TestChainNodeExec tests the Exec method on ChainNode
func (s *DockerTestSuite) TestChainNodeExec() {
	var err error
	s.provider = s.CreateDockerProvider()
	s.chain, err = s.provider.GetChain(s.ctx)
	s.Require().NoError(err)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	nodes := s.chain.GetNodes()
	s.Require().NotEmpty(nodes, "chain should have nodes")

	node := nodes[0]

	// test executing a simple command
	cmd := []string{"echo", "hello world"}

	stdout, stderr, err := node.Exec(s.ctx, cmd, nil)
	s.Require().NoError(err, "Exec should succeed")
	s.Require().Contains(string(stdout), "hello world", "stdout should contain expected output")
	s.Require().Empty(stderr, "stderr should be empty for successful echo command")

	// test executing a command with environment variables
	cmd = []string{"sh", "-c", "echo $TEST_VAR"}
	env := []string{"TEST_VAR=test_value"}

	stdout, stderr, err = node.Exec(s.ctx, cmd, env)
	s.Require().NoError(err, "Exec with env vars should succeed")
	s.Require().Contains(string(stdout), "test_value", "stdout should contain env var value")
	s.Require().Empty(stderr, "stderr should be empty for successful command")

	// test executing a command that outputs to stderr
	cmd = []string{"sh", "-c", "echo 'error message' >&2"}

	stdout, stderr, err = node.Exec(s.ctx, cmd, nil)
	s.Require().NoError(err, "Exec with stderr output should succeed")
	s.Require().Empty(stdout, "stdout should be empty")
	s.Require().Contains(string(stderr), "error message", "stderr should contain expected output")

	// test executing a command that returns an error
	cmd = []string{"sh", "-c", "exit 1"}

	stdout, stderr, err = node.Exec(s.ctx, cmd, nil)
	s.Require().Error(err, "Exec with failing command should return error")
	s.Require().Empty(stdout, "stdout should be empty for failing command")
	s.Require().Empty(stderr, "stderr should be empty for failing command")
}

func TestDockerSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	suite.Run(t, new(DockerTestSuite))
}
