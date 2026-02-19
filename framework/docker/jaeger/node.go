package jaeger

import (
    "context"
    "fmt"
    "sync"

    "github.com/celestiaorg/tastora/framework/docker/container"
    "github.com/celestiaorg/tastora/framework/docker/internal"
    "github.com/celestiaorg/tastora/framework/types"
    "github.com/docker/go-connections/nat"
    "go.uber.org/zap"
)

type nodeType int

func (nodeType) String() string { return "jaeger" }

// Ports used by Jaeger all-in-one
type Ports struct {
    OTLPGRPC string // 4317
    OTLPHTTP string // 4318
    Query    string // 16686
}

func defaultPorts() Ports { return Ports{OTLPGRPC: "4317", OTLPHTTP: "4318", Query: "16686"} }

type Config struct {
    Logger          *zap.Logger
    DockerClient    types.TastoraDockerClient
    DockerNetworkID string
    Image           container.Image
}

// Node represents a Jaeger all-in-one container
type Node struct {
    *container.Node

    cfg      Config
    logger   *zap.Logger
    started  bool
    mu       sync.Mutex
    external types.Ports // RPC->4317, HTTP->4318, Metrics->16686 (we'll use Metrics field to store query port)
}

// New creates a new Jaeger node (not started)
func New(ctx context.Context, cfg Config, testName string, index int) (*Node, error) {
    img := cfg.Image
    if img.Repository == "" {
        img = container.NewImage("jaegertracing/all-in-one", "1.57", "")
    }
    log := cfg.Logger.With(zap.String("component", "jaeger"), zap.Int("i", index))
    home := "/home/jaeger"
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
    return fmt.Sprintf("jaeger-%d-%s", n.Index, internal.SanitizeDockerResourceName(n.TestName))
}

// HostName returns a condensed hostname
func (n *Node) HostName() string { return internal.CondenseHostName(n.Name()) }

// Start creates and starts the container
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
    p := defaultPorts()
    hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, p.OTLPGRPC+"/tcp", p.OTLPHTTP+"/tcp", p.Query+"/tcp")
    if err != nil {
        return err
    }
    n.external = types.Ports{RPC: internal.MustExtractPort(hostPorts[0]), HTTP: internal.MustExtractPort(hostPorts[1]), Metrics: internal.MustExtractPort(hostPorts[2])}
    n.started = true
    return nil
}

func (n *Node) createContainer(ctx context.Context) error {
    p := defaultPorts()
    ports := nat.PortMap{
        nat.Port(p.OTLPGRPC + "/tcp"): {},
        nat.Port(p.OTLPHTTP + "/tcp"): {},
        nat.Port(p.Query + "/tcp"):    {},
    }
    // all-in-one entrypoint default is fine; no args required for OTLP ingest
    // Do not override entrypoint; use image defaults by passing nil
    return n.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, ports, "", n.Bind(), nil, n.HostName(), nil, nil, nil)
}

// IngestGRPCEndpoint returns the in-network OTLP/gRPC endpoint (host:port)
func (n *Node) IngestGRPCEndpoint() string { return fmt.Sprintf("%s:%s", n.Name(), defaultPorts().OTLPGRPC) }

// IngestHTTPEndpoint returns the in-network OTLP/HTTP endpoint (http://host:port)
func (n *Node) IngestHTTPEndpoint() string { return fmt.Sprintf("http://%s:%s", n.Name(), defaultPorts().OTLPHTTP) }

// QueryHostURL returns the host-mapped Jaeger query base URL (http://127.0.0.1:PORT)
func (n *Node) QueryHostURL() string { return fmt.Sprintf("http://127.0.0.1:%s", n.external.Metrics) }
