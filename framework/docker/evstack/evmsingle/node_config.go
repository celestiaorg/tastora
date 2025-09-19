package evmsingle

import (
    "context"

    "github.com/celestiaorg/tastora/framework/docker/container"
)

// NodeConfig defines per-node options for ev-node-evm-single
type NodeConfig struct {
    // Image overrides chain-level image
    Image container.Image

    // Optional env overrides (if empty and RethNode is set, they are derived)
    EVMEngineURL   string
    EVMETHURL      string
    EVMJWTSecret   string
    EVMGenesisHash string
    EVMBlockTime   string
    EVMSignerPassphrase string

    // Optional DA related env
    DAAddress   string
    DAAuthToken string
    DANamespace string

    // AdditionalStartArgs are appended to the entrypoint's default flags
    AdditionalStartArgs []string
    // AdditionalInitArgs are appended to the `init` command for flexibility
    AdditionalInitArgs []string

    // PostStart allows custom hooks after start
    PostStart []func(ctx context.Context, n *Node) error
}

// NodeConfigBuilder provides a fluent builder for NodeConfig
type NodeConfigBuilder struct{ cfg *NodeConfig }

func NewNodeConfigBuilder() *NodeConfigBuilder { return &NodeConfigBuilder{cfg: &NodeConfig{}} }

func (b *NodeConfigBuilder) WithEVMEngineURL(v string) *NodeConfigBuilder {
    b.cfg.EVMEngineURL = v
    return b
}

func (b *NodeConfigBuilder) WithEVMETHURL(v string) *NodeConfigBuilder {
    b.cfg.EVMETHURL = v
    return b
}

func (b *NodeConfigBuilder) WithEVMJWTSecret(v string) *NodeConfigBuilder {
    b.cfg.EVMJWTSecret = v
    return b
}

func (b *NodeConfigBuilder) WithEVMGenesisHash(v string) *NodeConfigBuilder {
    b.cfg.EVMGenesisHash = v
    return b
}

func (b *NodeConfigBuilder) WithEVMBlockTime(v string) *NodeConfigBuilder {
    b.cfg.EVMBlockTime = v
    return b
}

func (b *NodeConfigBuilder) WithEVMSignerPassphrase(v string) *NodeConfigBuilder {
    b.cfg.EVMSignerPassphrase = v
    return b
}

func (b *NodeConfigBuilder) WithDAAddress(v string) *NodeConfigBuilder {
    b.cfg.DAAddress = v
    return b
}

func (b *NodeConfigBuilder) WithDAAuthToken(v string) *NodeConfigBuilder {
    b.cfg.DAAuthToken = v
    return b
}

func (b *NodeConfigBuilder) WithDANamespace(v string) *NodeConfigBuilder {
    b.cfg.DANamespace = v
    return b
}

func (b *NodeConfigBuilder) WithAdditionalStartArgs(args ...string) *NodeConfigBuilder {
    b.cfg.AdditionalStartArgs = args
    return b
}

// WithAdditionalInitArgs appends extra flags to the `init` command.
func (b *NodeConfigBuilder) WithAdditionalInitArgs(args ...string) *NodeConfigBuilder {
    b.cfg.AdditionalInitArgs = args
    return b
}

func (b *NodeConfigBuilder) WithPostStart(hooks ...func(ctx context.Context, n *Node) error) *NodeConfigBuilder {
    b.cfg.PostStart = hooks
    return b
}
func (b *NodeConfigBuilder) Build() NodeConfig { return *b.cfg }
