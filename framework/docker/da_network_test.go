package docker

import (
	"context"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/types"
	"testing"
)

// TestDANetworkCreation tests the creation of a DataAvailabilityNetwork with one of each type of node.
func (s *DockerTestSuite) TestDANetworkCreation() {
	if testing.Short() {
		s.T().Skip("skipping due to short mode")
	}

	ctx := context.Background()

	var err error
	s.provider = s.CreateDockerProvider()
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

	chainID := "test"

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

	chainID := "test"

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
