package evmsingle

import (
	"github.com/celestiaorg/tastora/framework/docker/container"
)

// NodeConfig defines per-node options for ev-node-evm-single
type NodeConfig struct {
	// Image overrides chain-level image
	Image container.Image

	// Optional env overrides (if empty and RethNode is set, they are derived)
	EVMEngineURL        string
	EVMETHURL           string
	EVMJWTSecret        string
	EVMGenesisHash      string
	EVMBlockTime        string
	EVMSignerPassphrase string

	// Optional DA related env
	DAAddress   string
	DAAuthToken string
	DANamespace string

	// AdditionalStartArgs are appended to the entrypoint's default flags
	AdditionalStartArgs []string
	// AdditionalInitArgs are appended to the `init` command for flexibility
	AdditionalInitArgs []string
}

// NodeConfigBuilder provides a fluent builder for NodeConfig
type NodeConfigBuilder struct{ cfg *NodeConfig }

func NewNodeConfigBuilder() *NodeConfigBuilder { return &NodeConfigBuilder{cfg: &NodeConfig{}} }

func (b *NodeConfigBuilder) WithEVMEngineURL(engineURL string) *NodeConfigBuilder {
	b.cfg.EVMEngineURL = engineURL
	return b
}

func (b *NodeConfigBuilder) WithEVMETHURL(ethURL string) *NodeConfigBuilder {
	b.cfg.EVMETHURL = ethURL
	return b
}

func (b *NodeConfigBuilder) WithEVMJWTSecret(jwtSecret string) *NodeConfigBuilder {
	b.cfg.EVMJWTSecret = jwtSecret
	return b
}

func (b *NodeConfigBuilder) WithEVMGenesisHash(genesisHash string) *NodeConfigBuilder {
	b.cfg.EVMGenesisHash = genesisHash
	return b
}

func (b *NodeConfigBuilder) WithEVMBlockTime(blockTime string) *NodeConfigBuilder {
	b.cfg.EVMBlockTime = blockTime
	return b
}

func (b *NodeConfigBuilder) WithEVMSignerPassphrase(passphrase string) *NodeConfigBuilder {
	b.cfg.EVMSignerPassphrase = passphrase
	return b
}

func (b *NodeConfigBuilder) WithDAAddress(address string) *NodeConfigBuilder {
	b.cfg.DAAddress = address
	return b
}

func (b *NodeConfigBuilder) WithDAAuthToken(authToken string) *NodeConfigBuilder {
	b.cfg.DAAuthToken = authToken
	return b
}

func (b *NodeConfigBuilder) WithDANamespace(namespace string) *NodeConfigBuilder {
	b.cfg.DANamespace = namespace
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

func (b *NodeConfigBuilder) Build() NodeConfig { return *b.cfg }
