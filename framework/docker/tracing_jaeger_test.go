package docker

import (
	"context"
	"fmt"
	"testing"
	"time"

	evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	"github.com/celestiaorg/tastora/framework/docker/evstack/spamoor"
	"github.com/celestiaorg/tastora/framework/docker/jaeger"
	"github.com/celestiaorg/tastora/framework/docker/otelcol"
	"github.com/celestiaorg/tastora/framework/testutil/deploy"
	"github.com/stretchr/testify/require"
)

// TestTracingWithJaegerBackend starts a minimal stack (reth+evmsingle), routes
// telemetry through an OpenTelemetry Collector that forwards to Jaeger, and
// verifies traces arrive in both the collector's file export and in Jaeger.
func TestTracingWithJaegerBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	const evNodeServiceName = "ev-node"
	// ev-reth hard codes this, we cannot change it.
	const rethServiceName = "ev-reth"

	testCfg := setupDockerTest(t)

	// Bring up Celestia + DA using configured builders
	celestia, danet, err := deploy.CelestiaWithDA(testCfg.Ctx, testCfg.ChainBuilder, testCfg.DANetworkBuilder)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = danet.Remove(testCfg.Ctx)
		_ = celestia.Remove(testCfg.Ctx)
	})

	// Start Jaeger all-in-one as the final tracing backend
	j, err := jaeger.New(testCfg.Ctx, jaeger.Config{
		Logger:          testCfg.Logger,
		DockerClient:    testCfg.DockerClient,
		DockerNetworkID: testCfg.NetworkID,
	}, testCfg.TestName, 0)
	require.NoError(t, err)
	require.NoError(t, j.Start(testCfg.Ctx))
	t.Cleanup(func() {
		_ = j.Remove(testCfg.Ctx)
	})

	// Start the OTel Collector, forwarding traces to Jaeger
	col, err := otelcol.New(testCfg.Ctx, otelcol.Config{
		Logger:          testCfg.Logger,
		DockerClient:    testCfg.DockerClient,
		DockerNetworkID: testCfg.NetworkID,
		ExportEndpoint:  j.Internal.IngestGRPCEndpoint(),
	}, testCfg.TestName, 0)
	require.NoError(t, err)
	require.NoError(t, col.Start(testCfg.Ctx))
	t.Cleanup(func() {
		_ = col.Remove(testCfg.Ctx)
	})

	// Start reth, pointing OTLP at the collector
	rnode, err := testCfg.RethBuilder.
		WithEnv(
			"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="+col.Internal.OTLPHTTPEndpoint()+"/v1/traces",
			"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=http",
			"RUST_LOG=info",
			"OTEL_SDK_DISABLED=false",
		).
		Build(testCfg.Ctx)

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

	// Build evm chain with instrumentation flags pointing at the collector
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
		WithInstrumentationTracing(col.Internal.OTLPHTTPEndpoint(), evNodeServiceName, "1").
		Build()

	evm, err := evmsingle.NewChainBuilderWithTestName(t, testCfg.TestName).
		WithDockerClient(testCfg.DockerClient).
		WithDockerNetworkID(testCfg.NetworkID).
		WithNodes(evNodeCfg).
		Build(testCfg.Ctx)
	require.NoError(t, err)
	require.NoError(t, evm.Start(testCfg.Ctx))
	t.Cleanup(func() {
		_ = evm.Remove(testCfg.Ctx)
	})

	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(1*time.Minute))
	defer cancel()

	// Verify traces forwarded from the collector arrive in Jaeger
	hasService, err := j.External.HasService(ctx, evNodeServiceName, time.Second*5)
	require.NoError(t, err)
	require.True(t, hasService)

	traces, err := j.External.Traces(ctx, evNodeServiceName, 10)
	require.NoError(t, err)
	require.Greater(t, len(traces), 0, "jaeger should contain traces for "+evNodeServiceName)

	// Verify reth traces also arrive via the collector
	hasReth, err := j.External.HasService(ctx, rethServiceName, time.Second*5)
	require.NoError(t, err)
	if svcs, err := j.External.Services(ctx); err == nil {
		t.Logf("Jaeger services before reth assert: %v", svcs)
	}
	require.True(t, hasReth)
	rethTraces, err := j.External.Traces(ctx, rethServiceName, 10)
	require.NoError(t, err)
	require.Greater(t, len(rethTraces), 0, "jaeger should contain traces for "+rethServiceName)

	// Verify the collector's file exporter also captured traces
	traceData, err := col.ReadTraces(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, traceData, "collector file export should contain trace data")
}
