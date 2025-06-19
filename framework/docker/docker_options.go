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
