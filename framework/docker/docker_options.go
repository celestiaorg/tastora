package docker

// ConfigOption is a function that modifies a Config
type ConfigOption func(*Config)

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
		cfg.ChainConfig.Image = image
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
