package docker

import (
	"context"
	"testing"

	"github.com/celestiaorg/tastora/framework/docker/container"
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

	// configure different images for different DA node types
	bridgeNodeConfigs := map[int]*DANodeConfig{
		0: {
			Image: &container.Image{
				Repository: "ghcr.io/celestiaorg/celestia-node",
				Version:    "pr-4283",
				UIDGID:     "10001:10001",
			},
		},
	}

	fullNodeConfigs := map[int]*DANodeConfig{
		0: {
			Image: &container.Image{
				Repository: "ghcr.io/celestiaorg/celestia-node",
				Version:    "pr-4283",
				UIDGID:     "10001:10001",
			},
		},
	}

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t,
		WithPerBridgeNodeConfig(bridgeNodeConfigs),
		WithPerFullNodeConfig(fullNodeConfigs),
	)
	provider := testCfg.Provider
	chain, err := testCfg.Builder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	var (
		bridgeNodes []types.DANode
		lightNodes  []types.DANode
		fullNodes   []types.DANode
	)

	// default configuration uses 1/1/1 for Bridge/Light/Full da nodes.
	daNetwork, err := provider.GetDataAvailabilityNetwork(testCfg.Ctx)
	require.NoError(t, err)

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
			types.WithChainID(chainID),
			types.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			types.WithEnvironmentVariables(
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
			types.WithChainID(chainID),
			types.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			types.WithEnvironmentVariables(
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
			types.WithChainID(chainID),
			types.WithAdditionalStartArguments("--p2p.network", chainID, "--rpc.addr", "0.0.0.0"),
			types.WithEnvironmentVariables(
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
	var bridgeNodes []types.DANode

	provider := testCfg.Provider
	chain, err := testCfg.Builder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	// default configuration uses 1/1/1 for Bridge/Light/Full da nodes.
	daNetwork, err := provider.GetDataAvailabilityNetwork(testCfg.Ctx)
	require.NoError(t, err)

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
			types.WithChainID(chainID),
			types.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			types.WithEnvironmentVariables(
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

// TestDANetworkCustomPorts tests the configuration of custom ports for DA nodes
func TestDANetworkCustomPorts(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()
	configureBech32PrefixOnce()

	t.Run("test simple configuration with WithDefaultPorts", func(t *testing.T) {
		// Setup isolated docker environment for this test
		testCfg := setupDockerTest(t,
			WithDefaultPorts(), // This should use ports 26668, 2131, 26667, 9091
		)

		// Test the simple one-liner configuration
		provider := testCfg.Provider

		chain, err := testCfg.Builder.Build(testCfg.Ctx)
		require.NoError(t, err)

		err = chain.Start(testCfg.Ctx)
		require.NoError(t, err)
		defer func() { _ = chain.Stop(testCfg.Ctx) }()

		daNetwork, err := provider.GetDataAvailabilityNetwork(testCfg.Ctx)
		require.NoError(t, err)

		bridgeNodes := daNetwork.GetBridgeNodes()
		require.Len(t, bridgeNodes, 1)

		bridgeNode := bridgeNodes[0]

		// Verify that internal addresses use the custom ports
		rpcAddr, err := bridgeNode.GetInternalRPCAddress()
		require.NoError(t, err)
		require.Contains(t, rpcAddr, ":26668", "RPC address should use custom port 26668")

		p2pAddr, err := bridgeNode.GetInternalP2PAddress()
		require.NoError(t, err)
		require.Contains(t, p2pAddr, ":2131", "P2P address should use custom port 2131")
	})

	t.Run("test custom ports setup with individual functions", func(t *testing.T) {
		// Setup isolated docker environment for this test
		testCfg := setupDockerTest(t,
			WithDANodePorts("27000", "3000"),
			WithDANodeCoreConnection("27001", "9095"),
		)

		// Test the custom ports configuration using individual functions
		provider := testCfg.Provider

		chain, err := testCfg.Builder.Build(testCfg.Ctx)
		require.NoError(t, err)

		err = chain.Start(testCfg.Ctx)
		require.NoError(t, err)
		defer func() { _ = chain.Stop(testCfg.Ctx) }()

		daNetwork, err := provider.GetDataAvailabilityNetwork(testCfg.Ctx)
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
	})

	t.Run("test per-node configuration with WithNodePorts", func(t *testing.T) {
		// Setup isolated docker environment for this test
		testCfg := setupDockerTest(t,
			WithNodePorts(types.BridgeNode, 0, "28000", "4000"), // Configure bridge node 0 with specific ports
		)

		// Test per-node configuration
		provider := testCfg.Provider

		chain, err := testCfg.Builder.Build(testCfg.Ctx)
		require.NoError(t, err)

		err = chain.Start(testCfg.Ctx)
		require.NoError(t, err)
		defer func() { _ = chain.Stop(testCfg.Ctx) }()

		daNetwork, err := provider.GetDataAvailabilityNetwork(testCfg.Ctx)
		require.NoError(t, err)

		bridgeNodes := daNetwork.GetBridgeNodes()
		require.Len(t, bridgeNodes, 1)

		bridgeNode := bridgeNodes[0]

		// Verify that internal addresses use the per-node custom ports
		rpcAddr, err := bridgeNode.GetInternalRPCAddress()
		require.NoError(t, err)
		require.Contains(t, rpcAddr, ":28000", "RPC address should use per-node custom port 28000")

		p2pAddr, err := bridgeNode.GetInternalP2PAddress()
		require.NoError(t, err)
		require.Contains(t, p2pAddr, ":4000", "P2P address should use per-node custom port 4000")
	})

	t.Run("test backward compatibility - default behavior unchanged", func(t *testing.T) {
		// Setup isolated docker environment for this test
		testCfg := setupDockerTest(t) // No custom configuration

		// Test that existing code works unchanged
		provider := testCfg.Provider

		chain, err := testCfg.Builder.Build(testCfg.Ctx)
		require.NoError(t, err)

		err = chain.Start(testCfg.Ctx)
		require.NoError(t, err)
		defer func() { _ = chain.Stop(testCfg.Ctx) }()

		daNetwork, err := provider.GetDataAvailabilityNetwork(testCfg.Ctx)
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
	})
}
