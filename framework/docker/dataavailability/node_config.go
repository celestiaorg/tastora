package dataavailability

import (
	"context"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
)

// NodeConfig defines the configuration for a single dataavailability Node
type NodeConfig struct {
	// NodeType specifies the type of DA node (bridge, light, full)
	NodeType NodeType
	// Image overrides the network's default image for this specific node (optional)
	Image *container.Image
	// AdditionalStartArgs overrides the network-level AdditionalStartArgs for this specific node
	AdditionalStartArgs []string
	// Env overrides the network-level Env for this specific node
	Env []string
	// ConfigModifications specifies modifications to be applied to config files
	ConfigModifications map[string]toml.Toml
	// CustomPorts allows overriding default port configuration
	CustomPorts *PortConfig
	// postInit functions are executed sequentially after the node is initialized
	postInit []func(ctx context.Context, node *Node) error
}

// PortConfig allows customization of node ports
type PortConfig struct {
	RPCPort      string // Internal RPC port (default: "26658")
	P2PPort      string // Internal P2P port (default: "2121")  
	CoreRPCPort  string // Port to connect to celestia-app RPC (default: "26657")
	CoreGRPCPort string // Port to connect to celestia-app GRPC (default: "9090")
}

// NodeBuilder provides a fluent interface for building NodeConfig
type NodeBuilder struct {
	config *NodeConfig
}

// NewNodeBuilder creates a new NodeBuilder with bridge node as default
func NewNodeBuilder() *NodeBuilder {
	return &NodeBuilder{
		config: &NodeConfig{
			NodeType:            BridgeNodeType,
			AdditionalStartArgs: make([]string, 0),
			Env:                 make([]string, 0),
			ConfigModifications: make(map[string]toml.Toml),
		},
	}
}

// WithNodeType sets the node type (bridge, light, full)
func (b *NodeBuilder) WithNodeType(nodeType NodeType) *NodeBuilder {
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

// WithCustomPorts sets custom port configuration
func (b *NodeBuilder) WithCustomPorts(ports *PortConfig) *NodeBuilder {
	b.config.CustomPorts = ports
	return b
}

// WithPorts sets custom ports using individual values
func (b *NodeBuilder) WithPorts(rpcPort, p2pPort, coreRPCPort, coreGRPCPort string) *NodeBuilder {
	b.config.CustomPorts = &PortConfig{
		RPCPort:      rpcPort,
		P2PPort:      p2pPort,
		CoreRPCPort:  coreRPCPort,
		CoreGRPCPort: coreGRPCPort,
	}
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