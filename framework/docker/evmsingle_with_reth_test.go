package docker

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestEvmSingle_WithReth starts a single reth node and an ev-node-evm-single
// configured to talk to it via Engine/RPC, then checks basic liveness.
func TestEvmSingle_WithReth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	// provision the full stack with defaults.
	testCfg := setupDockerTest(t)
	stack, err := DeployMinimalStack(t, testCfg)
	require.NoError(t, err)

	ctx := context.Background()
	enodes := stack.EVM.Nodes()
	require.Len(t, enodes, 1)

	networkInfo, err := enodes[0].GetNetworkInfo(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, networkInfo.External.Ports.RPC)

	healthURL := fmt.Sprintf("http://0.0.0.0:%s/health/ready", networkInfo.External.Ports.RPC)
	require.Eventually(t, func() bool {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode == http.StatusOK
	}, 60*time.Second, 2*time.Second, "evm-single did not become healthy")
}
