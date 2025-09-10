package docker

import (
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

	chain, err := testCfg.ChainBuilder.Build(testCfg.Ctx)
	require.NoError(t, err)

	err = chain.Start(testCfg.Ctx)
	require.NoError(t, err)

	require.NoError(t, wait.ForBlocks(testCfg.Ctx, 5, chain))

	err = chain.UpgradeVersion(testCfg.Ctx, "v4.0.2-mocha")
	require.NoError(t, err)

	// chain is producing blocks at the next version
	err = wait.ForBlocks(testCfg.Ctx, 2, chain)
	require.NoError(t, err)

	validatorNode := chain.GetNodes()[0]

	rpcClient, err := validatorNode.GetRPCClient()
	require.NoError(t, err, "failed to get RPC client for version check")

	abciInfo, err := rpcClient.ABCIInfo(testCfg.Ctx)
	require.NoError(t, err, "failed to fetch ABCI info")
	require.Equal(t, "4.0.2-mocha", abciInfo.Response.GetVersion(), "version mismatch")
	require.Equal(t, uint64(4), abciInfo.Response.GetAppVersion(), "app_version mismatch")
}
