package docker

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMultiChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	// provision the full stack with defaults.
	testCfg := setupDockerTest(t)
	stack, err := DeployMultiChainStack(t, testCfg)
	require.NoError(t, err)

	ctx := context.Background()
	evnodes := stack.EvmSeq1.Nodes()
	require.Len(t, evnodes, 1)

	networkInfo, err := evnodes[0].GetNetworkInfo(ctx)
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
