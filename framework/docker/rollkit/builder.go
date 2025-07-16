package rollkit

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"testing"
)

type NodeConfig struct {
	Image               *container.Image
	AdditionalStartArgs []string
	Env                 []string
}

type NodeConfigBuilder struct {
	config *NodeConfig
}

func NewNodeConfigBuilder() *NodeConfigBuilder {
	return &NodeConfigBuilder{
		config: &NodeConfig{
			AdditionalStartArgs: make([]string, 0),
			Env:                 make([]string, 0),
		},
	}
}

func (b *NodeConfigBuilder) WithImage(image container.Image) *NodeConfigBuilder {
	b.config.Image = &image
	return b
}

func (b *NodeConfigBuilder) WithAdditionalStartArgs(args ...string) *NodeConfigBuilder {
	b.config.AdditionalStartArgs = args
	return b
}

func (b *NodeConfigBuilder) WithEnvVars(envVars ...string) *NodeConfigBuilder {
	b.config.Env = envVars
	return b
}

func (b *NodeConfigBuilder) Build() NodeConfig {
	return *b.config
}

type ChainBuilder struct {
	t                    *testing.T
	nodes                []NodeConfig
	dockerClient         *client.Client
	dockerNetworkID      string
	name                 string
	chainID              string
	binaryName           string
	aggregatorPassphrase string
	logger               *zap.Logger
	dockerImage          *container.Image
	additionalStartArgs  []string
	env                  []string
}

func NewChainBuilder(t *testing.T) *ChainBuilder {
	t.Helper()
	return &ChainBuilder{
		t:                    t,
		name:                 "rollkit",
		chainID:              "test-rollkit",
		binaryName:           "rollkit",
		aggregatorPassphrase: "",
		logger:               zaptest.NewLogger(t),
		nodes:                make([]NodeConfig, 0),
		additionalStartArgs:  make([]string, 0),
		env:                  make([]string, 0),
	}
}

func (b *ChainBuilder) WithName(name string) *ChainBuilder {
	b.name = name
	return b
}

func (b *ChainBuilder) WithChainID(chainID string) *ChainBuilder {
	b.chainID = chainID
	return b
}

func (b *ChainBuilder) WithBinaryName(binaryName string) *ChainBuilder {
	b.binaryName = binaryName
	return b
}

func (b *ChainBuilder) WithAggregatorPassphrase(passphrase string) *ChainBuilder {
	b.aggregatorPassphrase = passphrase
	return b
}

func (b *ChainBuilder) WithT(t *testing.T) *ChainBuilder {
	t.Helper()
	b.t = t
	return b
}

func (b *ChainBuilder) WithLogger(logger *zap.Logger) *ChainBuilder {
	b.logger = logger
	return b
}

func (b *ChainBuilder) WithNode(config NodeConfig) *ChainBuilder {
	b.nodes = append(b.nodes, config)
	return b
}

func (b *ChainBuilder) WithNodes(nodeConfigs ...NodeConfig) *ChainBuilder {
	b.nodes = nodeConfigs
	return b
}

func (b *ChainBuilder) WithDockerClient(client *client.Client) *ChainBuilder {
	b.dockerClient = client
	return b
}

func (b *ChainBuilder) WithDockerNetworkID(networkID string) *ChainBuilder {
	b.dockerNetworkID = networkID
	return b
}

func (b *ChainBuilder) WithImage(image container.Image) *ChainBuilder {
	b.dockerImage = &image
	return b
}

func (b *ChainBuilder) WithAdditionalStartArgs(args ...string) *ChainBuilder {
	b.additionalStartArgs = args
	return b
}

func (b *ChainBuilder) WithEnv(env ...string) *ChainBuilder {
	b.env = env
	return b
}

func (b *ChainBuilder) Build(ctx context.Context) (*Chain, error) {
	if len(b.nodes) == 0 {
		b.nodes = append(b.nodes, NodeConfig{})
	}

	nodes, err := b.initializeNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize rollkit nodes: %w", err)
	}

	return &Chain{
		name:                 b.name,
		chainID:              b.chainID,
		binaryName:           b.binaryName,
		aggregatorPassphrase: b.aggregatorPassphrase,
		dockerClient:         b.dockerClient,
		dockerNetworkID:      b.dockerNetworkID,
		log:                  b.logger,
		nodes:                nodes,
	}, nil
}

func (b *ChainBuilder) initializeNodes(ctx context.Context) ([]*Node, error) {
	var nodes []*Node

	for i, nodeConfig := range b.nodes {
		node, err := b.newNode(ctx, nodeConfig, i)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

func (b *ChainBuilder) newNode(ctx context.Context, nodeConfig NodeConfig, index int) (*Node, error) {
	image := b.getImage(nodeConfig)
	
	node := NewNode(b.dockerClient, b.dockerNetworkID, b.logger, b.t.Name(), image, index, b.chainID, b.binaryName, b.aggregatorPassphrase)

	if err := node.CreateAndSetupVolume(ctx, node.Name()); err != nil {
		return nil, err
	}

	return node, nil
}

func (b *ChainBuilder) getImage(nodeConfig NodeConfig) container.Image {
	if nodeConfig.Image != nil {
		return *nodeConfig.Image
	}
	if b.dockerImage != nil {
		return *b.dockerImage
	}
	panic("no image specified: neither node-specific nor chain default image provided")
}