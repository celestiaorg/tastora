package otelcol

import (
	"context"
	"fmt"
	"sync"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type nodeType int

func (nodeType) String() string { return "otelcol" }

const (
	defaultOTLPGRPCPort = "4317"
	defaultOTLPHTTPPort = "4318"
)

const (
	homeDir        = "/home/otelcol"
	tracesRelPath  = "data/traces.json"
	metricsRelPath = "data/metrics.json"
)

type Config struct {
	Logger          *zap.Logger
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string
	Image           container.Image
	// ExportEndpoint is an optional OTLP gRPC endpoint (e.g. "jaeger:4317") to
	// forward traces to in addition to the file exporters. When empty, traces
	// and metrics are only written to files.
	ExportEndpoint string
}

type collectorConfig struct {
	Receivers  map[string]any `yaml:"receivers"`
	Processors map[string]any `yaml:"processors"`
	Connectors map[string]any `yaml:"connectors"`
	Exporters  map[string]any `yaml:"exporters"`
	Service    serviceConfig  `yaml:"service"`
}

type serviceConfig struct {
	Pipelines map[string]pipelineConfig `yaml:"pipelines"`
}

type pipelineConfig struct {
	Receivers  []string `yaml:"receivers"`
	Processors []string `yaml:"processors,omitempty"`
	Exporters  []string `yaml:"exporters"`
}

// buildConfigYAML generates the collector config. File exporters are always
// present so ReadTraces/ReadMetrics always work. When an export endpoint is
// set, an additional OTLP exporter forwards traces to that endpoint.
func buildConfigYAML(exportEndpoint string) ([]byte, error) {
	tracesPath := homeDir + "/" + tracesRelPath
	metricsPath := homeDir + "/" + metricsRelPath

	cfg := collectorConfig{
		Receivers: map[string]any{
			"otlp": map[string]any{
				"protocols": map[string]any{
					"grpc": map[string]any{"endpoint": "0.0.0.0:" + defaultOTLPGRPCPort},
					"http": map[string]any{"endpoint": "0.0.0.0:" + defaultOTLPHTTPPort},
				},
			},
		},
		Processors: map[string]any{
			"batch": nil,
		},
		Connectors: map[string]any{
			"spanmetrics": nil,
		},
		Exporters: map[string]any{
			"file/traces":  map[string]any{"path": tracesPath},
			"file/metrics": map[string]any{"path": metricsPath},
		},
		Service: serviceConfig{
			Pipelines: map[string]pipelineConfig{
				"traces": {
					Receivers:  []string{"otlp"},
					Processors: []string{"batch"},
					Exporters:  []string{"file/traces", "spanmetrics"},
				},
				"metrics": {
					Receivers: []string{"spanmetrics"},
					Exporters: []string{"file/metrics"},
				},
			},
		},
	}

	if exportEndpoint != "" {
		cfg.Exporters["otlp/forward"] = map[string]any{
			"endpoint": exportEndpoint,
			"tls":      map[string]any{"insecure": true},
		}
		tp := cfg.Service.Pipelines["traces"]
		tp.Exporters = append(tp.Exporters, "otlp/forward")
		cfg.Service.Pipelines["traces"] = tp
	}

	return yaml.Marshal(cfg)
}

type Node struct {
	*container.Node

	cfg     Config
	logger  *zap.Logger
	started bool
	mu      sync.Mutex

	internalPorts types.Ports
	externalPorts types.Ports

	Internal Scope
	External Scope
}

// Scope provides scoped (internal/external) access to collector OTLP endpoints.
type Scope struct {
	hostname func() string
	ports    *types.Ports
}

func (s Scope) OTLPGRPCEndpoint() string {
	return fmt.Sprintf("%s:%s", s.hostname(), s.ports.GRPC)
}

func (s Scope) OTLPHTTPEndpoint() string {
	return fmt.Sprintf("http://%s:%s", s.hostname(), s.ports.HTTP)
}

func New(ctx context.Context, cfg Config, testName string, index int) (*Node, error) {
	img := cfg.Image
	if img.Repository == "" {
		img = container.NewImage("otel/opentelemetry-collector-contrib", "0.120.0", "10001")
	}
	log := cfg.Logger.With(zap.String("component", "otelcol"), zap.Int("i", index))

	n := &Node{cfg: cfg, logger: log}
	n.Node = container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, img, homeDir, index, nodeType(0), log)
	n.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, n.Name()))

	if err := n.CreateAndSetupVolume(ctx, n.Name()); err != nil {
		return nil, err
	}

	configYAML, err := buildConfigYAML(cfg.ExportEndpoint)
	if err != nil {
		return nil, fmt.Errorf("building collector config: %w", err)
	}
	// seed the volume with the config and empty export files so the file
	// exporter can open them on startup.
	for relPath, content := range map[string][]byte{
		"config.yaml":  configYAML,
		tracesRelPath:  {},
		metricsRelPath: {},
	} {
		if err := n.WriteFile(ctx, relPath, content); err != nil {
			return nil, fmt.Errorf("writing %s: %w", relPath, err)
		}
	}

	n.Internal = Scope{hostname: func() string { return n.Name() }, ports: &n.internalPorts}
	n.External = Scope{hostname: func() string { return "0.0.0.0" }, ports: &n.externalPorts}
	return n, nil
}

