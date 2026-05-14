package victoriatraces

import (
	"context"
	"fmt"
	"sync"

	"net/netip"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/moby/moby/api/types/network"
	"go.uber.org/zap"
)

type nodeType int

func (nodeType) String() string { return "victoriatraces" }

// default port for VictoriaTraces — serves both OTLP HTTP ingest and query API.
const defaultHTTPPort = "10428"

type Config struct {
	Logger          *zap.Logger
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string
	Image           container.Image
	HomeDir         string
}

type Node struct {
	*container.Node

	cfg     Config
	logger  *zap.Logger
	started bool
	mu      sync.Mutex

	internalHTTPPort string
	externalHTTPPort string

	Internal scope
	External scope
}

func New(ctx context.Context, cfg Config, testName string, index int) (*Node, error) {
	img := cfg.Image
	if img.Repository == "" {
		img = container.NewImage("victoriametrics/victoria-traces", "latest", "")
	}
	log := cfg.Logger.With(zap.String("component", "victoriatraces"), zap.Int("i", index))
	homeDir := cfg.HomeDir
	if homeDir == "" {
		homeDir = DefaultHomeDir()
	}
	n := &Node{cfg: cfg, logger: log, internalHTTPPort: defaultHTTPPort}
	n.Node = container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, img, homeDir, index, nodeType(0), log)
	name := n.Name()
	n.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, name))
	if err := n.CreateAndSetupVolume(ctx, name); err != nil {
		return nil, err
	}
	n.Internal = scope{hostname: n.HostName(), port: &n.internalHTTPPort}
	n.External = scope{hostname: "0.0.0.0", port: &n.externalHTTPPort}
	return n, nil
}

func (n *Node) Name() string {
	return fmt.Sprintf("victoriatraces-%d-%s", n.Index, internal.SanitizeDockerResourceName(n.TestName))
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
	hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, n.internalHTTPPort+"/tcp")
	if err != nil {
		return err
	}
	n.externalHTTPPort = internal.MustExtractPort(hostPorts[0])
	n.started = true
	return nil
}

// DefaultHomeDir returns the default home directory for victoriatraces containers.
func DefaultHomeDir() string {
	return "/home/victoriatraces"
}

func (n *Node) createContainer(ctx context.Context) error {
	port := network.MustParsePort(n.internalHTTPPort + "/tcp")
	ports := network.PortMap{
		port: []network.PortBinding{{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: ""}},
	}
	cmd := []string{"-storageDataPath", n.HomeDir() + "/data"}
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
			Ports:    types.Ports{HTTP: n.internalHTTPPort},
		},
		External: types.Network{
			Hostname: "0.0.0.0",
			Ports:    types.Ports{HTTP: n.externalHTTPPort},
		},
	}, nil
}

// scope provides scoped (internal/external) access to VictoriaTraces endpoints.
type scope struct {
	hostname string
	port     *string
}

// IngestHTTPEndpoint returns the full OTLP HTTP ingest URL including the /v1/traces path.
// Use this for exporters (like Go's otlptracehttp with WithEndpointURL) that send to the
// URL as-is without appending any path.
func (s scope) IngestHTTPEndpoint() string {
	return fmt.Sprintf("http://%s:%s/insert/opentelemetry/v1/traces", s.hostname, *s.port)
}

// OTLPBaseEndpoint returns the OTLP base URL without the /v1/traces suffix.
// Use this for exporters (like Rust's opentelemetry-otlp) that auto-append /v1/traces.
func (s scope) OTLPBaseEndpoint() string {
	return fmt.Sprintf("http://%s:%s/insert/opentelemetry", s.hostname, *s.port)
}

func (s scope) QueryURL() string {
	return fmt.Sprintf("http://%s:%s", s.hostname, *s.port)
}
