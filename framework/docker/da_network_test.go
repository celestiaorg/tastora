package docker

import (
	"context"
	"testing"

	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/types"
)

// TestDANetworkCreation tests the creation of a DataAvailabilityNetwork with one of each type of node.
func (s *DockerTestSuite) TestDANetworkCreation() {
	if testing.Short() {
		s.T().Skip("skipping due to short mode")
	}

	ctx := context.Background()

	// configure different images for different DA node types
	bridgeNodeConfigs := map[int]*DANodeConfig{
		0: {
			Image: &DockerImage{
				Repository: "ghcr.io/celestiaorg/celestia-node",
				Version:    "pr-4283",
				UIDGID:     "10001:10001",
			},
		},
	}

	fullNodeConfigs := map[int]*DANodeConfig{
		0: {
			Image: &DockerImage{
				Repository: "ghcr.io/celestiaorg/celestia-node",
				Version:    "pr-4283",
				UIDGID:     "10001:10001",
			},
		},
	}

	var err error
	s.provider = s.CreateDockerProvider(
		WithPerBridgeNodeConfig(bridgeNodeConfigs),
		WithPerFullNodeConfig(fullNodeConfigs),
	)
	s.chain, err = s.provider.GetChain(ctx)
	s.Require().NoError(err)

	err = s.chain.Start(ctx)
	s.Require().NoError(err)

	var (
		bridgeNodes []types.DANode
		lightNodes  []types.DANode
		fullNodes   []types.DANode
	)

	// default configuration uses 1/1/1 for Bridge/Light/Full da nodes.
	daNetwork, err := s.provider.GetDataAvailabilityNetwork(ctx)
	s.Require().NoError(err)

	s.T().Run("da nodes can be created", func(t *testing.T) {
		bridgeNodes = daNetwork.GetBridgeNodes()
		s.Require().Len(bridgeNodes, 1)

		lightNodes = daNetwork.GetLightNodes()
		s.Require().Len(lightNodes, 1)

		fullNodes = daNetwork.GetFullNodes()
		s.Require().Len(fullNodes, 1)
	})

	genesisHash := s.getGenesisHash(ctx)

	hostname, err := s.chain.GetNodes()[0].GetInternalHostName(ctx)
	s.Require().NoError(err, "failed to get internal hostname")

	bridgeNode := bridgeNodes[0]
	fullNode := fullNodes[0]
	lightNode := lightNodes[0]

	chainID := s.chain.GetChainID()

	s.T().Run("bridge node can be started", func(t *testing.T) {
		err = bridgeNode.Start(ctx,
			types.WithChainID(chainID),
			types.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			types.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		s.Require().NoError(err)
	})

	s.T().Run("full node can be started", func(t *testing.T) {
		p2pInfo, err := bridgeNode.GetP2PInfo(ctx)
		s.Require().NoError(err, "failed to get bridge node p2p info")

		p2pAddr, err := p2pInfo.GetP2PAddress()
		s.Require().NoError(err, "failed to get bridge node p2p address")

		err = fullNode.Start(ctx,
			types.WithChainID(chainID),
			types.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			types.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, p2pAddr),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		s.Require().NoError(err)
	})

	s.T().Run("light node can be started", func(t *testing.T) {
		p2pInfo, err := fullNode.GetP2PInfo(ctx)
		s.Require().NoError(err, "failed to get full node p2p info")

		p2pAddr, err := p2pInfo.GetP2PAddress()
		s.Require().NoError(err, "failed to get full node p2p address")

		err = lightNode.Start(ctx,
			types.WithChainID(chainID),
			types.WithAdditionalStartArguments("--p2p.network", chainID, "--rpc.addr", "0.0.0.0"),
			types.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, p2pAddr),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		s.Require().NoError(err)
	})
}

// TestModifyConfigFileDANetwork ensures modification of config files is possible by
// - disabling auth at startup
// - enabling auth and making sure it is not possible to query RPC
// - disabling auth again and verifying it is possible to query RPC
func (s *DockerTestSuite) TestModifyConfigFileDANetwork() {
	if testing.Short() {
		s.T().Skip("skipping due to short mode")
	}
	ctx := context.Background()
	var bridgeNodes []types.DANode

	var err error
	s.provider = s.CreateDockerProvider()
	s.chain, err = s.provider.GetChain(ctx)
	s.Require().NoError(err)

	err = s.chain.Start(ctx)
	s.Require().NoError(err)

	// default configuration uses 1/1/1 for Bridge/Light/Full da nodes.
	daNetwork, err := s.provider.GetDataAvailabilityNetwork(ctx)
	s.Require().NoError(err)

	s.T().Run("da nodes can be created", func(t *testing.T) {
		bridgeNodes = daNetwork.GetBridgeNodes()
		s.Require().Len(bridgeNodes, 1)
	})

	genesisHash := s.getGenesisHash(ctx)

	hostname, err := s.chain.GetNodes()[0].GetInternalHostName(ctx)
	s.Require().NoError(err, "failed to get internal hostname")

	bridgeNode := bridgeNodes[0]

	chainID := s.chain.GetChainID()

	s.T().Run("bridge node can be started", func(t *testing.T) {
		err = bridgeNode.Start(ctx,
			types.WithChainID(chainID),
			types.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			types.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		s.Require().NoError(err)
	})

	s.T().Run("bridge node config changed", func(t *testing.T) {
		s.setAuth(ctx, bridgeNode, true)
	})

	s.T().Run("bridge node rpc in-accessible", func(t *testing.T) {
		_, err := bridgeNode.GetP2PInfo(ctx)
		s.Require().Error(err, "was able to get bridge node p2p info after auth was enabled")
	})

	s.T().Run("bridge node config changed back", func(t *testing.T) {
		s.setAuth(ctx, bridgeNode, false)
	})

	s.T().Run("bridge node rpc accessible again", func(t *testing.T) {
		_, err := bridgeNode.GetP2PInfo(ctx)
		s.Require().NoError(err, "failed to get bridge node p2p info")
	})
}

// setAuth modifies the node's configuration to enable or disable authentication and restarts the node to apply changes.
func (s *DockerTestSuite) setAuth(ctx context.Context, daNode types.DANode, auth bool) {
	modifications := map[string]toml.Toml{
		"config.toml": {
			"RPC": toml.Toml{
				"SkipAuth": !auth,
			},
		},
	}

	err := daNode.Stop(ctx)
	s.Require().NoErrorf(err, "failed to stop %s node", daNode.GetType().String())

	err = daNode.ModifyConfigFiles(ctx, modifications)
	s.Require().NoError(err, "failed to modify config files")

	err = daNode.Start(ctx)
	s.Require().NoErrorf(err, "failed to re-start %s node", daNode.GetType().String())
}

// TestDANetworkCustomPorts tests the configuration of custom ports for DA nodes
func (s *DockerTestSuite) TestDANetworkCustomPorts() {
	if testing.Short() {
		s.T().Skip("skipping due to short mode")
	}

	ctx := context.Background()

	s.T().Run("test simple configuration with WithNonConflictingPorts", func(t *testing.T) {
		// Test the simple one-liner configuration
		provider := s.CreateDockerProvider(
			WithNonConflictingPorts(), // This should use ports 26668, 2131, 26667, 9091
		)

		chain, err := provider.GetChain(ctx)
		s.Require().NoError(err)

		err = chain.Start(ctx)
		s.Require().NoError(err)
		defer func() { _ = chain.Stop(ctx) }()

		daNetwork, err := provider.GetDataAvailabilityNetwork(ctx)
		s.Require().NoError(err)

		bridgeNodes := daNetwork.GetBridgeNodes()
		s.Require().Len(bridgeNodes, 1)

		bridgeNode := bridgeNodes[0]

		// Verify that internal addresses use the custom ports
		rpcAddr, err := bridgeNode.GetInternalRPCAddress()
		s.Require().NoError(err)
		s.Require().Contains(rpcAddr, ":26668", "RPC address should use custom port 26668")

		p2pAddr, err := bridgeNode.GetInternalP2PAddress()
		s.Require().NoError(err)
		s.Require().Contains(p2pAddr, ":2131", "P2P address should use custom port 2131")
	})

	s.T().Run("test custom ports setup with individual functions", func(t *testing.T) {
		// Test the custom ports configuration using individual functions
		provider := s.CreateDockerProvider(
			WithDANodePorts("27000", "3000"),
			WithDANodeCoreConnection("27001", "9095"),
		)

		chain, err := provider.GetChain(ctx)
		s.Require().NoError(err)

		err = chain.Start(ctx)
		s.Require().NoError(err)
		defer func() { _ = chain.Stop(ctx) }()

		daNetwork, err := provider.GetDataAvailabilityNetwork(ctx)
		s.Require().NoError(err)

		bridgeNodes := daNetwork.GetBridgeNodes()
		s.Require().Len(bridgeNodes, 1)

		bridgeNode := bridgeNodes[0]

		// Verify that internal addresses use the custom ports
		rpcAddr, err := bridgeNode.GetInternalRPCAddress()
		s.Require().NoError(err)
		s.Require().Contains(rpcAddr, ":27000", "RPC address should use custom port 27000")

		p2pAddr, err := bridgeNode.GetInternalP2PAddress()
		s.Require().NoError(err)
		s.Require().Contains(p2pAddr, ":3000", "P2P address should use custom port 3000")
	})

	s.T().Run("test per-node configuration with WithBridgeNodePorts", func(t *testing.T) {
		// Test per-node configuration
		provider := s.CreateDockerProvider(
			WithBridgeNodePorts(0, "28000", "4000"), // Configure bridge node 0 with specific ports
		)

		chain, err := provider.GetChain(ctx)
		s.Require().NoError(err)

		err = chain.Start(ctx)
		s.Require().NoError(err)
		defer func() { _ = chain.Stop(ctx) }()

		daNetwork, err := provider.GetDataAvailabilityNetwork(ctx)
		s.Require().NoError(err)

		bridgeNodes := daNetwork.GetBridgeNodes()
		s.Require().Len(bridgeNodes, 1)

		bridgeNode := bridgeNodes[0]

		// Verify that internal addresses use the per-node custom ports
		rpcAddr, err := bridgeNode.GetInternalRPCAddress()
		s.Require().NoError(err)
		s.Require().Contains(rpcAddr, ":28000", "RPC address should use per-node custom port 28000")

		p2pAddr, err := bridgeNode.GetInternalP2PAddress()
		s.Require().NoError(err)
		s.Require().Contains(p2pAddr, ":4000", "P2P address should use per-node custom port 4000")
	})

	s.T().Run("test backward compatibility - default behavior unchanged", func(t *testing.T) {
		// Test that existing code works unchanged
		provider := s.CreateDockerProvider() // No custom configuration

		chain, err := provider.GetChain(ctx)
		s.Require().NoError(err)

		err = chain.Start(ctx)
		s.Require().NoError(err)
		defer func() { _ = chain.Stop(ctx) }()

		daNetwork, err := provider.GetDataAvailabilityNetwork(ctx)
		s.Require().NoError(err)

		bridgeNodes := daNetwork.GetBridgeNodes()
		s.Require().Len(bridgeNodes, 1)

		bridgeNode := bridgeNodes[0]

		// Verify that internal addresses use the default ports
		rpcAddr, err := bridgeNode.GetInternalRPCAddress()
		s.Require().NoError(err)
		s.Require().Contains(rpcAddr, ":26658", "RPC address should use default port 26658")

		p2pAddr, err := bridgeNode.GetInternalP2PAddress()
		s.Require().NoError(err)
		s.Require().Contains(p2pAddr, ":2121", "P2P address should use default port 2121")
	})
}
