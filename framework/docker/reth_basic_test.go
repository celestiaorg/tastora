package docker

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net"
    "net/http"
    "testing"
    "time"

    reth "github.com/celestiaorg/tastora/framework/docker/evstack/reth"
    "github.com/stretchr/testify/require"
)

// TestRethChain_BasicStart attempts to start a single-node Reth chain using the built-in dev chain,
// then checks JSON-RPC liveness via web3_clientVersion.
func TestRethChain_BasicStart(t *testing.T) {
    testCfg := setupDockerTest(t)

    builder := reth.NewChainBuilder(t).
        WithDockerClient(testCfg.DockerClient).
        WithDockerNetworkID(testCfg.NetworkID).
        WithNodes(
            reth.NewNodeConfigBuilder().Build(),
            reth.NewNodeConfigBuilder().Build(),
            reth.NewNodeConfigBuilder().Build(),
        )

    chain := builder.Build()

    ctx, cancel := context.WithCancel(testCfg.Ctx)
    defer cancel()

    // Ensure cleanup regardless of test outcome
    t.Cleanup(func() {
        _ = chain.StopAll(ctx)
        _ = chain.RemoveAll(ctx)
    })

    require.NoError(t, chain.StartAll(ctx))

    nodes := chain.GetNodes()
    require.Len(t, nodes, 3)

    // For each node, check JSON-RPC readiness, Engine TCP liveness, and Metrics availability
    for i := range nodes {
        ni, err := nodes[i].GetNetworkInfo(ctx)
        require.NoError(t, err)
        require.NotEmpty(t, ni.External.Ports.RPC)
        require.NotEmpty(t, ni.External.Ports.Engine)
        require.NotEmpty(t, ni.External.Ports.Metrics)

        // JSON-RPC liveness via web3_clientVersion
        rpcURL := fmt.Sprintf("http://0.0.0.0:%s", ni.External.Ports.RPC)
        require.Eventually(t, func() bool {
            reqBody := map[string]any{
                "jsonrpc": "2.0",
                "id":      1,
                "method":  "web3_clientVersion",
                "params":  []any{},
            }
            b, _ := json.Marshal(reqBody)
            req, _ := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(b))
            req.Header.Set("Content-Type", "application/json")

            resp, err := http.DefaultClient.Do(req)
            if err != nil { return false }
            defer func() { _ = resp.Body.Close() }()
            return resp.StatusCode == http.StatusOK
        }, 30*time.Second, 1*time.Second, "reth JSON-RPC did not become ready for node %d", i)

        // Engine/AuthRPC TCP liveness (port open)
        engineAddr := fmt.Sprintf("0.0.0.0:%s", ni.External.Ports.Engine)
        require.Eventually(t, func() bool {
            d, err := net.DialTimeout("tcp", engineAddr, 2*time.Second)
            if err != nil { return false }
            _ = d.Close()
            return true
        }, 30*time.Second, 1*time.Second, "reth engine port did not open for node %d", i)

        // Metrics endpoint readiness (HTTP 200)
        metricsURL := fmt.Sprintf("http://0.0.0.0:%s/metrics", ni.External.Ports.Metrics)
        require.Eventually(t, func() bool {
            req, _ := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
            resp, err := http.DefaultClient.Do(req)
            if err != nil { return false }
            defer func() { _ = resp.Body.Close() }()
            return resp.StatusCode == http.StatusOK
        }, 30*time.Second, 1*time.Second, "reth metrics endpoint did not become ready for node %d", i)
    }
}
