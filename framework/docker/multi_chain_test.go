package docker

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	reth "github.com/celestiaorg/tastora/framework/docker/evstack/reth"
	"github.com/stretchr/testify/require"
)

type MultiChainDeployment struct {
	Celestia *cosmos.Chain
	DA       *da.Network

	Reth1      *reth.Node
	Sequencer1 *evmsingle.Chain

	Reth2      *reth.Node
	Sequencer2 *evmsingle.Chain
}

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
