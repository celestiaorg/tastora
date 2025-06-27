package docker

// ConfigOption is a function that modifies a Config
type ConfigOption func(*Config)

// WithPerNodeConfig adds per-node configuration to the chain config
func WithPerNodeConfig(nodeConfigs map[int]*ChainNodeConfig) ConfigOption {
	return func(cfg *Config) {
		if cfg.ChainConfig == nil {
			cfg.ChainConfig = &ChainConfig{}
		}
		cfg.ChainConfig.ChainNodeConfigs = nodeConfigs
	}
}

// WithNumValidators sets the number of validators to start the chain with.
func WithNumValidators(numValidators int) ConfigOption {
	return func(cfg *Config) {
		if cfg.ChainConfig == nil {
			cfg.ChainConfig = &ChainConfig{}
		}
		cfg.ChainConfig.NumValidators = &numValidators
	}
}

// WithChainImage sets the default chain image
func WithChainImage(image DockerImage) ConfigOption {
	return func(cfg *Config) {
		if cfg.ChainConfig == nil {
			cfg.ChainConfig = &ChainConfig{}
		}
		cfg.ChainConfig.Images = []DockerImage{image}
	}
}

// WithAdditionalStartArgs sets chain-level additional start arguments
func WithAdditionalStartArgs(args ...string) ConfigOption {
	return func(cfg *Config) {
		if cfg.ChainConfig == nil {
			cfg.ChainConfig = &ChainConfig{}
		}
		cfg.ChainConfig.AdditionalStartArgs = args
	}
}

// WithPerBridgeNodeConfig adds per-bridge-node configuration to the DA network config
func WithPerBridgeNodeConfig(nodeConfigs map[int]*DANodeConfig) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}
		cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs = nodeConfigs
	}
}

// WithPerFullNodeConfig adds per-full-node configuration to the DA network config
func WithPerFullNodeConfig(nodeConfigs map[int]*DANodeConfig) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}
		cfg.DataAvailabilityNetworkConfig.FullNodeConfigs = nodeConfigs
	}
}

// WithPerLightNodeConfig adds per-light-node configuration to the DA network config
func WithPerLightNodeConfig(nodeConfigs map[int]*DANodeConfig) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}
		cfg.DataAvailabilityNetworkConfig.LightNodeConfigs = nodeConfigs
	}
}

// SIMPLE: One-liner to configure all DA node ports
func WithDANodePorts(rpcPort, p2pPort string) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}
		cfg.DataAvailabilityNetworkConfig.DefaultRPCPort = rpcPort
		cfg.DataAvailabilityNetworkConfig.DefaultP2PPort = p2pPort
	}
}

// SIMPLE: One-liner to configure core connection ports
func WithDANodeCoreConnection(rpcPort, grpcPort string) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}
		cfg.DataAvailabilityNetworkConfig.DefaultCoreRPCPort = rpcPort
		cfg.DataAvailabilityNetworkConfig.DefaultCoreGRPCPort = grpcPort
	}
}

// SIMPLE: Per-node-type configuration for bridge nodes
func WithBridgeNodePorts(nodeIndex int, rpcPort, p2pPort string) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}
		if cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs == nil {
			cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs = make(map[int]*DANodeConfig)
		}
		if cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[nodeIndex] == nil {
			cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[nodeIndex] = &DANodeConfig{}
		}
		cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[nodeIndex].RPCPort = rpcPort
		cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[nodeIndex].P2PPort = p2pPort
	}
}

// SIMPLE: Per-node-type configuration for full nodes
func WithFullNodePorts(nodeIndex int, rpcPort, p2pPort string) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}
		if cfg.DataAvailabilityNetworkConfig.FullNodeConfigs == nil {
			cfg.DataAvailabilityNetworkConfig.FullNodeConfigs = make(map[int]*DANodeConfig)
		}
		if cfg.DataAvailabilityNetworkConfig.FullNodeConfigs[nodeIndex] == nil {
			cfg.DataAvailabilityNetworkConfig.FullNodeConfigs[nodeIndex] = &DANodeConfig{}
		}
		cfg.DataAvailabilityNetworkConfig.FullNodeConfigs[nodeIndex].RPCPort = rpcPort
		cfg.DataAvailabilityNetworkConfig.FullNodeConfigs[nodeIndex].P2PPort = p2pPort
	}
}

// SIMPLE: Per-node-type configuration for light nodes
func WithLightNodePorts(nodeIndex int, rpcPort, p2pPort string) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}
		if cfg.DataAvailabilityNetworkConfig.LightNodeConfigs == nil {
			cfg.DataAvailabilityNetworkConfig.LightNodeConfigs = make(map[int]*DANodeConfig)
		}
		if cfg.DataAvailabilityNetworkConfig.LightNodeConfigs[nodeIndex] == nil {
			cfg.DataAvailabilityNetworkConfig.LightNodeConfigs[nodeIndex] = &DANodeConfig{}
		}
		cfg.DataAvailabilityNetworkConfig.LightNodeConfigs[nodeIndex].RPCPort = rpcPort
		cfg.DataAvailabilityNetworkConfig.LightNodeConfigs[nodeIndex].P2PPort = p2pPort
	}
}

// SIMPLE: Common scenarios helper - uses non-conflicting ports
func WithNonConflictingPorts() ConfigOption {
	return func(cfg *Config) {
		// Apply DA node ports
		WithDANodePorts("26668", "2131")(cfg)
		// Apply core connection ports
		WithDANodeCoreConnection("26667", "9091")(cfg)
	}
}
