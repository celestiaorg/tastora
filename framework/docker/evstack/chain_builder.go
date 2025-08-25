package evstack

import (
	"context"
	"fmt"
	"testing"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// ChainBuilder defines a builder for configuring and initializing an evstack Chain for testing purposes
type ChainBuilder struct {
	// t is the testing context used for test assertions, container naming, and test lifecycle management
	t *testing.T
	// testName is the unique test identifier used for Docker resource naming in parallel execution
	testName string
	// nodes is the array of node configurations that define the chain topology and individual node settings
	nodes []NodeConfig
	// dockerClient is the Docker client instance used for all container operations
	dockerClient *client.Client
	// dockerNetworkID is the ID of the Docker network where all Nodes are deployed
	dockerNetworkID string
	// logger is the structured logger for chain operations and debugging. Defaults to test logger.
	logger *zap.Logger
	// dockerImage is the default Docker image configuration for all nodes in the chain (can be overridden per node)
	dockerImage *container.Image
	// additionalStartArgs are the default additional command-line arguments for all nodes in the chain
	additionalStartArgs []string
	// env are the default environment variables for all nodes in the chain
	env []string
	// chainID is the Chain ID for network identification (e.g., "test-evstack")
	chainID string
	// binaryName is the name of the Node binary executable (e.g., "testapp")
	binaryName string
	// aggregatorPassphrase is the passphrase used for aggregator nodes
	aggregatorPassphrase string
}

// NewChainBuilder initializes and returns a new ChainBuilder with default values for testing purposes
func NewChainBuilder(t *testing.T) *ChainBuilder {
	return NewChainBuilderWithTestName(t, t.Name())
}

// NewChainBuilderWithTestName initializes and returns a new ChainBuilder with a custom test name
func NewChainBuilderWithTestName(t *testing.T, testName string) *ChainBuilder {
	t.Helper()
	return &ChainBuilder{
		t:                    t,
		testName:             testName,
		logger:               zaptest.NewLogger(t),
		chainID:              "test-evstack",
		binaryName:           "testapp",
		aggregatorPassphrase: "12345678",
		additionalStartArgs:  make([]string, 0),
		env:                  make([]string, 0),
	}
}

// WithTestName sets the test name
func (b *ChainBuilder) WithTestName(testName string) *ChainBuilder {
	b.testName = testName
	return b
}

// WithLogger sets the logger
func (b *ChainBuilder) WithLogger(logger *zap.Logger) *ChainBuilder {
	b.logger = logger
	return b
}

// WithChainID sets the chain ID
func (b *ChainBuilder) WithChainID(chainID string) *ChainBuilder {
	b.chainID = chainID
	return b
}

// WithBinaryName sets the binary name
func (b *ChainBuilder) WithBinaryName(binaryName string) *ChainBuilder {
	b.binaryName = binaryName
	return b
}

// WithAggregatorPassphrase sets the aggregator passphrase
func (b *ChainBuilder) WithAggregatorPassphrase(passphrase string) *ChainBuilder {
	b.aggregatorPassphrase = passphrase
	return b
}

// WithDockerClient sets the Docker client
func (b *ChainBuilder) WithDockerClient(client *client.Client) *ChainBuilder {
	b.dockerClient = client
	return b
}

// WithDockerNetworkID sets the Docker network ID
func (b *ChainBuilder) WithDockerNetworkID(networkID string) *ChainBuilder {
	b.dockerNetworkID = networkID
	return b
}

// WithImage sets the default Docker image for all nodes in the chain
func (b *ChainBuilder) WithImage(image container.Image) *ChainBuilder {
	b.dockerImage = &image
	return b
}

// WithAdditionalStartArgs sets the default additional start arguments for all nodes in the chain
func (b *ChainBuilder) WithAdditionalStartArgs(args ...string) *ChainBuilder {
	b.additionalStartArgs = args
	return b
}

// WithEnv sets the default environment variables for all nodes in the chain
func (b *ChainBuilder) WithEnv(env ...string) *ChainBuilder {
	b.env = env
	return b
}

// WithNode adds a node configuration to the chain
func (b *ChainBuilder) WithNode(config NodeConfig) *ChainBuilder {
	b.nodes = append(b.nodes, config)
	return b
}

// WithNodes adds multiple node configurations
func (b *ChainBuilder) WithNodes(nodeConfigs ...NodeConfig) *ChainBuilder {
	b.nodes = nodeConfigs
	return b
}

// Build creates and returns a new Chain instance
func (b *ChainBuilder) Build(ctx context.Context) (*Chain, error) {
	if b.dockerImage == nil {
		return nil, fmt.Errorf("docker image must be specified")
	}

	nodes, err := b.initializeNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Nodes: %w", err)
	}

	return &Chain{
		cfg: Config{
			Logger:               b.logger,
			DockerClient:         b.dockerClient,
			DockerNetworkID:      b.dockerNetworkID,
			ChainID:              b.chainID,
			Env:                  b.env,
			Bin:                  b.binaryName,
			AggregatorPassphrase: b.aggregatorPassphrase,
			Image:                *b.dockerImage,
		},
		log:   b.logger,
		nodes: nodes,
	}, nil
}

func (b *ChainBuilder) initializeNodes(ctx context.Context) ([]*Node, error) {
	var nodes []*Node

	for i, nodeConfig := range b.nodes {
		node, err := b.newNode(ctx, nodeConfig, i)
		if err != nil {
			return nil, fmt.Errorf("failed to create Node %d: %w", i, err)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (b *ChainBuilder) newNode(ctx context.Context, nodeConfig NodeConfig, index int) (*Node, error) {
	// Get the appropriate image using fallback logic
	imageToUse := b.getImage(nodeConfig)

	cfg := Config{
		Logger:               b.logger,
		DockerClient:         b.dockerClient,
		DockerNetworkID:      b.dockerNetworkID,
		ChainID:              b.chainID,
		Env:                  b.getEnv(nodeConfig),
		Bin:                  b.binaryName,
		AggregatorPassphrase: b.aggregatorPassphrase,
		Image:                imageToUse,
	}

	node := NewNode(cfg, b.testName, imageToUse, index, nodeConfig.IsAggregator, b.getAdditionalStartArgs(nodeConfig))

	// Create and setup volume using shared logic
	if err := node.CreateAndSetupVolume(ctx, node.Name()); err != nil {
		return nil, err
	}

	// Run post-init functions if any
	for _, postInitFn := range nodeConfig.postInit {
		if err := postInitFn(ctx, node); err != nil {
			return nil, fmt.Errorf("post-init failed for node %d: %w", index, err)
		}
	}

	return node, nil
}

// getImage returns the appropriate Docker image for a node, using node-specific override if available,
// otherwise falling back to the chain's default image
func (b *ChainBuilder) getImage(nodeConfig NodeConfig) container.Image {
	if nodeConfig.Image != nil {
		return *nodeConfig.Image
	}
	if b.dockerImage != nil {
		return *b.dockerImage
	}
	panic("no image specified: neither node-specific nor chain default image provided")
}

// getEnv returns the appropriate environment variables for a node
func (b *ChainBuilder) getEnv(nodeConfig NodeConfig) []string {
	if len(nodeConfig.Env) > 0 {
		return nodeConfig.Env
	}
	return b.env
}

// getAdditionalStartArgs returns the appropriate additional start arguments for a node
func (b *ChainBuilder) getAdditionalStartArgs(nodeConfig NodeConfig) []string {
	if len(nodeConfig.AdditionalStartArgs) > 0 {
		return nodeConfig.AdditionalStartArgs
	}
	return b.additionalStartArgs
}
