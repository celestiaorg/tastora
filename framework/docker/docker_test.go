package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	reth "github.com/celestiaorg/tastora/framework/docker/evstack/reth"
	"github.com/celestiaorg/tastora/framework/testutil/random"
	"github.com/celestiaorg/tastora/framework/types"
	govmodule "github.com/cosmos/cosmos-sdk/x/gov"
	"github.com/moby/moby/client"
	"sync"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/cosmos/ibc-go/v8/modules/apps/transfer"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

var sdkConfigOnce sync.Once

// configureBech32Prefix configures the SDK's Bech32 prefix once globally
func configureBech32Prefix() {
	sdkConf := sdk.GetConfig()
	sdkConf.SetBech32PrefixForAccount("celestia", "celestiapub")
}

// configureBech32PrefixOnce ensures the SDK configuration is set up once for all tests
func configureBech32PrefixOnce() {
	sdkConfigOnce.Do(configureBech32Prefix)
}

// TestSetupConfig contains all the components needed for a Docker test
type TestSetupConfig struct {
	DockerClient          *client.Client
	NetworkID             string
	TestName              string
	Logger                *zap.Logger
	EncConfig             testutil.TestEncodingConfig
	ChainBuilder          *cosmos.ChainBuilder
	DANetworkBuilder      *da.NetworkBuilder
	RethBuilder           *reth.NodeBuilder
	EVMSingleChainBuilder *evmsingle.ChainBuilder
	Chain                 *cosmos.Chain
	Ctx                   context.Context
}

// setupDockerTest creates an isolated Docker test environment
func setupDockerTest(t *testing.T) *TestSetupConfig {
	t.Helper()

	// ensure Bech32 prefix is configured once globally
	configureBech32PrefixOnce()

	// generate unique test name for parallel execution
	uniqueTestName := fmt.Sprintf("%s-%s", t.Name(), random.LowerCaseLetterString(8))

	ctx := context.Background()
	dockerClient, networkID := DockerSetup(t)

	// Override the default cleanup to use our unique test name
	t.Cleanup(DockerCleanupWithTestName(t, dockerClient, uniqueTestName))

	logger := zaptest.NewLogger(t)
	encConfig := testutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{}, transfer.AppModuleBasic{}, govmodule.AppModuleBasic{})

	defaultImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-app",
		Version:    "v5.0.10",
		UIDGID:     "10001:10001",
	}

	builder := cosmos.NewChainBuilderWithTestName(t, uniqueTestName).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID).
		WithImage(defaultImage).
		WithEncodingConfig(&encConfig).
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--grpc.enable",
			"--grpc.address", "0.0.0.0:9090",
			"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
			"--timeout-commit", "1s",
			"--minimum-gas-prices", "0utia",
		).
		WithNode(cosmos.NewChainNodeConfigBuilder().Build())

	// default image for the DA network
	defaultDAImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-node",
		Version:    "v0.26.4",
		UIDGID:     "10001:10001",
	}

	// create node configurations for each type
	bridgeNodeConfig := da.NewNodeBuilder().
		WithNodeType(types.BridgeNode).
		Build()

	fullNodeConfig := da.NewNodeBuilder().
		WithNodeType(types.FullNode).
		Build()

	lightNodeConfig := da.NewNodeBuilder().
		WithNodeType(types.LightNode).
		Build()

	daNetworkBuilder := da.NewNetworkBuilderWithTestName(t, uniqueTestName).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID).
		WithImage(defaultDAImage).
		WithNodes(bridgeNodeConfig, lightNodeConfig, fullNodeConfig)

	// Pre-configured Reth single-node builder with a default evolve genesis
	rethBuilder := reth.NewNodeBuilderWithTestName(t, uniqueTestName).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID).
		WithGenesis([]byte(reth.DefaultEvolveGenesisJSON()))

	// Pre-configured evm-single chain builder; tests add nodes and wiring
	evSingleBuilder := evmsingle.NewChainBuilderWithTestName(t, uniqueTestName).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID)

	return &TestSetupConfig{
		DockerClient:          dockerClient,
		NetworkID:             networkID,
		TestName:              uniqueTestName,
		Logger:                logger,
		EncConfig:             encConfig,
		ChainBuilder:          builder,
		DANetworkBuilder:      daNetworkBuilder,
		RethBuilder:           rethBuilder,
		EVMSingleChainBuilder: evSingleBuilder,
		Ctx:                   ctx,
	}
}

// getGenesisHash returns the genesis hash of the given chain node.
func getGenesisHash(ctx context.Context, chain *cosmos.Chain) (string, error) {
	node := chain.GetNodes()[0]
	c, err := node.GetRPCClient()
	if err != nil {
		return "", fmt.Errorf("failed to get node client: %v", err)
	}

	first := int64(1)
	block, err := c.Block(ctx, &first)
	if err != nil {
		return "", fmt.Errorf("failed to get block: %v", err)
	}

	genesisHash := block.Block.Header.Hash().String()
	if genesisHash == "" {
		return "", fmt.Errorf("genesis hash is empty")
	}
	return genesisHash, nil
}

