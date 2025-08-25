package docker

import (
	"context"
	"testing"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/dataavailability"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/stretchr/testify/require"
)

// TestDANetworkCreation tests the creation of a DataAvailabilityNetwork with one of each type of node.
func TestDANetworkCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)
	
	chain, err := testCfg.Builder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	// Configure different images for different DA node types using builder pattern
	bridgeImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-node",
		Version:    "pr-4283",
		UIDGID:     "10001:10001",
	}
	
	fullImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-node",
		Version:    "pr-4283",
		UIDGID:     "10001:10001",
	}

	// Create node configurations with different images
	bridgeNodeConfig := dataavailability.NewNodeBuilder().
		WithNodeType(dataavailability.BridgeNodeType).
		WithImage(bridgeImage).
		Build()

	fullNodeConfig := dataavailability.NewNodeBuilder().
		WithNodeType(dataavailability.FullNodeType).
		WithImage(fullImage).
		Build()

	// Default image for the network
	defaultImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-node",
		Version:    "main",
		UIDGID:     "10001:10001",
	}

	// Add light node config for testing
	lightNodeConfig := dataavailability.NewNodeBuilder().
		WithNodeType(dataavailability.LightNodeType).
		Build()

	// Create DA network with all node types (default configuration uses 1/1/1 for Bridge/Light/Full da nodes)
	daNetwork, err := dataavailability.NewNetworkBuilder(t).
		WithChainID(chain.GetChainID()).
		WithImage(defaultImage).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithNodes(bridgeNodeConfig, lightNodeConfig, fullNodeConfig).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	var (
		bridgeNodes []*dataavailability.Node
		lightNodes  []*dataavailability.Node
		fullNodes   []*dataavailability.Node
	)

	t.Run("da nodes can be created", func(t *testing.T) {
		bridgeNodes = daNetwork.GetBridgeNodes()
		require.Len(t, bridgeNodes, 1)

		lightNodes = daNetwork.GetLightNodes()
		require.Len(t, lightNodes, 1)

		fullNodes = daNetwork.GetFullNodes()
		require.Len(t, fullNodes, 1)
	})

	genesisHash, err := getGenesisHash(testCfg.Ctx, chain)
	require.NoError(t, err)

	hostname, err := chain.GetNodes()[0].GetInternalHostName(testCfg.Ctx)
	require.NoError(t, err, "failed to get internal hostname")

	bridgeNode := bridgeNodes[0]
	fullNode := fullNodes[0]
	lightNode := lightNodes[0]

	chainID := chain.GetChainID()

	t.Run("bridge node can be started", func(t *testing.T) {
		err = bridgeNode.Start(testCfg.Ctx,
			dataavailability.WithChainID(chainID),
			dataavailability.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			dataavailability.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		require.NoError(t, err)
	})

	t.Run("full node can be started", func(t *testing.T) {
		p2pInfo, err := bridgeNode.GetP2PInfo(testCfg.Ctx)
		require.NoError(t, err, "failed to get bridge node p2p info")

		p2pAddr, err := p2pInfo.GetP2PAddress()
		require.NoError(t, err, "failed to get bridge node p2p address")

		err = fullNode.Start(testCfg.Ctx,
			dataavailability.WithChainID(chainID),
			dataavailability.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			dataavailability.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, p2pAddr),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		require.NoError(t, err)
	})

	t.Run("light node can be started", func(t *testing.T) {
		p2pInfo, err := fullNode.GetP2PInfo(testCfg.Ctx)
		require.NoError(t, err, "failed to get full node p2p info")

		p2pAddr, err := p2pInfo.GetP2PAddress()
		require.NoError(t, err, "failed to get full node p2p address")

		err = lightNode.Start(testCfg.Ctx,
			dataavailability.WithChainID(chainID),
			dataavailability.WithAdditionalStartArguments("--p2p.network", chainID, "--rpc.addr", "0.0.0.0"),
			dataavailability.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, p2pAddr),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		require.NoError(t, err)
	})
}