func (n *Node) Name() string {
	return fmt.Sprintf("otelcol-%d-%s", n.Index, internal.SanitizeDockerResourceName(n.TestName))
}

func (n *Node) HostName() string {
	return internal.CondenseHostName(n.Name())
}

func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.started {
		return n.StartContainer(ctx)
	}
	if err := n.createContainer(ctx); err != nil {
		return err
	}
	if err := n.ContainerLifecycle.StartContainer(ctx); err != nil {
		return err
	}
	hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, n.internalPorts.GRPC+"/tcp", n.internalPorts.HTTP+"/tcp")
	if err != nil {
		return err
	}
	n.externalPorts = types.Ports{
		GRPC: internal.MustExtractPort(hostPorts[0]),
		HTTP: internal.MustExtractPort(hostPorts[1]),
	}
	n.started = true
	return nil
}

func (n *Node) createContainer(ctx context.Context) error {
	if n.internalPorts.GRPC == "" {
		n.internalPorts.GRPC = defaultOTLPGRPCPort
	}
	if n.internalPorts.HTTP == "" {
		n.internalPorts.HTTP = defaultOTLPHTTPPort
	}

	ports := nat.PortMap{
		nat.Port(n.internalPorts.GRPC + "/tcp"): {},
		nat.Port(n.internalPorts.HTTP + "/tcp"): {},
	}
	cmd := []string{"--config", homeDir + "/config.yaml"}
	return n.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, ports, "", n.Bind(), nil, n.HostName(), cmd, nil, nil)
}

func (n *Node) GetNetworkInfo(ctx context.Context) (types.NetworkInfo, error) {
	internalIP, err := internal.GetContainerInternalIP(ctx, n.DockerClient, n.ContainerLifecycle.ContainerID())
	if err != nil {
		return types.NetworkInfo{}, err
	}
	return types.NetworkInfo{
		Internal: types.Network{
			Hostname: n.HostName(),
			IP:       internalIP,
			Ports:    n.internalPorts,
		},
		External: types.Network{
			Hostname: "0.0.0.0",
			Ports:    n.externalPorts,
		},
	}, nil
}

// ReadTraces reads the exported traces JSON file from the container volume.
// Only usable when the collector config includes a file/traces exporter writing
// to the default path.
func (n *Node) ReadTraces(ctx context.Context) ([]byte, error) {
	return n.ReadFile(ctx, tracesRelPath)
}

// ReadMetrics reads the exported metrics JSON file from the container volume.
// Only usable when the collector config includes a file/metrics exporter writing
// to the default path.
func (n *Node) ReadMetrics(ctx context.Context) ([]byte, error) {
	return n.ReadFile(ctx, metricsRelPath)
}
