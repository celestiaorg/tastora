package dataavailability

import (
	"context"
	"fmt"
	"testing"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// NetworkBuilder defines a builder for configuring and initializing a da Network for testing purposes
type NetworkBuilder struct {
	// t is the testing context used for test assertions, container naming, and test lifecycle management
	t *testing.T
	// testName is the unique test identifier used for Docker resource naming in parallel execution
	testName string
	// nodes is the array of node configurations that define the network topology and individual node settings
	nodes []NodeConfig
	// dockerClient is the Docker client instance used for all container operations
	dockerClient *client.Client
	// dockerNetworkID is the ID of the Docker network where all Nodes are deployed
	dockerNetworkID string
	// logger is the structured logger for network operations and debugging. Defaults to test logger.
	logger *zap.Logger
	// dockerImage is the default Docker image configuration for all nodes in the network (can be overridden per node)
	dockerImage *container.Image
	// additionalStartArgs are the default additional command-line arguments for all nodes in the network
	additionalStartArgs []string
	// env are the default environment variables for all nodes in the network
	env []string
	// chainID is the Chain ID for network identification (e.g., "test-chain")
	chainID string
	// binaryName is the name of the Node binary executable (e.g., "celestia")
	binaryName string
}

// NewNetworkBuilder initializes and returns a new NetworkBuilder with default values for testing purposes
func NewNetworkBuilder(t *testing.T) *NetworkBuilder {
	return NewNetworkBuilderWithTestName(t, t.Name())
}

// NewNetworkBuilderWithTestName initializes and returns a new NetworkBuilder with a custom test name
func NewNetworkBuilderWithTestName(t *testing.T, testName string) *NetworkBuilder {
	t.Helper()
	return &NetworkBuilder{
		t:                   t,
		testName:            testName,
		logger:              zaptest.NewLogger(t),
		chainID:             "test-chain",
		binaryName:          "celestia",
		additionalStartArgs: make([]string, 0),
		env:                 make([]string, 0),
	}
}

// NewNetworkBuilderFromNetwork creates a NetworkBuilder from an existing Network
// This allows reusing the builder's node creation logic for dynamic node addition
func NewNetworkBuilderFromNetwork(network *Network) *NetworkBuilder {
	return &NetworkBuilder{
		testName:            network.testName,
		logger:              network.log,
		dockerClient:        network.cfg.DockerClient,
		dockerNetworkID:     network.cfg.DockerNetworkID,
		dockerImage:         &network.cfg.Image,
		additionalStartArgs: network.cfg.Env,
		env:                 network.cfg.Env,
		chainID:             network.cfg.ChainID,
		binaryName:          network.cfg.Bin,
	}
}

// WithTestName sets the test name
func (b *NetworkBuilder) WithTestName(testName string) *NetworkBuilder {
	b.testName = testName
	return b
}

// WithLogger sets the logger
func (b *NetworkBuilder) WithLogger(logger *zap.Logger) *NetworkBuilder {
	b.logger = logger
	return b
}

// WithChainID sets the chain ID
func (b *NetworkBuilder) WithChainID(chainID string) *NetworkBuilder {
	b.chainID = chainID
	return b
}

// WithBinaryName sets the binary name
func (b *NetworkBuilder) WithBinaryName(binaryName string) *NetworkBuilder {
	b.binaryName = binaryName
	return b
}

// WithDockerClient sets the Docker client
func (b *NetworkBuilder) WithDockerClient(client *client.Client) *NetworkBuilder {
	b.dockerClient = client
	return b
}

// WithDockerNetworkID sets the Docker network ID
func (b *NetworkBuilder) WithDockerNetworkID(networkID string) *NetworkBuilder {
	b.dockerNetworkID = networkID
	return b
}

// WithImage sets the default Docker image for all nodes in the network
func (b *NetworkBuilder) WithImage(image container.Image) *NetworkBuilder {
	b.dockerImage = &image
	return b
}

// WithAdditionalStartArgs sets the default additional start arguments for all nodes in the network
func (b *NetworkBuilder) WithAdditionalStartArgs(args ...string) *NetworkBuilder {
	b.additionalStartArgs = args
	return b
}

// WithEnv sets the default environment variables for all nodes in the network
func (b *NetworkBuilder) WithEnv(env ...string) *NetworkBuilder {
	b.env = env
	return b
}

// WithNode adds a node configuration to the network
func (b *NetworkBuilder) WithNode(config NodeConfig) *NetworkBuilder {
	b.nodes = append(b.nodes, config)
	return b
}

// WithNodes adds multiple node configurations
func (b *NetworkBuilder) WithNodes(nodeConfigs ...NodeConfig) *NetworkBuilder {
	b.nodes = nodeConfigs
	return b
}

// Build creates and returns a new Network instance
func (b *NetworkBuilder) Build(ctx context.Context) (*Network, error) {
	if err := b.validate(); err != nil {
		return nil, fmt.Errorf("invalid builder configuration: %w", err)
	}

	nodeMap, err := b.initializeNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Nodes: %w", err)
	}

	return &Network{
		cfg: Config{
			Logger:          b.logger,
			DockerClient:    b.dockerClient,
			DockerNetworkID: b.dockerNetworkID,
			ChainID:         b.chainID,
			Env:             b.env,
			Bin:             b.binaryName,
			Image:           *b.dockerImage,
		},
		log:         b.logger,
		nodeMap:     nodeMap,
		nextNodeIdx: len(nodeMap), // start from the number of initial nodes
		testName:    b.testName,   // store original test name for dynamic node naming
	}, nil
}

