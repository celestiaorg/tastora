package docker

import (
	"context"
	"fmt"
	"testing"
	"time"

	evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	"github.com/celestiaorg/tastora/framework/docker/evstack/spamoor"
	"github.com/celestiaorg/tastora/framework/docker/jaeger"
	"github.com/celestiaorg/tastora/framework/testutil/deploy"
	"github.com/stretchr/testify/require"
)

// TestTracingWithJaegerBackend starts a minimal stack (reth+evmsingle), wires up telemetry
// and collects with Jaeger.
func TestTracingWithJaegerBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	const evNodeServiceName = "ev-node"
	// ev-reth hard codes this, we cannot change it.
	// const rethServiceName = "ev-reth"

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

	t.Cleanup(func() {
		_ = j.Remove(testCfg.Ctx)
	})

	// Start reth using generic OTEL envs, NOTE: won't have any impact until there is a tagged release built with support.
	rnode, err := testCfg.RethBuilder.
		WithEnv(
			// Use OTLP/HTTP with explicit traces path per Rust exporter expectations
			"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT="+j.IngestHTTPEndpoint()+"/v1/traces",
			"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=http/protobuf",
			"RUST_LOG=info",
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
		WithInstrumentationTracing(j.IngestHTTPEndpoint(), evNodeServiceName, "1").
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

	hasService, err := j.HasService(ctx, evNodeServiceName, time.Second*5)
	require.NoError(t, err)
	require.True(t, hasService)

	traces, err := j.Traces(ctx, evNodeServiceName, 10)
	require.NoError(t, err)
	require.Greater(t, len(traces), 0, "jaeger should contain traces for "+evNodeServiceName)

	/*
		// TODO: ev-reth does not yet have a tag with OTLP enabled.
		// Verify reth traces also arrive
		hasReth, err := j.HasService(ctx, rethServiceName, time.Second*5)
		require.NoError(t, err)
		// Log services again before asserting to aid debugging
		if svcs, err := j.Services(ctx); err == nil {
			t.Logf("Jaeger services before reth assert: %v", svcs)
		}
		require.True(t, hasReth)
		rethTraces, err := j.Traces(ctx, rethServiceName, 10)
		require.NoError(t, err)
		require.Greater(t, len(rethTraces), 0, "jaeger should contain traces for "+rethServiceName)
	*/

}
