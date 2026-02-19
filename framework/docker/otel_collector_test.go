package docker

import (
    "encoding/json"
    "fmt"
    "net/http"
    "testing"
    "time"

    evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
    "github.com/celestiaorg/tastora/framework/docker/evstack/spamoor"
    "github.com/celestiaorg/tastora/framework/docker/jaeger"
    "github.com/celestiaorg/tastora/framework/testutil/deploy"
    "github.com/stretchr/testify/require"
)

// TestOTELCollector_StackAndMetrics starts a minimal stack (reth+evmsingle), spamoor, and an OTEL collector,
// then verifies the collector's Prometheus metrics endpoint is reachable and emits uptime.
func TestTracingWithJaegerBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	testCfg := setupDockerTest(t)

	// Bring up Celestia + DA using configured builders
	celestia, danet, err := deploy.CelestiaWithDA(testCfg.Ctx, testCfg.ChainBuilder, testCfg.DANetworkBuilder)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = danet.Remove(testCfg.Ctx)
		_ = celestia.Remove(testCfg.Ctx)
	})

    // Start Jaeger all-in-one as the tracing backend
    j, err := jaeger.New(testCfg.Ctx, jaeger.Config{
        Logger:          testCfg.Logger,
        DockerClient:    testCfg.DockerClient,
        DockerNetworkID: testCfg.NetworkID,
    }, testCfg.TestName, 0)
    require.NoError(t, err)
    require.NoError(t, j.Start(testCfg.Ctx))
    t.Cleanup(func() { _ = j.Stop(testCfg.Ctx); _ = j.Remove(testCfg.Ctx) })

	// Start reth using the pre-configured builder
	rnode, err := testCfg.RethBuilder.Build(testCfg.Ctx)
	require.NoError(t, err)
	require.NoError(t, rnode.Start(testCfg.Ctx))
	t.Cleanup(func() {
		_ = rnode.Stop(testCfg.Ctx)
		_ = rnode.Remove(testCfg.Ctx)
	})

	// Wait briefly to allow EL to be fully ready
	time.Sleep(2 * time.Second)

	rni, err := rnode.GetNetworkInfo(testCfg.Ctx)
	require.NoError(t, err)
	rpcInternal := fmt.Sprintf("http://%s:%s", rni.Internal.Hostname, rni.Internal.Ports.RPC)

	spam := spamoor.NewNodeBuilder(testCfg.TestName).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithLogger(testCfg.Logger).
		WithRPCHosts(rpcInternal).
		// Use the hard-coded key associated with 0xaF9053bB6c... in the default reth genesis
		WithPrivateKey("0x82bfcfadbf1712f6550d8d2c00a39f05b33ec78939d0167be2a737d691f33a6a")
	spamNode, err := spam.Build(testCfg.Ctx)
	require.NoError(t, err)
	require.NoError(t, spamNode.Start(testCfg.Ctx))
	t.Cleanup(func() {
		_ = spamNode.Remove(testCfg.Ctx)
	})

	// Build evm chain with instrumentation flags (first and only evm)
	// Wire DA address like WithDefaults does
	bni, err := danet.GetBridgeNodes()[0].GetNetworkInfo(testCfg.Ctx)
	require.NoError(t, err)
	daAddress := fmt.Sprintf("http://%s:%s", bni.Internal.IP, bni.Internal.Ports.RPC)
    evNodeCfg := evmsingle.NewNodeConfigBuilder().
        WithEVMEngineURL(fmt.Sprintf("http://%s:%s", rni.Internal.Hostname, rni.Internal.Ports.Engine)).
        WithEVMETHURL(fmt.Sprintf("http://%s:%s", rni.Internal.Hostname, rni.Internal.Ports.RPC)).
        WithEVMJWTSecret(rnode.JWTSecretHex()).
        WithEVMGenesisHash(func() string { h, _ := rnode.GenesisHash(testCfg.Ctx); return h }()).
        WithEVMBlockTime("1s").
        WithEVMSignerPassphrase("secret").
        WithDAAddress(daAddress).
        // Send spans directly to Jaeger OTLP/HTTP
        WithInstrumentationTracing(j.IngestHTTPEndpoint(), "ev-node-smoke", "1").
        Build()

	newEVM, err := evmsingle.NewChainBuilderWithTestName(t, testCfg.TestName).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		// Inject OpenTelemetry SDK env vars to ensure ev-node exports traces
		//WithEnv(otel.EnvForService("ev-node-smoke", collector, "grpc")...).
		WithNodes(evNodeCfg).
		Build(testCfg.Ctx)
	require.NoError(t, err)
	require.NoError(t, newEVM.Start(testCfg.Ctx))
	t.Cleanup(func() {
		_ = newEVM.Remove(testCfg.Ctx)
	})

    // Verify by querying Jaeger API for service and traces
    client := &http.Client{Timeout: 1200 * time.Millisecond}
    deadline := time.Now().Add(60 * time.Second)

    // Wait for service registration
    svcURL := j.QueryHostURL() + "/api/services"
    seen := false
    for time.Now().Before(deadline) && !seen {
        resp, err := client.Get(svcURL)
        if err == nil && resp.StatusCode == http.StatusOK {
            var out struct{ Data []string `json:"data"` }
            _ = json.NewDecoder(resp.Body).Decode(&out)
            _ = resp.Body.Close()
            for _, s := range out.Data {
                if s == "ev-node-smoke" {
                    seen = true
                    break
                }
            }
        }
        time.Sleep(1 * time.Second)
    }
    require.True(t, seen, "jaeger should list ev-node-smoke service")

    // Query traces for the service
    haveTraces := false
    tracesURL := j.QueryHostURL() + "/api/traces?service=ev-node-smoke&limit=5"
    for time.Now().Before(deadline) && !haveTraces {
        resp, err := client.Get(tracesURL)
        if err == nil && resp.StatusCode == http.StatusOK {
            var out struct{ Data []any `json:"data"` }
            _ = json.NewDecoder(resp.Body).Decode(&out)
            _ = resp.Body.Close()
            if len(out.Data) > 0 {
                haveTraces = true
                break
            }
        }
        time.Sleep(1 * time.Second)
    }
    require.True(t, haveTraces, "jaeger should contain traces for ev-node-smoke")
}