// validate checks the builder configuration for common errors
func (b *NetworkBuilder) validate() error {
	if b.dockerImage == nil {
		return fmt.Errorf("docker image must be specified")
	}
	if b.dockerClient == nil {
		return fmt.Errorf("docker client must be specified")
	}
	if b.dockerNetworkID == "" {
		return fmt.Errorf("docker network ID must be specified")
	}
	if len(b.nodes) == 0 {
		return fmt.Errorf("at least one node configuration must be specified")
	}
	if b.chainID == "" {
		return fmt.Errorf("chain ID must be specified")
	}
	if b.binaryName == "" {
		return fmt.Errorf("binary name must be specified")
	}
	return nil
}

func (b *NetworkBuilder) initializeNodes(ctx context.Context) (map[string]*Node, error) {
	nodeMap := make(map[string]*Node)

	for i, nodeConfig := range b.nodes {
		node, err := b.newNode(ctx, nodeConfig, i)
		if err != nil {
			return nil, fmt.Errorf("failed to create Node %d: %w", i, err)
		}
		nodeMap[node.Name()] = node
	}

	return nodeMap, nil
}

func (b *NetworkBuilder) newNode(ctx context.Context, nodeConfig NodeConfig, index int) (*Node, error) {
	// Get the appropriate image using fallback logic
	imageToUse := b.getImage(nodeConfig)

	cfg := Config{
		Logger:          b.logger,
		DockerClient:    b.dockerClient,
		DockerNetworkID: b.dockerNetworkID,
		ChainID:         b.chainID,
		Env:             b.env, // Will be overridden by nodeConfig.Env after fallback logic below
		Bin:             b.binaryName,
		Image:           imageToUse,
	}

	// Apply fallback logic for node config
	if len(nodeConfig.AdditionalStartArgs) == 0 {
		nodeConfig.AdditionalStartArgs = b.additionalStartArgs
	}
	if len(nodeConfig.Env) == 0 {
		nodeConfig.Env = b.env
	}

	node := NewNode(cfg, b.testName, imageToUse, index, nodeConfig)

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
// otherwise falling back to the network's default image
func (b *NetworkBuilder) getImage(nodeConfig NodeConfig) container.Image {
	if nodeConfig.Image != nil {
		return *nodeConfig.Image
	}
	if b.dockerImage != nil {
		return *b.dockerImage
	}
	panic("no image specified: neither node-specific nor network default image provided")
}
