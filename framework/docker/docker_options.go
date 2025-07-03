package docker

import "github.com/celestiaorg/tastora/framework/types"

// ConfigOption is a function that modifies a Config
type ConfigOption func(*Config)

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

// WithDANodePorts allows the configuration of default RPC and P2P ports for all DA nodes
func WithDANodePorts(rpcPort, p2pPort string) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}
		cfg.DataAvailabilityNetworkConfig.DefaultRPCPort = rpcPort
		cfg.DataAvailabilityNetworkConfig.DefaultP2PPort = p2pPort
	}
}

// WithDANodeCoreConnection allows the configuration of RPC and GRPC ports for connecting to the celestia-app core
func WithDANodeCoreConnection(rpcPort, grpcPort string) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}
		cfg.DataAvailabilityNetworkConfig.DefaultCoreRPCPort = rpcPort
		cfg.DataAvailabilityNetworkConfig.DefaultCoreGRPCPort = grpcPort
	}
}

// WithNodePorts allows the configuration of rpcPort and p2pPort for a node of the specified type at the specified index
func WithNodePorts(nodeType types.DANodeType, nodeIndex int, rpcPort, p2pPort string) ConfigOption {
	return func(cfg *Config) {
		if cfg.DataAvailabilityNetworkConfig == nil {
			cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{}
		}

		switch nodeType {
		case types.BridgeNode:
			if cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs == nil {
				cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs = make(map[int]*DANodeConfig)
			}
			if cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[nodeIndex] == nil {
				cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[nodeIndex] = &DANodeConfig{}
			}
			cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[nodeIndex].RPCPort = rpcPort
			cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[nodeIndex].P2PPort = p2pPort

		case types.FullNode:
			if cfg.DataAvailabilityNetworkConfig.FullNodeConfigs == nil {
				cfg.DataAvailabilityNetworkConfig.FullNodeConfigs = make(map[int]*DANodeConfig)
			}
			if cfg.DataAvailabilityNetworkConfig.FullNodeConfigs[nodeIndex] == nil {
				cfg.DataAvailabilityNetworkConfig.FullNodeConfigs[nodeIndex] = &DANodeConfig{}
			}
			cfg.DataAvailabilityNetworkConfig.FullNodeConfigs[nodeIndex].RPCPort = rpcPort
			cfg.DataAvailabilityNetworkConfig.FullNodeConfigs[nodeIndex].P2PPort = p2pPort

		case types.LightNode:
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
}

// WithDefaultPorts configures the default ports for DA nodes
func WithDefaultPorts() ConfigOption {
	return func(cfg *Config) {
		// Apply DA node ports
		WithDANodePorts("26668", "2131")(cfg)
		// Apply core connection ports
		WithDANodeCoreConnection("26667", "9091")(cfg)
	}
}
