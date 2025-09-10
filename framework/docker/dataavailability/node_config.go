package dataavailability

import (
	"context"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/types"
)

// NodeConfig defines the configuration for a single da Node
type NodeConfig struct {
	// NodeType specifies the type of DA node (bridge, light, full)
	NodeType types.DANodeType
	// Image overrides the network's default image for this specific node (optional)
	Image *container.Image
	// AdditionalStartArgs overrides the network-level AdditionalStartArgs for this specific node
	AdditionalStartArgs []string
	// Env overrides the network-level Env for this specific node
	Env []string
	// ConfigModifications specifies modifications to be applied to config files
	ConfigModifications map[string]toml.Toml
	// InternalPorts allows overriding default port configuration
	InternalPorts types.Ports
	// postInit functions are executed sequentially after the node is initialized
	postInit []func(ctx context.Context, node *Node) error
}

// NodeBuilder provides a fluent interface for building NodeConfig
type NodeBuilder struct {
	config *NodeConfig
}

// NewNodeBuilder creates a new NodeBuilder with bridge node as default
func NewNodeBuilder() *NodeBuilder {
	return &NodeBuilder{
		config: &NodeConfig{
			NodeType:            types.BridgeNode,
			AdditionalStartArgs: make([]string, 0),
			Env:                 make([]string, 0),
			ConfigModifications: make(map[string]toml.Toml),
		},
	}
}

// WithNodeType sets the node type (bridge, light, full)
func (b *NodeBuilder) WithNodeType(nodeType types.DANodeType) *NodeBuilder {
	b.config.NodeType = nodeType
	return b
}

// WithImage sets the Docker image for the node (overrides network default)
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

// WithConfigModifications sets the config file modifications
func (b *NodeBuilder) WithConfigModifications(modifications map[string]toml.Toml) *NodeBuilder {
	b.config.ConfigModifications = modifications
	return b
}

// WithInternalPorts sets custom internal port configuration
func (b *NodeBuilder) WithInternalPorts(ports types.Ports) *NodeBuilder {
	b.config.InternalPorts = ports
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
