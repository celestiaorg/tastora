package docker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/types"
	dockertypes "github.com/moby/moby/api/types"
	dockerclient "github.com/moby/moby/client"

	"github.com/celestiaorg/tastora/framework/testutil/toml"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// DockerTestSuite is a test suite which can be used to perform tests which spin up a docker network.
type DockerTestSuite struct {
	suite.Suite
	ctx          context.Context
	dockerClient *dockerclient.Client
	networkID    string
	logger       *zap.Logger
	encConfig    testutil.TestEncodingConfig
	provider     *Provider
	chain        types.Chain
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
}

// TearDownTest removes docker resources.
func (s *DockerTestSuite) TearDownTest() {
	DockerCleanup(s.T(), s.dockerClient)()
}

// CreateDockerProvider returns a provider with configuration options applied to the default Celestia config.
func (s *DockerTestSuite) CreateDockerProvider(opts ...ConfigOption) *Provider {
	numValidators := 1
	numFullNodes := 0

	cfg := Config{
		Logger:          s.logger,
		DockerClient:    s.dockerClient,
		DockerNetworkID: s.networkID,
		ChainConfig: &ChainConfig{
			ConfigFileOverrides: map[string]any{
				"config/app.toml":    appOverrides(),
				"config/config.toml": configOverrides(),
			},
			Type:          "celestia",
			Name:          "celestia",
			Version:       "v4.0.0-rc6",
			NumValidators: &numValidators,
			NumFullNodes:  &numFullNodes,
			ChainID:       "test",
			Images: []DockerImage{
				{
					Repository: "ghcr.io/celestiaorg/celestia-app",
					Version:    "v4.0.0-rc6",
					UIDGID:     "10001:10001",
				},
			},
			Bin:            "celestia-appd",
			Bech32Prefix:   "celestia",
			Denom:          "utia",
			CoinType:       "118",
			GasPrices:      "0.025utia",
			GasAdjustment:  1.3,
			EncodingConfig: &s.encConfig,
			AdditionalStartArgs: []string{
				"--force-no-bbr",
				"--grpc.enable",
				"--grpc.address",
				"0.0.0.0:9090",
				"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
				"--timeout-commit", "1s", // shorter block time.
			},
		},
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

// enable indexing of transactions so Broadcasting of transactions works.
func appOverrides() toml.Toml {
	tomlCfg := make(toml.Toml)
	txIndex := make(toml.Toml)
	txIndex["indexer"] = "kv"
	tomlCfg["tx-index"] = txIndex
	return tomlCfg
}

func configOverrides() toml.Toml {
	tomlCfg := make(toml.Toml)
	txIndex := make(toml.Toml)
	txIndex["indexer"] = "kv"
	tomlCfg["tx_index"] = txIndex
	return tomlCfg
}

// TestPerNodeDifferentImages tests that nodes can be deployed with different Docker images
func (s *DockerTestSuite) TestPerNodeDifferentImages() {
	defaultImage := DockerImage{
		Repository: "ghcr.io/celestiaorg/celestia-app",
		Version:    "v4.0.0-rc6",
		UIDGID:     "10001:10001",
	}

	alternativeImage := DockerImage{
		Repository: "ghcr.io/celestiaorg/celestia-app",
		Version:    "v4.0.0-rc5", // Different version
		UIDGID:     "10001:10001",
	}

	// set up chain with 2 validators using different images
	numValidators := 2

	var err error
	s.provider = s.CreateDockerProvider(
		WithNumValidators(numValidators),
		WithPerNodeConfig(map[int]*ChainNodeConfig{
			0: {
				Image:               &defaultImage, // first validator uses one image
				AdditionalStartArgs: []string{"--force-no-bbr", "--log_level", "info"},
			},
			1: {
				Image:               &alternativeImage, // second validator uses a different image
				AdditionalStartArgs: []string{"--force-no-bbr", "--log_level", "debug"},
			},
		}),
	)

	s.chain, err = s.provider.GetChain(s.ctx)
	s.Require().NoError(err)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	validatorNodes := s.chain.GetNodes()
	s.Require().Len(validatorNodes, numValidators, "expected 2 validators")

	s.T().Run("TestPerNodeDifferentImages completed", func(t *testing.T) {
		for i, node := range validatorNodes {
			client, err := node.GetRPCClient()
			s.Require().NoError(err, "node %d should have accessible RPC client", i)

			status, err := client.Status(s.ctx)
			s.Require().NoError(err, "node %d should return status", i)
			s.Require().NotNil(status, "node %d status should not be nil", i)

			s.T().Logf("Node %d is running with chain ID: %s", i, status.NodeInfo.Network)
		}
	})
}

// TestChainNodeExecBinInContainer tests the ExecBinInContainer method with a running chain
func (s *DockerTestSuite) TestChainNodeExecBinInContainer() {
	// Skip in short mode
	if testing.Short() {
		s.T().Skip("Skipping TestChainNodeExecBinInContainer in short mode")
	}

	// Start a chain with a validator
	var err error
	s.provider = s.CreateDockerProvider()
	s.chain, err = s.provider.GetChain(s.ctx)
	s.Require().NoError(err)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	validatorNodes := s.chain.GetNodes()
	s.Require().Len(validatorNodes, 1, "expected 1 validator")

	validator := validatorNodes[0]

	s.T().Run("ExecBinInContainer can execute keys list command", func(t *testing.T) {
		// Execute a command that should succeed
		stdout, stderr, err := validator.ExecBinInContainer(s.ctx, "keys", "list", "--keyring-backend", "test")
		s.Require().NoError(err, "ExecBinInContainer should execute successfully")
		s.Require().Contains(string(stdout), "validator")
		s.Require().Empty(stderr)
	})

	s.T().Run("ExecBinInContainer can execute version command", func(t *testing.T) {
		// Execute a command that should succeed
		stdout, stderr, err := validator.ExecBinInContainer(s.ctx, "version")
		s.Require().NoError(err, "ExecBinInContainer should execute version command successfully")
		s.Require().NotEmpty(stdout)
		s.Require().Empty(stderr)
	})

	s.T().Run("ExecBinInContainer handles invalid commands gracefully", func(t *testing.T) {
		// Execute a command that should fail
		_, stderr, err := validator.ExecBinInContainer(s.ctx, "invalid-command")
		s.Require().Error(err, "ExecBinInContainer should return error for invalid command")
		s.Require().NotEmpty(stderr)
	})
}

func TestDockerSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	suite.Run(t, new(DockerTestSuite))
}

func TestExec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx := context.Background()

	// Setup test environment
	testName := fmt.Sprintf("test-exec-%d", time.Now().Unix())
	log := zap.NewNop()
	dockerClient, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv)
	require.NoError(t, err)

	// Create network
	netResp, err := dockerClient.NetworkCreate(ctx, testName, dockertypes.NetworkCreate{
		Labels: map[string]string{consts.CleanupLabel: testName},
	})
	require.NoError(t, err)
	defer func() {
		_ = dockerClient.NetworkRemove(ctx, netResp.ID)
	}()

	// Create a container lifecycle
	containerName := fmt.Sprintf("test-exec-container-%d", time.Now().Unix())
	lifecycle := NewContainerLifecycle(log, dockerClient, containerName)

	// Create container with a command that keeps it running
	err = lifecycle.CreateContainer(
		ctx,
		testName,
		netResp.ID,
		DockerImage{
			Repository: "busybox",
			Version:    "latest",
		},
		nil,        // no ports needed
		"",         // no IP address
		[]string{}, // no volume binds
		nil,        // no mounts
		containerName,
		[]string{"sleep", "300"}, // keep container running for 5 minutes
		[]string{},               // no env vars
		nil,                      // no entrypoint override
	)
	require.NoError(t, err)
	defer func() {
		_ = lifecycle.RemoveContainer(ctx)
	}()

	err = lifecycle.StartContainer(ctx)
	require.NoError(t, err)

	// Create a minimal ChainNode with just enough configuration for Exec to work
	node := &ChainNode{
		ContainerNode: &ContainerNode{
			DockerClient:       dockerClient,
			containerLifecycle: lifecycle,
			logger:             log,
		},
		cfg: Config{
			ChainConfig: &ChainConfig{
				Env: []string{},
			},
		},
	}

	// Test Exec functionality
	stdout, stderr, err := node.Exec(ctx, "echo", "hello world")
	require.NoError(t, err)
	require.Equal(t, "hello world\n", string(stdout))
	require.Empty(t, stderr)

	// Test with a command that produces stderr
	stdout, stderr, err = node.Exec(ctx, "sh", "-c", "echo hello to stderr >&2")
	require.NoError(t, err)
	require.Empty(t, stdout)
	require.Equal(t, "hello to stderr\n", string(stderr))

	// Test with a failing command
	stdout, stderr, err = node.Exec(ctx, "false")
	require.Error(t, err)
	require.Contains(t, err.Error(), "exited with code 1")
	require.Empty(t, stdout)
	require.Empty(t, stderr)
}
