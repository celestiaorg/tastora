package docker

import (
	"github.com/celestiaorg/tastora/framework/testutil/wait"
)

// TestUpgradeVersion verifies that you can upgrade from one tag to another.
func (s *DockerTestSuite) TestUpgradeVersion() {
	var err error
	s.provider = s.CreateDockerProvider()
	s.chain, err = s.provider.GetChain(s.ctx)
	s.Require().NoError(err)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	s.Require().NoError(wait.ForBlocks(s.ctx, 5, s.chain))

	err = s.chain.Stop(s.ctx)
	s.Require().NoError(err)

	s.chain.UpgradeVersion(s.ctx, "v4.0.2-mocha")

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	// chain is producing blocks at the next version
	err = wait.ForBlocks(s.ctx, 2, s.chain)
	s.Require().NoError(err)

	validatorNode := s.chain.GetNodes()[0]

	rpcClient, err := validatorNode.GetRPCClient()
	s.Require().NoError(err, "failed to get RPC client for version check")

	abciInfo, err := rpcClient.ABCIInfo(s.ctx)
	s.Require().NoError(err, "failed to fetch ABCI info")
	s.Require().Equal("4.0.2-mocha", abciInfo.Response.GetVersion(), "version mismatch")
	s.Require().Equal(uint64(4), abciInfo.Response.GetAppVersion(), "app_version mismatch")
}