// TestModifyConfigFileDANetwork ensures modification of config files is possible by
// - disabling auth at startup
// - enabling auth and making sure it is not possible to query RPC
// - disabling auth again and verifying it is possible to query RPC
func TestModifyConfigFileDANetwork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)
	
	chain, err := testCfg.Builder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	// Default image for the DA network
	defaultImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-node",
		Version:    "main",
		UIDGID:     "10001:10001",
	}

	// Create bridge node config for testing
	bridgeNodeConfig := dataavailability.NewNodeBuilder().
		WithNodeType(dataavailability.BridgeNodeType).
		Build()

	// Create DA network with bridge node
	daNetwork, err := dataavailability.NewNetworkBuilder(t).
		WithChainID(chain.GetChainID()).
		WithImage(defaultImage).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithNode(bridgeNodeConfig).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	var bridgeNodes []*dataavailability.Node
	t.Run("da nodes can be created", func(t *testing.T) {
		bridgeNodes = daNetwork.GetBridgeNodes()
		require.Len(t, bridgeNodes, 1)
	})

	genesisHash, err := getGenesisHash(testCfg.Ctx, chain)
	require.NoError(t, err)

	hostname, err := chain.GetNodes()[0].GetInternalHostName(testCfg.Ctx)
	require.NoError(t, err, "failed to get internal hostname")

	bridgeNode := bridgeNodes[0]

	chainID := chain.GetChainID()

	t.Run("bridge node can be started", func(t *testing.T) {
		err = bridgeNode.Start(testCfg.Ctx,
			dataavailability.WithChainID(chainID),
			dataavailability.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			dataavailability.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		require.NoError(t, err)
	})

	t.Run("bridge node config changed", func(t *testing.T) {
		setAuth(t, testCfg.Ctx, bridgeNode, true)
	})

	t.Run("bridge node rpc in-accessible", func(t *testing.T) {
		_, err := bridgeNode.GetP2PInfo(testCfg.Ctx)
		require.Error(t, err, "was able to get bridge node p2p info after auth was enabled")
	})

	t.Run("bridge node config changed back", func(t *testing.T) {
		setAuth(t, testCfg.Ctx, bridgeNode, false)
	})

	t.Run("bridge node rpc accessible again", func(t *testing.T) {
		_, err := bridgeNode.GetP2PInfo(testCfg.Ctx)
		require.NoError(t, err, "failed to get bridge node p2p info")
	})
}

// setAuth modifies the node's configuration to enable or disable authentication and restarts the node to apply changes.
func setAuth(t *testing.T, ctx context.Context, daNode types.DANode, auth bool) {
	modifications := map[string]toml.Toml{
		"config.toml": {
			"RPC": toml.Toml{
				"SkipAuth": !auth,
			},
		},
	}

	err := daNode.Stop(ctx)
	require.NoErrorf(t, err, "failed to stop %s node", daNode.GetType().String())

	err = daNode.ModifyConfigFiles(ctx, modifications)
	require.NoError(t, err, "failed to modify config files")

	err = daNode.Start(ctx)
	require.NoErrorf(t, err, "failed to re-start %s node", daNode.GetType().String())
}

