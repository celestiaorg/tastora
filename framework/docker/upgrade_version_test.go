package docker

import (
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestUpgradeVersion verifies that you can upgrade from one tag to another.
func TestUpgradeVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	// Setup isolated docker environment for this test
	testCfg := setupDockerTest(t)

	chain, err := testCfg.ChainBuilder.
		WithImage(container.NewImage("ghcr.io/celestiaorg/celestia-app", "v5.0.9", "10001:10001")).
		Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	require.NoError(t, wait.ForBlocks(testCfg.Ctx, 5, chain))

	err = chain.UpgradeVersion(testCfg.Ctx, "v5.0.10")
	require.NoError(t, err)

	// chain is producing blocks at the next version
	err = wait.ForBlocks(testCfg.Ctx, 2, chain)
	require.NoError(t, err)

	validatorNode := chain.GetNodes()[0]

	rpcClient, err := validatorNode.GetRPCClient()
	require.NoError(t, err, "failed to get RPC client for version check")

	abciInfo, err := rpcClient.ABCIInfo(testCfg.Ctx)
	require.NoError(t, err, "failed to fetch ABCI info")
	require.Equal(t, "5.0.10", abciInfo.Response.GetVersion(), "version mismatch")
	require.Equal(t, uint64(5), abciInfo.Response.GetAppVersion(), "app_version mismatch")
}
