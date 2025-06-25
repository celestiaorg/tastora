package docker

import (
	"testing"

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

	t.Run("WithCustomPortsSetup configures all ports", func(t *testing.T) {
		cfg := Config{}
		option := WithCustomPortsSetup("27000", "3000", "27001", "9095")
		option(&cfg)

		require.NotNil(t, cfg.DataAvailabilityNetworkConfig)
		require.Equal(t, "27000", cfg.DataAvailabilityNetworkConfig.DefaultRPCPort)
		require.Equal(t, "3000", cfg.DataAvailabilityNetworkConfig.DefaultP2PPort)
		require.Equal(t, "27001", cfg.DataAvailabilityNetworkConfig.DefaultCoreRPCPort)
		require.Equal(t, "9095", cfg.DataAvailabilityNetworkConfig.DefaultCoreGRPCPort)
	})

	t.Run("WithNonConflictingPorts uses predefined non-conflicting ports", func(t *testing.T) {
		cfg := Config{}
		option := WithNonConflictingPorts()
		option(&cfg)

		require.NotNil(t, cfg.DataAvailabilityNetworkConfig)
		require.Equal(t, "26668", cfg.DataAvailabilityNetworkConfig.DefaultRPCPort)
		require.Equal(t, "2131", cfg.DataAvailabilityNetworkConfig.DefaultP2PPort)
		require.Equal(t, "26667", cfg.DataAvailabilityNetworkConfig.DefaultCoreRPCPort)
		require.Equal(t, "9091", cfg.DataAvailabilityNetworkConfig.DefaultCoreGRPCPort)
	})

	t.Run("WithBridgeNodePorts configures per-node ports", func(t *testing.T) {
		cfg := Config{}
		option := WithBridgeNodePorts(0, "28000", "4000")
		option(&cfg)

		require.NotNil(t, cfg.DataAvailabilityNetworkConfig)
		require.NotNil(t, cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs)
		require.Contains(t, cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs, 0)
		require.Equal(t, "28000", cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[0].RPCPort)
		require.Equal(t, "4000", cfg.DataAvailabilityNetworkConfig.BridgeNodeConfigs[0].P2PPort)
	})
}