// TestDANetworkCustomPorts tests the configuration of custom ports for DA nodes using the new builder pattern
func TestDANetworkCustomPorts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	t.Run("test custom ports using builder pattern", func(t *testing.T) {
		// Setup isolated docker environment for this test
		testCfg := setupDockerTest(t)
		
		chain, err := testCfg.Builder.Build(testCfg.Ctx)
		require.NoError(t, err)

		err = chain.Start(testCfg.Ctx)
		require.NoError(t, err)
		defer func() { _ = chain.Stop(testCfg.Ctx) }()

		// Default image for the DA network
		defaultImage := container.Image{
			Repository: "ghcr.io/celestiaorg/celestia-node",
			Version:    "main",
			UIDGID:     "10001:10001",
		}

		// Create bridge node config with custom ports
		bridgeNodeConfig := dataavailability.NewNodeBuilder().
			WithNodeType(dataavailability.BridgeNodeType).
			WithPorts("27000", "3000", "27001", "9095").
			Build()

		// Create DA network with custom port bridge node
		daNetwork, err := dataavailability.NewNetworkBuilder(t).
			WithChainID(chain.GetChainID()).
			WithImage(defaultImage).
			WithDockerClient(testCfg.DockerClient).
			WithDockerNetworkID(testCfg.NetworkID).
			WithNode(bridgeNodeConfig).
			Build(testCfg.Ctx)
		require.NoError(t, err)

		bridgeNodes := daNetwork.GetBridgeNodes()
		require.Len(t, bridgeNodes, 1)

		bridgeNode := bridgeNodes[0]

		// Verify that internal addresses use the custom ports
		rpcAddr, err := bridgeNode.GetInternalRPCAddress()
		require.NoError(t, err)
		require.Contains(t, rpcAddr, ":27000", "RPC address should use custom port 27000")

		p2pAddr, err := bridgeNode.GetInternalP2PAddress()
		require.NoError(t, err)
		require.Contains(t, p2pAddr, ":3000", "P2P address should use custom port 3000")

		// Verify all custom ports using GetPortInfo
		portInfo := bridgeNode.GetPortInfo()
		require.Equal(t, "27000", portInfo.RPCPort, "RPC port should be custom port 27000")
		require.Equal(t, "3000", portInfo.P2PPort, "P2P port should be custom port 3000")
		require.Equal(t, "27001", portInfo.CoreRPCPort, "Core RPC port should be custom port 27001")
		require.Equal(t, "9095", portInfo.CoreGRPCPort, "Core GRPC port should be custom port 9095")
	})

	t.Run("test default ports behavior", func(t *testing.T) {
		// Setup isolated docker environment for this test
		testCfg := setupDockerTest(t)
		
		chain, err := testCfg.Builder.Build(testCfg.Ctx)
		require.NoError(t, err)

		err = chain.Start(testCfg.Ctx)
		require.NoError(t, err)
		defer func() { _ = chain.Stop(testCfg.Ctx) }()

		// Default image for the DA network
		defaultImage := container.Image{
			Repository: "ghcr.io/celestiaorg/celestia-node",
			Version:    "main",
			UIDGID:     "10001:10001",
		}

		// Create bridge node config with default ports (no custom ports specified)
		bridgeNodeConfig := dataavailability.NewNodeBuilder().
			WithNodeType(dataavailability.BridgeNodeType).
			Build()

		// Create DA network with default port bridge node
		daNetwork, err := dataavailability.NewNetworkBuilder(t).
			WithChainID(chain.GetChainID()).
			WithImage(defaultImage).
			WithDockerClient(testCfg.DockerClient).
			WithDockerNetworkID(testCfg.NetworkID).
			WithNode(bridgeNodeConfig).
			Build(testCfg.Ctx)
		require.NoError(t, err)

		bridgeNodes := daNetwork.GetBridgeNodes()
		require.Len(t, bridgeNodes, 1)

		bridgeNode := bridgeNodes[0]

		// Verify that internal addresses use the default ports
		rpcAddr, err := bridgeNode.GetInternalRPCAddress()
		require.NoError(t, err)
		require.Contains(t, rpcAddr, ":26658", "RPC address should use default port 26658")

		p2pAddr, err := bridgeNode.GetInternalP2PAddress()
		require.NoError(t, err)
		require.Contains(t, p2pAddr, ":2121", "P2P address should use default port 2121")

		// Verify all default ports using GetPortInfo
		portInfo := bridgeNode.GetPortInfo()
		require.Equal(t, "26658", portInfo.RPCPort, "RPC port should be default port 26658")
		require.Equal(t, "2121", portInfo.P2PPort, "P2P port should be default port 2121")
		require.Equal(t, "26657", portInfo.CoreRPCPort, "Core RPC port should be default port 26657")
		require.Equal(t, "9090", portInfo.CoreGRPCPort, "Core GRPC port should be default port 9090")
	})
}
