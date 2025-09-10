package docker

import (
	"context"
	"testing"

	"github.com/celestiaorg/tastora/framework/docker/container"
	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/stretchr/testify/require"
)

// TestDANetworkCreation tests the creation of a dataavailability.Network with one of each type of node.
func TestDANetworkCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	chain, err := testCfg.ChainBuilder.Build(testCfg.Ctx)
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
	bridgeNodeConfig := da.NewNodeBuilder().
		WithNodeType(types.BridgeNode).
		WithImage(bridgeImage).
		Build()

	fullNodeConfig := da.NewNodeBuilder().
		WithNodeType(types.FullNode).
		WithImage(fullImage).
		Build()

	// Default image for the network
	defaultImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-node",
		Version:    "pr-4283",
		UIDGID:     "10001:10001",
	}

	// Add light node config for testing
	lightNodeConfig := da.NewNodeBuilder().
		WithNodeType(types.LightNode).
		Build()

	// Create DA network with all node types (default configuration uses 1/1/1 for Bridge/Light/Full da nodes)
	daNetwork, err := testCfg.DANetworkBuilder.
		WithChainID(chain.GetChainID()).
		WithImage(defaultImage).
		WithNodes(bridgeNodeConfig, lightNodeConfig, fullNodeConfig).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	var (
		bridgeNodes []*da.Node
		lightNodes  []*da.Node
		fullNodes   []*da.Node
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

	chainNetworkInfo, err := chain.GetNodes()[0].GetNetworkInfo(testCfg.Ctx)
	require.NoError(t, err, "failed to get network info")
	hostname := chainNetworkInfo.Internal.Hostname

	bridgeNode := bridgeNodes[0]
	fullNode := fullNodes[0]
	lightNode := lightNodes[0]

	chainID := chain.GetChainID()

	t.Run("bridge node can be started", func(t *testing.T) {
		err = bridgeNode.Start(testCfg.Ctx,
			da.WithChainID(chainID),
			da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			da.WithEnvironmentVariables(
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
			da.WithChainID(chainID),
			da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			da.WithEnvironmentVariables(
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
			da.WithChainID(chainID),
			da.WithAdditionalStartArguments("--p2p.network", chainID, "--rpc.addr", "0.0.0.0"),
			da.WithEnvironmentVariables(
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

	chain, err := testCfg.ChainBuilder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	// Default image for the DA network
	defaultImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-node",
		Version:    "pr-4283",
		UIDGID:     "10001:10001",
	}

	// Create bridge node config for testing
	bridgeNodeConfig := da.NewNodeBuilder().
		WithNodeType(types.BridgeNode).
		Build()

	// Create DA network with bridge node
	daNetwork, err := testCfg.DANetworkBuilder.
		WithChainID(chain.GetChainID()).
		WithImage(defaultImage).
		WithNodes(bridgeNodeConfig).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	var bridgeNodes []*da.Node
	t.Run("da nodes can be created", func(t *testing.T) {
		bridgeNodes = daNetwork.GetBridgeNodes()
		require.Len(t, bridgeNodes, 1)
	})

	genesisHash, err := getGenesisHash(testCfg.Ctx, chain)
	require.NoError(t, err)

	chainNetworkInfo, err := chain.GetNodes()[0].GetNetworkInfo(testCfg.Ctx)
	require.NoError(t, err, "failed to get network info")
	hostname := chainNetworkInfo.Internal.Hostname

	bridgeNode := bridgeNodes[0]

	chainID := chain.GetChainID()

	t.Run("bridge node can be started", func(t *testing.T) {
		err = bridgeNode.Start(testCfg.Ctx,
			da.WithChainID(chainID),
			da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			da.WithEnvironmentVariables(
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
func setAuth(t *testing.T, ctx context.Context, daNode *da.Node, auth bool) {
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

// TestDANetworkCustomPorts tests the configuration of custom ports for DA nodes.
func TestDANetworkCustomPorts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	t.Run("test custom ports using builder pattern", func(t *testing.T) {
		// Setup isolated docker environment for this test
		testCfg := setupDockerTest(t)

		chain, err := testCfg.ChainBuilder.Build(testCfg.Ctx)
		require.NoError(t, err)

		err = chain.Start(testCfg.Ctx)
		require.NoError(t, err)
		defer func() { _ = chain.Stop(testCfg.Ctx) }()

		// Default image for the DA network
		defaultImage := container.Image{
			Repository: "ghcr.io/celestiaorg/celestia-node",
			Version:    "pr-4283",
			UIDGID:     "10001:10001",
		}

		// Create bridge node config with custom ports
		bridgeNodeConfig := da.NewNodeBuilder().
			WithNodeType(types.BridgeNode).
			WithInternalPorts(types.Ports{
				RPC:      "27000",
				P2P:      "3000",
				CoreRPC:  "27001",
				CoreGRPC: "9095",
			}).
			Build()

		// Create DA network with custom port bridge node
		daNetwork, err := testCfg.DANetworkBuilder.
			WithChainID(chain.GetChainID()).
			WithImage(defaultImage).
			WithNodes(bridgeNodeConfig).
			Build(testCfg.Ctx)
		require.NoError(t, err)

		bridgeNodes := daNetwork.GetBridgeNodes()
		require.Len(t, bridgeNodes, 1)

		bridgeNode := bridgeNodes[0]

		chainNetworkInfo, err := chain.GetNetworkInfo(context.Background())
		require.NoError(t, err)

		chainID := chain.GetChainID()
		genesisHash, err := getGenesisHash(testCfg.Ctx, chain)
		require.NoError(t, err)

		require.NoError(t, bridgeNode.Start(testCfg.Ctx,
			da.WithChainID(chainID),
			da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", chainNetworkInfo.Internal.Hostname, "--rpc.addr", "0.0.0.0"),
			da.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
					"P2P_NETWORK":     chainID,
				},
			)))

		// Verify that internal addresses use the custom ports
		bridgeNetworkInfo, err := bridgeNode.GetNetworkInfo(context.Background())
		require.NoError(t, err)
		rpcAddr := bridgeNetworkInfo.Internal.RPCAddress()
		require.Contains(t, rpcAddr, ":27000", "RPC address should use custom port 27000")

		bridgeP2PNetworkInfo, err := bridgeNode.GetNetworkInfo(context.Background())
		require.NoError(t, err)
		p2pAddr := bridgeP2PNetworkInfo.Internal.P2PAddress()
		require.Contains(t, p2pAddr, ":3000", "P2P address should use custom port 3000")

		// Verify all custom ports using GetPortInfo
		portInfo := bridgeP2PNetworkInfo.Internal.Ports
		require.Equal(t, "27000", portInfo.RPC, "RPC port should be custom port 27000")
		require.Equal(t, "3000", portInfo.P2P, "P2P port should be custom port 3000")
		require.Equal(t, "27001", portInfo.CoreRPC, "Core RPC port should be custom port 27001")
		require.Equal(t, "9095", portInfo.CoreGRPC, "Core GRPC port should be custom port 9095")
	})

	t.Run("test default ports behavior", func(t *testing.T) {
		// Setup isolated docker environment for this test
		testCfg := setupDockerTest(t)

		chain, err := testCfg.ChainBuilder.Build(testCfg.Ctx)
		require.NoError(t, err)

		err = chain.Start(testCfg.Ctx)
		require.NoError(t, err)
		defer func() { _ = chain.Stop(testCfg.Ctx) }()

		// Default image for the DA network
		defaultImage := container.Image{
			Repository: "ghcr.io/celestiaorg/celestia-node",
			Version:    "pr-4283",
			UIDGID:     "10001:10001",
		}

		// Create bridge node config with default ports (no custom ports specified)
		bridgeNodeConfig := da.NewNodeBuilder().
			WithNodeType(types.BridgeNode).
			Build()

		// Create DA network with default port bridge node
		daNetwork, err := testCfg.DANetworkBuilder.
			WithChainID(chain.GetChainID()).
			WithImage(defaultImage).
			WithNodes(bridgeNodeConfig).
			Build(testCfg.Ctx)
		require.NoError(t, err)

		bridgeNodes := daNetwork.GetBridgeNodes()
		require.Len(t, bridgeNodes, 1)

		bridgeNode := bridgeNodes[0]

		chainNetworkInfo, err := chain.GetNetworkInfo(context.Background())
		require.NoError(t, err)

		chainID := chain.GetChainID()
		genesisHash, err := getGenesisHash(testCfg.Ctx, chain)
		require.NoError(t, err)

		require.NoError(t, bridgeNode.Start(testCfg.Ctx,
			da.WithChainID(chainID),
			da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", chainNetworkInfo.Internal.Hostname, "--rpc.addr", "0.0.0.0"),
			da.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
					"P2P_NETWORK":     chainID,
				},
			)))

		// Verify that internal addresses use the default ports
		bridgeNetworkInfo, err := bridgeNode.GetNetworkInfo(context.Background())
		require.NoError(t, err)
		rpcAddr := bridgeNetworkInfo.Internal.RPCAddress()
		require.Contains(t, rpcAddr, ":26658", "RPC address should use default port 26658")

		require.NoError(t, err)
		p2pAddr := bridgeNetworkInfo.Internal.P2PAddress()
		require.Contains(t, p2pAddr, ":2121", "P2P address should use default port 2121")

		// Verify all default ports using GetPortInfo
		portInfo := bridgeNetworkInfo.Internal.Ports
		require.Equal(t, "26658", portInfo.RPC, "RPC port should be default port 26658")
		require.Equal(t, "2121", portInfo.P2P, "P2P port should be default port 2121")
		require.Equal(t, "26657", portInfo.CoreRPC, "Core RPC port should be default port 26657")
		require.Equal(t, "9090", portInfo.CoreGRPC, "Core GRPC port should be default port 9090")
	})
}

// TestDANetworkAddNode tests the dynamic addition of nodes to a DA network
func TestDANetworkAddNode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	chain, err := testCfg.ChainBuilder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	// Default image for the DA network
	defaultImage := container.Image{
		Repository: "ghcr.io/celestiaorg/celestia-node",
		Version:    "pr-4283",
		UIDGID:     "10001:10001",
	}

	// Create initial bridge node config
	bridgeNodeConfig := da.NewNodeBuilder().
		WithNodeType(types.BridgeNode).
		Build()

	// Create DA network with just the bridge node initially
	daNetwork, err := testCfg.DANetworkBuilder.
		WithChainID(chain.GetChainID()).
		WithImage(defaultImage).
		WithNodes(bridgeNodeConfig).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	// Verify initial state - only bridge node exists
	require.Len(t, daNetwork.GetBridgeNodes(), 1, "should have 1 bridge node initially")
	require.Len(t, daNetwork.GetFullNodes(), 0, "should have 0 full nodes initially")
	require.Len(t, daNetwork.GetLightNodes(), 0, "should have 0 light nodes initially")
	require.Len(t, daNetwork.GetNodes(), 1, "should have 1 total node initially")

	// Start the initial bridge node first
	bridgeNode := daNetwork.GetBridgeNodes()[0]
	chainID := chain.GetChainID()
	genesisHash, err := getGenesisHash(testCfg.Ctx, chain)
	require.NoError(t, err)

	chainNetworkInfo, err := chain.GetNodes()[0].GetNetworkInfo(testCfg.Ctx)
	require.NoError(t, err, "failed to get network info")
	hostname := chainNetworkInfo.Internal.Hostname

	err = bridgeNode.Start(testCfg.Ctx,
		da.WithChainID(chainID),
		da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
		da.WithEnvironmentVariables(
			map[string]string{
				"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
				"P2P_NETWORK":     chainID,
			},
		),
	)
	require.NoError(t, err, "should be able to start initial bridge node")

	t.Run("can dynamically add a full node", func(t *testing.T) {
		// Create a new full node configuration
		fullNodeConfig := da.NewNodeBuilder().
			WithNodeType(types.FullNode).
			Build()

		// Dynamically add the full node to the network
		newNode, err := daNetwork.AddNode(testCfg.Ctx, fullNodeConfig)
		require.NoError(t, err, "should be able to add a full node dynamically")
		require.NotNil(t, newNode, "new node should not be nil")
		require.Equal(t, types.FullNode, newNode.GetType(), "new node should be a full node")

		// Get bridge node P2P info for the new full node
		p2pInfo, err := bridgeNode.GetP2PInfo(testCfg.Ctx)
		require.NoError(t, err, "failed to get bridge node p2p info")

		p2pAddr, err := p2pInfo.GetP2PAddress()
		require.NoError(t, err, "failed to get bridge node p2p address")

		// Start the new full node with proper configuration
		err = newNode.Start(testCfg.Ctx,
			da.WithChainID(chainID),
			da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			da.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, p2pAddr),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		require.NoError(t, err, "should be able to start dynamically added full node")

		// Verify the node was added to the network
		require.Len(t, daNetwork.GetBridgeNodes(), 1, "should still have 1 bridge node")
		require.Len(t, daNetwork.GetFullNodes(), 1, "should now have 1 full node")
		require.Len(t, daNetwork.GetLightNodes(), 0, "should still have 0 light nodes")
		require.Len(t, daNetwork.GetNodes(), 2, "should now have 2 total nodes")
	})

	t.Run("can dynamically add a light node", func(t *testing.T) {
		// Create a new light node configuration
		lightNodeConfig := da.NewNodeBuilder().
			WithNodeType(types.LightNode).
			Build()

		// Dynamically add the light node to the network
		newNode, err := daNetwork.AddNode(testCfg.Ctx, lightNodeConfig)
		require.NoError(t, err, "should be able to add a light node dynamically")
		require.NotNil(t, newNode, "new node should not be nil")
		require.Equal(t, types.LightNode, newNode.GetType(), "new node should be a light node")

		// Get full node P2P info for the new light node (light nodes connect to full nodes)
		fullNodes := daNetwork.GetFullNodes()
		require.Len(t, fullNodes, 1, "should have at least 1 full node to connect light node to")

		p2pInfo, err := fullNodes[0].GetP2PInfo(testCfg.Ctx)
		require.NoError(t, err, "failed to get full node p2p info")

		p2pAddr, err := p2pInfo.GetP2PAddress()
		require.NoError(t, err, "failed to get full node p2p address")

		// Start the new light node with proper configuration
		err = newNode.Start(testCfg.Ctx,
			da.WithChainID(chainID),
			da.WithAdditionalStartArguments("--p2p.network", chainID, "--rpc.addr", "0.0.0.0"),
			da.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, p2pAddr),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		require.NoError(t, err, "should be able to start dynamically added light node")

		// Verify the node was added to the network
		require.Len(t, daNetwork.GetBridgeNodes(), 1, "should still have 1 bridge node")
		require.Len(t, daNetwork.GetFullNodes(), 1, "should still have 1 full node")
		require.Len(t, daNetwork.GetLightNodes(), 1, "should now have 1 light node")
		require.Len(t, daNetwork.GetNodes(), 3, "should now have 3 total nodes")
	})

	t.Run("can remove nodes dynamically", func(t *testing.T) {
		// Remove the light node first
		lightNodes := daNetwork.GetLightNodes()
		require.Len(t, lightNodes, 1, "should have 1 light node before removal")
		
		lightNodeName := lightNodes[0].Name()
		err := daNetwork.RemoveNode(testCfg.Ctx, lightNodeName)
		require.NoError(t, err, "should be able to remove light node by name")

		// Verify light node was removed
		require.Len(t, daNetwork.GetBridgeNodes(), 1, "should still have 1 bridge node")
		require.Len(t, daNetwork.GetFullNodes(), 1, "should still have 1 full node") 
		require.Len(t, daNetwork.GetLightNodes(), 0, "should now have 0 light nodes")
		require.Len(t, daNetwork.GetNodes(), 2, "should now have 2 total nodes")

		// Now remove the full node
		fullNodes := daNetwork.GetFullNodes()
		require.Len(t, fullNodes, 1, "should have 1 full node before removal")
		
		fullNodeName := fullNodes[0].Name()
		err = daNetwork.RemoveNode(testCfg.Ctx, fullNodeName)
		require.NoError(t, err, "should be able to remove full node by name")

		// Verify full node was also removed
		require.Len(t, daNetwork.GetBridgeNodes(), 1, "should still have 1 bridge node")
		require.Len(t, daNetwork.GetFullNodes(), 0, "should now have 0 full nodes")
		require.Len(t, daNetwork.GetLightNodes(), 0, "should still have 0 light nodes")
		require.Len(t, daNetwork.GetNodes(), 1, "should now have 1 total node (only bridge)")

		// Verify removed nodes are no longer in any node lists
		allNodes := daNetwork.GetNodes()
		for _, node := range allNodes {
			require.NotEqual(t, lightNodeName, node.Name(), "removed light node should not be in any node list")
			require.NotEqual(t, fullNodeName, node.Name(), "removed full node should not be in any node list")
		}
		
		// Verify only the bridge node remains
		require.Equal(t, 1, len(allNodes), "should only have bridge node remaining")
		require.Equal(t, types.BridgeNode, allNodes[0].GetType(), "remaining node should be bridge node")
	})

	t.Run("removing non-existent node returns error", func(t *testing.T) {
		err := daNetwork.RemoveNode(testCfg.Ctx, "non-existent-node")
		require.Error(t, err, "removing non-existent node should return error")
		require.Contains(t, err.Error(), "not found in network", "error should indicate node not found")
	})

	t.Run("can add nodes back after removing them", func(t *testing.T) {
		// Current state: only bridge node remains (from previous test)
		require.Len(t, daNetwork.GetNodes(), 1, "should only have bridge node at start")
		require.Len(t, daNetwork.GetBridgeNodes(), 1, "should have 1 bridge node")

		// Add a new full node back
		newFullNodeConfig := da.NewNodeBuilder().
			WithNodeType(types.FullNode).
			Build()

		newFullNode, err := daNetwork.AddNode(testCfg.Ctx, newFullNodeConfig)
		require.NoError(t, err, "should be able to add full node back")

		// Start the new full node
		p2pInfo, err := bridgeNode.GetP2PInfo(testCfg.Ctx)
		require.NoError(t, err, "failed to get bridge node p2p info")
		
		p2pAddr, err := p2pInfo.GetP2PAddress()
		require.NoError(t, err, "failed to get bridge node p2p address")

		err = newFullNode.Start(testCfg.Ctx,
			da.WithChainID(chainID),
			da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			da.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, p2pAddr),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		require.NoError(t, err, "should be able to start new full node")

		// Verify network state after adding back
		require.Len(t, daNetwork.GetNodes(), 2, "should now have 2 total nodes")
		require.Len(t, daNetwork.GetBridgeNodes(), 1, "should still have 1 bridge node")
		require.Len(t, daNetwork.GetFullNodes(), 1, "should now have 1 full node again")
		require.Len(t, daNetwork.GetLightNodes(), 0, "should still have 0 light nodes")

		// Verify the nodes are sorted by name in GetNodes()
		allNodes := daNetwork.GetNodes()
		if len(allNodes) >= 2 {
			require.True(t, allNodes[0].Name() < allNodes[1].Name(), "nodes should be sorted by name")
		}
	})
}
