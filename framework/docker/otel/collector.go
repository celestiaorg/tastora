package otel

import (
	"context"
	"fmt"
	"path"
	"sync"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

// NodeType for logging/identification
type nodeType int

func (nodeType) String() string { return "otel-collector" }

// Ports exposed by the collector
type Ports struct {
    OTLPGRPC string // default: 4317
    OTLPHTTP string // default: 4318
    Metrics  string // default: 8888 (service.telemetry.metrics)
}

func defaultInternalPorts() Ports { return Ports{OTLPGRPC: "4317", OTLPHTTP: "4318", Metrics: "8888"} }

// Config defines the OTEL collector runtime parameters
type Config struct {
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string
	Logger          *zap.Logger
	Image           container.Image

    // Config expressed as a generic map. If nil, a minimal logging config is used.
    // The map is marshaled to YAML when written into the container.
    ConfigMap map[string]any

    // Optional upstream OTLP exporter (traces) to fan-out spans to a real backend.
    // If set, the collector will export traces to this endpoint in addition to the local debug exporter.
    // Assumes plaintext gRPC; set endpoints like "jaeger-collector:4317" or another collector.
    UpstreamOTLPEndpoint string
}

// Node represents a running OpenTelemetry Collector in Docker
type Node struct {
    *container.Node

    cfg      Config
    logger   *zap.Logger
    started  bool
    mu       sync.Mutex
    external types.Ports // HTTP: 4318, GRPC: 4317
}

// NewCollector creates a new collector node
func NewCollector(ctx context.Context, cfg Config, testName string, index int) (*Node, error) {
	log := cfg.Logger.With(zap.String("component", "otel-collector"), zap.Int("i", index))
	home := "/otelcol"
	// Default image if not provided
	img := cfg.Image
    if img.Repository == "" {
        // Pin to a known-good version to avoid config drift; collector runs as non-root.
        img = container.NewImage("otel/opentelemetry-collector-contrib", "0.146.1", "10001:10001")
    }

	n := &Node{cfg: cfg, logger: log}
	n.Node = container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, img, home, index, nodeType(0), log)
	n.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, n.Name()))

	if err := n.CreateAndSetupVolume(ctx, n.Name()); err != nil {
		return nil, err
	}
	return n, nil
}

// Name returns a stable container name
func (n *Node) Name() string {
	return fmt.Sprintf("otel-collector-%d-%s", n.Index, internal.SanitizeDockerResourceName(n.TestName))
}

// HostName returns a condensed hostname (used for in-network endpoints)
func (n *Node) HostName() string { return internal.CondenseHostName(n.Name()) }

// Start writes config, creates, and starts the collector
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.started {
		return n.StartContainer(ctx)
	}

	if err := n.writeConfig(ctx); err != nil {
		return err
	}

	if err := n.createContainer(ctx); err != nil {
		return err
	}

	if err := n.ContainerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	// resolve host-mapped ports
	p := defaultInternalPorts()
    hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, p.OTLPGRPC+"/tcp", p.OTLPHTTP+"/tcp", p.Metrics+"/tcp")
    if err != nil {
        return err
    }
    n.external = types.Ports{RPC: internal.MustExtractPort(hostPorts[0]), HTTP: internal.MustExtractPort(hostPorts[1]), Metrics: internal.MustExtractPort(hostPorts[2])}
	n.started = true
	return nil
}

func (n *Node) GRPCEndpoint() string {
	return fmt.Sprintf("%s:%s", n.HostName(), defaultInternalPorts().OTLPGRPC)
}

func (n *Node) HTTPEndpoint() string {
	return fmt.Sprintf("http://%s:%s", n.HostName(), defaultInternalPorts().OTLPHTTP)
}

// MetricsHostURL returns the host-mapped Prometheus metrics endpoint
func (n *Node) MetricsHostURL() string {
    return fmt.Sprintf("http://127.0.0.1:%s/metrics", n.external.Metrics)
}

// (No app metrics port; only collector self-telemetry on MetricsHostURL)

func (n *Node) writeConfig(ctx context.Context) error {
    // Use provided map, otherwise fall back to minimal config map
    var cfg map[string]any
    if n.cfg.ConfigMap != nil {
        cfg = n.cfg.ConfigMap
    } else {
        cfg = MinimalLoggingConfigMap()
    }
    // If an upstream OTLP endpoint is provided, add an exporter and include it in the traces pipeline.
    if n.cfg.UpstreamOTLPEndpoint != "" {
        // exporters.otlp/upstream
        ensureMap := func(m map[string]any, key string) map[string]any {
            if v, ok := m[key]; ok {
                if mm, ok2 := v.(map[string]any); ok2 {
                    return mm
                }
            }
            mm := map[string]any{}
            m[key] = mm
            return mm
        }
        exporters := ensureMap(cfg, "exporters")
        exporters["otlp/upstream"] = map[string]any{
            "endpoint": n.cfg.UpstreamOTLPEndpoint,
            "tls": map[string]any{"insecure": true},
        }
        service := ensureMap(cfg, "service")
        pipelines := ensureMap(service, "pipelines")
        if tracesRaw, ok := pipelines["traces"]; ok {
            if traces, ok2 := tracesRaw.(map[string]any); ok2 {
                if exRaw, ok3 := traces["exporters"]; ok3 {
                    if ex, ok4 := exRaw.([]string); ok4 {
                        traces["exporters"] = append(ex, "otlp/upstream")
                    }
                }
            }
        }
    }
    bz, err := yaml.Marshal(cfg)
    if err != nil {
        return err
    }
    return n.WriteFile(ctx, path.Join("config", "collector.yaml"), bz)
}

func (n *Node) createContainer(ctx context.Context) error {
	p := defaultInternalPorts()
    ports := nat.PortMap{
        nat.Port(p.OTLPGRPC + "/tcp"): {},
        nat.Port(p.OTLPHTTP + "/tcp"): {},
        nat.Port(p.Metrics + "/tcp"):  {},
    }

	cmd := []string{
		"--config", path.Join(n.HomeDir(), "config", "collector.yaml"),
	}

	return n.CreateContainer(
		ctx,
		n.TestName,
		n.NetworkID,
		n.Image,
		ports,
		"",
		n.Bind(),
		nil,
		n.HostName(),
		cmd,
		nil,
		[]string{"/otelcol-contrib"},
	)
}
