package docker

import (
	"context"
	"testing"

	"github.com/celestiaorg/tastora/framework/types"
)

// TestDANetworkCreation tests the creation of a DataAvailabilityNetwork with one of each type of node.
func (s *DockerTestSuite) TestDANetworkCreation() {
	if testing.Short() {
		s.T().Skip("skipping due to short mode")
	}
	s.T().Parallel()

	ctx := context.Background()
	var (
		bridgeNodes []types.DANode
		lightNodes  []types.DANode
	)

	// default configuration uses 1/1/1 for Bridge/Light/Full da nodes.
	daNetwork, err := s.provider.GetDataAvailabilityNetwork(ctx)
	s.Require().NoError(err)

	s.T().Run("da nodes can be created", func(t *testing.T) {
		bridgeNodes = daNetwork.GetBridgeNodes()
		s.Require().Len(bridgeNodes, 1)

		lightNodes = daNetwork.GetLightNodes()
		s.Require().Len(lightNodes, 1)
	})

	genesisHash := s.getGenesisHash(ctx)

	hostname, err := s.chain.GetNodes()[0].GetInternalHostName(ctx)
	s.Require().NoError(err, "failed to get internal hostname")

	bridgeNode := bridgeNodes[0]
	lightNode := lightNodes[0]

	chainID := s.chain.cfg.ChainConfig.ChainID

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

	s.T().Run("light node can be started", func(t *testing.T) {
		p2pInfo, err := bridgeNode.GetP2PInfo(ctx)
		s.Require().NoError(err, "failed to get bridge node p2p info")

		p2pAddr, err := p2pInfo.GetP2PAddress()
		s.Require().NoError(err, "failed to get bridge node p2p address")

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
