package docker

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	"github.com/celestiaorg/tastora/framework/docker/evstack/spamoor"
	otel "github.com/celestiaorg/tastora/framework/docker/otel"
	"github.com/celestiaorg/tastora/framework/testutil/deploy"
	"github.com/stretchr/testify/require"
)

// TestOTELCollector_StackAndMetrics starts a minimal stack (reth+evmsingle), spamoor, and an OTEL collector,
// then verifies the collector's Prometheus metrics endpoint is reachable and emits uptime.
func TestOTELCollector_StackAndMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	testCfg := setupDockerTest(t)

	// Bring up Celestia + DA using configured builders
	celestia, danet, err := deploy.CelestiaWithDA(testCfg.Ctx, testCfg.ChainBuilder, testCfg.DANetworkBuilder)
	require.NoError(t, err)
	t.Cleanup(func() {
		//_ = danet.Stop(testCfg.Ctx)
		//_ = danet.Remove(testCfg.Ctx)
		_ = celestia.Stop(testCfg.Ctx)
		_ = celestia.Remove(testCfg.Ctx)
	})

	// Start OTEL collector with default minimal config (includes telemetry metrics on 8888)
	collector, err := otel.NewCollector(testCfg.Ctx, otel.Config{
		DockerClient:    testCfg.DockerClient,
		DockerNetworkID: testCfg.NetworkID,
		Logger:          testCfg.Logger,
		ConfigMap:       otel.MinimalLoggingConfigMap(),
	}, testCfg.TestName, 0)
	require.NoError(t, err)
	require.NoError(t, collector.Start(testCfg.Ctx))

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
		_ = spamNode.Stop(testCfg.Ctx)
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
		WithInstrumentationTracing(collector.GRPCEndpoint(), "ev-node-smoke", "1.0").
		Build()

	newEVM, err := evmsingle.NewChainBuilderWithTestName(t, testCfg.TestName).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithNodes(evNodeCfg).
		Build(testCfg.Ctx)
	require.NoError(t, err)
	require.NoError(t, newEVM.Start(testCfg.Ctx))
	t.Cleanup(func() {
		_ = newEVM.Stop(testCfg.Ctx)
		_ = newEVM.Remove(testCfg.Ctx)
	})

	// Verify collector metrics endpoint responds and includes uptime metric
	client := &http.Client{Timeout: 1 * time.Second}
	url := collector.MetricsHostURL()
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if strings.Contains(string(b), "otelcol_process_uptime") {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("collector metrics endpoint not ready or missing uptime metric at %s", url)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
