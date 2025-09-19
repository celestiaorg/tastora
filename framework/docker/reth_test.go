package docker

import (
	"math/big"
	"strings"
	"testing"
	"time"

	reth "github.com/celestiaorg/tastora/framework/docker/evstack/reth"
	"github.com/stretchr/testify/require"
)

// TestRethNode_LivenessAndGenesis verifies the first-class reth resource by
// starting a single node, asserting RPC liveness, chain ID, txpool API, and
// genesis hash consistency. Uses the shared setupDockerTest utilities.
func TestRethNode_LivenessAndGenesis(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	testCfg := setupDockerTest(t)

	// Build a 1-node Reth chain with a known-good genesis JSON
	builder := reth.NewChainBuilderWithTestName(t, testCfg.TestName).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithGenesis([]byte(reth.DefaultEvolveGenesisJSON())).
		WithNodes(reth.NewNodeConfigBuilder().Build())

	chain, err := builder.Build(testCfg.Ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = chain.Stop(testCfg.Ctx)
		_ = chain.Remove(testCfg.Ctx)
	})

	require.NoError(t, chain.Start(testCfg.Ctx))

	nodes := chain.Nodes()
	require.Len(t, nodes, 1)
	node := nodes[0]

	// Wait until ethclient is responsive
	require.Eventually(t, func() bool {
		ec, err := node.GetEthClient(testCfg.Ctx)
		if err != nil {
			return false
		}
		if _, err := ec.BlockNumber(testCfg.Ctx); err != nil {
			return false
		}
		return true
	}, 45*time.Second, 500*time.Millisecond, "reth JSON-RPC (ethclient) did not become ready")

	ec, err := node.GetEthClient(testCfg.Ctx)
	require.NoError(t, err)

	// Check chain ID from helper genesis (1234)
	cid, err := ec.ChainID(testCfg.Ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1234, cid.Int64(), "unexpected chain ID")

	// compare genesis header hash with helper method
	hdr0, err := ec.HeaderByNumber(testCfg.Ctx, big.NewInt(0))
	require.NoError(t, err)
	gh, err := node.GenesisHash(testCfg.Ctx)
	require.NoError(t, err)
	require.Equal(t, hdr0.Hash().Hex(), gh, "genesis hash mismatch")

	// client version should include "reth"
	rpcCl, err := node.GetRPCClient(testCfg.Ctx)
	require.NoError(t, err)
	var ver string
	require.NoError(t, rpcCl.CallContext(testCfg.Ctx, &ver, "web3_clientVersion"))
	require.Contains(t, strings.ToLower(ver), "reth", "unexpected client version: %s", ver)

	// txpool_status should be available and contain expected keys
	var status map[string]string
	require.NoError(t, rpcCl.CallContext(testCfg.Ctx, &status, "txpool_status"))
	_, hasPending := status["pending"]
	_, hasQueued := status["queued"]
	require.True(t, hasPending && hasQueued, "txpool_status missing keys: %+v", status)

	// stop and verify RPC stops responding
	require.NoError(t, chain.Stop(testCfg.Ctx))
	err = rpcCl.CallContext(testCfg.Ctx, &ver, "web3_clientVersion")
	require.Error(t, err, "expected RPC to fail after Stop")
}
