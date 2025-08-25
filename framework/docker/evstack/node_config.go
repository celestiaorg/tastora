package evstack

import (
	"context"

	"github.com/celestiaorg/tastora/framework/docker/container"
)

// NodeConfig defines the configuration for a single evstack Node
type NodeConfig struct {
	// IsAggregator specifies whether this node should act as an aggregator
	IsAggregator bool
	// Image overrides the chain's default image for this specific node (optional)
	Image *container.Image
	// AdditionalStartArgs overrides the chain-level AdditionalStartArgs for this specific node
	AdditionalStartArgs []string
	// Env overrides the chain-level Env for this specific node
	Env []string
	// postInit functions are executed sequentially after the node is initialized
	postInit []func(ctx context.Context, node *Node) error
}

// NodeBuilder provides a fluent interface for building NodeConfig
type NodeBuilder struct {
	config *NodeConfig
}

// NewNodeBuilder creates a new NodeBuilder
func NewNodeBuilder() *NodeBuilder {
	return &NodeBuilder{
		config: &NodeConfig{
			IsAggregator:        false,
			AdditionalStartArgs: make([]string, 0),
			Env:                 make([]string, 0),
		},
	}
}

// WithAggregator sets whether this node should be an aggregator
func (b *NodeBuilder) WithAggregator(isAggregator bool) *NodeBuilder {
	b.config.IsAggregator = isAggregator
	return b
}

// WithImage sets the Docker image for the node (overrides chain default)
func (b *NodeBuilder) WithImage(image container.Image) *NodeBuilder {
	b.config.Image = &image
	return b
}

// WithAdditionalStartArgs sets the additional start arguments
func (b *NodeBuilder) WithAdditionalStartArgs(args ...string) *NodeBuilder {
	b.config.AdditionalStartArgs = args
	return b
}

// WithEnvVars sets the environment variables
func (b *NodeBuilder) WithEnvVars(envVars ...string) *NodeBuilder {
	b.config.Env = envVars
	return b
}

// WithPostInit sets the post init functions
func (b *NodeBuilder) WithPostInit(postInitFns ...func(ctx context.Context, node *Node) error) *NodeBuilder {
	b.config.postInit = append(b.config.postInit, postInitFns...)
	return b
}

// Build returns the configured NodeConfig
func (b *NodeBuilder) Build() NodeConfig {
	return *b.config
}
