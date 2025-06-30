package docker

import (
	"testing"

	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestDANodePortConfiguration(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("default ports when no configuration provided", func(t *testing.T) {
		cfg := Config{
			Logger:                        logger,
			DataAvailabilityNetworkConfig: &DataAvailabilityNetworkConfig{},
		}

		daNode := &DANode{
			cfg: cfg,
		}

		// Test default ports
		require.Equal(t, "26658/tcp", daNode.getRPCPort())
		require.Equal(t, "2121/tcp", daNode.getP2PPort())
		require.Equal(t, "26657", daNode.getCoreRPCPort())
		require.Equal(t, "9090", daNode.getCoreGRPCPort())
	})

	t.Run("network-level port configuration", func(t *testing.T) {
		cfg := Config{
			Logger: logger,
			DataAvailabilityNetworkConfig: &DataAvailabilityNetworkConfig{
				DefaultRPCPort:      "27000",
				DefaultP2PPort:      "3000",
				DefaultCoreRPCPort:  "27001",
				DefaultCoreGRPCPort: "9095",
			},
		}

		daNode := &DANode{
			cfg: cfg,
		}

		// Test network-level custom ports
		require.Equal(t, "27000/tcp", daNode.getRPCPort())
		require.Equal(t, "3000/tcp", daNode.getP2PPort())
		require.Equal(t, "27001", daNode.getCoreRPCPort())
		require.Equal(t, "9095", daNode.getCoreGRPCPort())
	})

	t.Run("per-node port configuration overrides network defaults", func(t *testing.T) {
		cfg := Config{
			Logger: logger,
			DataAvailabilityNetworkConfig: &DataAvailabilityNetworkConfig{
				DefaultRPCPort:      "27000",
				DefaultP2PPort:      "3000",
				DefaultCoreRPCPort:  "27001",
				DefaultCoreGRPCPort: "9095",
				BridgeNodeConfigs: map[int]*DANodeConfig{
					0: {
						RPCPort:      "28000",
						P2PPort:      "4000",
						CoreRPCPort:  "28001",
						CoreGRPCPort: "9096",
					},
				},
			},
		}

		daNode := &DANode{
			cfg: cfg,
			node: &node{
				Index: 0,
			},
		}

		// Test per-node custom ports override network defaults
		require.Equal(t, "28000/tcp", daNode.getRPCPort())
		require.Equal(t, "4000/tcp", daNode.getP2PPort())
		require.Equal(t, "28001", daNode.getCoreRPCPort())
		require.Equal(t, "9096", daNode.getCoreGRPCPort())
	})

	t.Run("port map generation uses configurable ports", func(t *testing.T) {
		cfg := Config{
			Logger: logger,
			DataAvailabilityNetworkConfig: &DataAvailabilityNetworkConfig{
				DefaultRPCPort: "27000",
				DefaultP2PPort: "3000",
			},
		}

		daNode := &DANode{
			cfg: cfg,
		}

		portMap := daNode.getPortMap()

		// Verify that port map contains the custom ports
		require.Contains(t, portMap, nat.Port("27000/tcp"))
		require.Contains(t, portMap, nat.Port("3000/tcp"))
		require.NotContains(t, portMap, nat.Port("26658/tcp")) // Should not contain default RPC port
		require.NotContains(t, portMap, nat.Port("2121/tcp"))  // Should not contain default P2P port
	})

	t.Run("internal address methods use configurable ports", func(t *testing.T) {
		cfg := Config{
			Logger: logger,
			DataAvailabilityNetworkConfig: &DataAvailabilityNetworkConfig{
				DefaultRPCPort: "27000",
				DefaultP2PPort: "3000",
			},
		}

		daNode := &DANode{
			cfg: cfg,
			node: &node{
				TestName: "test",
				Index:    0,
			},
		}

		// Test internal address methods
		rpcAddr, err := daNode.GetInternalRPCAddress()
		require.NoError(t, err)
		require.Contains(t, rpcAddr, ":27000")

		p2pAddr, err := daNode.GetInternalP2PAddress()
		require.NoError(t, err)
		require.Contains(t, p2pAddr, ":3000")
	})
}

func TestConfigurationOptions(t *testing.T) {
	t.Run("WithDANodePorts configures DA node ports", func(t *testing.T) {
		cfg := Config{}
		option := WithDANodePorts("27000", "3000")
		option(&cfg)

		require.NotNil(t, cfg.DataAvailabilityNetworkConfig)
		require.Equal(t, "27000", cfg.DataAvailabilityNetworkConfig.DefaultRPCPort)
		require.Equal(t, "3000", cfg.DataAvailabilityNetworkConfig.DefaultP2PPort)
	})

	t.Run("WithDANodeCoreConnection configures core connection ports", func(t *testing.T) {
		cfg := Config{}
		option := WithDANodeCoreConnection("27001", "9095")
		option(&cfg)

		require.NotNil(t, cfg.DataAvailabilityNetworkConfig)
		require.Equal(t, "27001", cfg.DataAvailabilityNetworkConfig.DefaultCoreRPCPort)
		require.Equal(t, "9095", cfg.DataAvailabilityNetworkConfig.DefaultCoreGRPCPort)
	})

	t.Run("WithDANodePorts and WithDANodeCoreConnection combined configure all ports", func(t *testing.T) {
		cfg := Config{}
		// Apply both options to configure all ports
		WithDANodePorts("27000", "3000")(&cfg)
		WithDANodeCoreConnection("27001", "9095")(&cfg)

		require.NotNil(t, cfg.DataAvailabilityNetworkConfig)
		require.Equal(t, "27000", cfg.DataAvailabilityNetworkConfig.DefaultRPCPort)
		require.Equal(t, "3000", cfg.DataAvailabilityNetworkConfig.DefaultP2PPort)
		require.Equal(t, "27001", cfg.DataAvailabilityNetworkConfig.DefaultCoreRPCPort)
		require.Equal(t, "9095", cfg.DataAvailabilityNetworkConfig.DefaultCoreGRPCPort)
	})

	t.Run("WithDefaultPorts uses predefined default ports", func(t *testing.T) {
		cfg := Config{}
		option := WithDefaultPorts()
		option(&cfg)

		require.NotNil(t, cfg.DataAvailabilityNetworkConfig)
		require.Equal(t, "26668", cfg.DataAvailabilityNetworkConfig.DefaultRPCPort)
		require.Equal(t, "2131", cfg.DataAvailabilityNetworkConfig.DefaultP2PPort)
		require.Equal(t, "26667", cfg.DataAvailabilityNetworkConfig.DefaultCoreRPCPort)
		require.Equal(t, "9091", cfg.DataAvailabilityNetworkConfig.DefaultCoreGRPCPort)
	})

	t.Run("WithNodePorts configures per-node ports for bridge nodes", func(t *testing.T) {
		cfg := Config{}
		option := WithNodePorts(types.BridgeNode, 0, "28000", "4000")
		option(&cfg)

		require.NotNil(t, cfg.DataAvailabilityNetworkConfig)
		require.NotNil(t, cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs)
		require.Contains(t, cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs, 0)
		require.Equal(t, "28000", cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[0].RPCPort)
		require.Equal(t, "4000", cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[0].P2PPort)
	})

	t.Run("WithNodePorts configures per-node ports for full nodes", func(t *testing.T) {
		cfg := Config{}
		option := WithNodePorts(types.FullNode, 0, "28001", "4001")
		option(&cfg)

		require.NotNil(t, cfg.DataAvailabilityNetworkConfig)
		require.NotNil(t, cfg.DataAvailabilityNetworkConfig.FullNodeConfigs)
		require.Contains(t, cfg.DataAvailabilityNetworkConfig.FullNodeConfigs, 0)
		require.Equal(t, "28001", cfg.DataAvailabilityNetworkConfig.FullNodeConfigs[0].RPCPort)
		require.Equal(t, "4001", cfg.DataAvailabilityNetworkConfig.FullNodeConfigs[0].P2PPort)
	})

	t.Run("WithNodePorts configures per-node ports for light nodes", func(t *testing.T) {
		cfg := Config{}
		option := WithNodePorts(types.LightNode, 0, "28002", "4002")
		option(&cfg)

		require.NotNil(t, cfg.DataAvailabilityNetworkConfig)
		require.NotNil(t, cfg.DataAvailabilityNetworkConfig.LightNodeConfigs)
		require.Contains(t, cfg.DataAvailabilityNetworkConfig.LightNodeConfigs, 0)
		require.Equal(t, "28002", cfg.DataAvailabilityNetworkConfig.LightNodeConfigs[0].RPCPort)
		require.Equal(t, "4002", cfg.DataAvailabilityNetworkConfig.LightNodeConfigs[0].P2PPort)
	})
}