// TestPerNodeDifferentImages tests that nodes can be deployed with different Docker images
func TestPerNodeDifferentImages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	alternativeImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-app",
		Version:    "5.0.9", // different version from default
		UIDGID:     "10001:10001",
	}

	// create node configs with different images and settings
	validator0Config := cosmos.NewChainNodeConfigBuilder().
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--grpc.enable",
			"--grpc.address", "0.0.0.0:9090",
			"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
			"--timeout-commit", "1s",
			"--log_level", "info",
		).
		Build() // uses default image

	validator1Config := cosmos.NewChainNodeConfigBuilder().
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
	chain, err := testCfg.ChainBuilder.
		WithNodes(validator0Config, validator1Config).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	validatorNodes := chain.GetNodes()
	require.Len(t, validatorNodes, 2, "expected 2 validators")

	// verify both validators are accessible
	for i, node := range validatorNodes {
		client, err := node.GetRPCClient()
		require.NoError(t, err, "node %d should have accessible RPC client", i)

		status, err := client.Status(testCfg.Ctx)
		require.NoError(t, err, "node %d should return status", i)
		require.NotNil(t, status, "node %d status should not be nil", i)

		t.Logf("Node %d is running with chain ID: %s", i, status.NodeInfo.Network)
	}
}

// TestChainNodeExec tests the Exec method on ChainNode
func TestChainNodeExec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	chain, err := testCfg.ChainBuilder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	nodes := chain.GetNodes()
	require.NotEmpty(t, nodes, "chain should have nodes")

	node := nodes[0]

	// test executing a simple command
	cmd := []string{"echo", "hello world"}

	stdout, stderr, err := node.Exec(testCfg.Ctx, cmd, nil)
	require.NoError(t, err, "Exec should succeed")
	require.Contains(t, string(stdout), "hello world", "stdout should contain expected output")
	require.Empty(t, stderr, "stderr should be empty for successful echo command")

	// test executing a command with environment variables
	cmd = []string{"sh", "-c", "echo $TEST_VAR"}
	env := []string{"TEST_VAR=test_value"}

	stdout, stderr, err = node.Exec(testCfg.Ctx, cmd, env)
	require.NoError(t, err, "Exec with env vars should succeed")
	require.Contains(t, string(stdout), "test_value", "stdout should contain env var value")
	require.Empty(t, stderr, "stderr should be empty for successful command")

	// test executing a command that outputs to stderr
	cmd = []string{"sh", "-c", "echo 'error message' >&2"}

	stdout, stderr, err = node.Exec(testCfg.Ctx, cmd, nil)
	require.NoError(t, err, "Exec with stderr output should succeed")
	require.Empty(t, stdout, "stdout should be empty")
	require.Contains(t, string(stderr), "error message", "stderr should contain expected output")

	// test executing a command that returns an error
	cmd = []string{"sh", "-c", "exit 1"}

	stdout, stderr, err = node.Exec(testCfg.Ctx, cmd, nil)
	require.Error(t, err, "Exec with failing command should return error")
	require.Empty(t, stdout, "stdout should be empty for failing command")
	require.Empty(t, stderr, "stderr should be empty for failing command")
}

// TestCustomPortConfigurations tests that chains work correctly with custom port configurations
func TestCustomPortConfigurations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	// setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	// create node configuration with custom ports
	customPortsConfig := cosmos.NewChainNodeConfigBuilder().
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--rpc.laddr=tcp://0.0.0.0:26757",  // custom RPC port
			"--grpc.address=0.0.0.0:9091",      // custom GRPC port
			"--api.address=tcp://0.0.0.0:1318", // custom API port
			"--p2p.laddr=tcp://0.0.0.0:26658",  // custom P2P port
			"--timeout-commit=1s",
			"--minimum-gas-prices=0utia",
		).
		Build()

	// build and start chain with custom ports
	chain, err := testCfg.ChainBuilder.
		WithNodes(customPortsConfig).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	// verify chain started successfully and RPC client works
	nodes := chain.GetNodes()
	require.Len(t, nodes, 1, "expected 1 node")

	node := nodes[0].(*cosmos.ChainNode)

	// test height fetching
	height, err := node.Height(testCfg.Ctx)
	require.NoError(t, err, "should be able to fetch height with custom ports")
	require.Greater(t, height, int64(0), "height should be greater than 0")

	// verify network info returns correct custom ports
	networkInfo, err := node.GetNetworkInfo(testCfg.Ctx)
	require.NoError(t, err, "should get network info")

	// verify internal ports match our custom configuration
	require.Equal(t, "26757", networkInfo.Internal.Ports.RPC, "internal RPC port should be custom port")
	require.Equal(t, "9091", networkInfo.Internal.Ports.GRPC, "internal GRPC port should be custom port")
	require.Equal(t, "1318", networkInfo.Internal.Ports.API, "internal API port should be custom port")
	require.Equal(t, "26658", networkInfo.Internal.Ports.P2P, "internal P2P port should be custom port")
}
