package jaeger

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

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

	cfg           Config
	logger        *zap.Logger
	started       bool
	mu            sync.Mutex
	external      types.Ports // RPC->4317, HTTP->4318
	queryHostPort string      // host-mapped port for Jaeger query/UI (16686)
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
func (n *Node) HostName() string {
	return internal.CondenseHostName(n.Name())
}

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
	n.external = types.Ports{RPC: internal.MustExtractPort(hostPorts[0]), HTTP: internal.MustExtractPort(hostPorts[1])}
	n.queryHostPort = internal.MustExtractPort(hostPorts[2])
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
	// Enable OTLP receivers, keep default entrypoint/cmd
	env := []string{"COLLECTOR_OTLP_ENABLED=true"}
	return n.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, ports, "", n.Bind(), nil, n.HostName(), nil, env, nil)
}

// IngestGRPCEndpoint returns the in-network OTLP/gRPC endpoint (host:port)
func (n *Node) IngestGRPCEndpoint() string {
	return fmt.Sprintf("%s:%s", n.Name(), defaultPorts().OTLPGRPC)
}

// IngestHTTPEndpoint returns the in-network OTLP/HTTP endpoint (http://host:port)
func (n *Node) IngestHTTPEndpoint() string {
	return fmt.Sprintf("http://%s:%s", n.Name(), defaultPorts().OTLPHTTP)
}

// QueryHostURL returns the host-mapped Jaeger query base URL (http://127.0.0.1:PORT)
func (n *Node) QueryHostURL() string {
	return fmt.Sprintf("http://127.0.0.1:%s", n.queryHostPort)
}

// Services queries Jaeger's /api/services and returns the list of service names.
func (n *Node) Services(ctx context.Context) ([]string, error) {
	u := n.QueryHostURL() + "/api/services"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jaeger services http %d", resp.StatusCode)
	}
	var out struct {
		Data []string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// HasService polls /api/services until the given service appears or context is done.
func (n *Node) HasService(ctx context.Context, service string, interval time.Duration) (bool, error) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-t.C:
			svcs, err := n.Services(ctx)
			if err == nil {
				for _, s := range svcs {
					if s == service {
						return true, nil
					}
				}
			}
		}
	}
}

// Traces queries Jaeger for traces for a given service, returning the raw data array.
func (n *Node) Traces(ctx context.Context, service string, limit int) ([]any, error) {
	if limit <= 0 {
		limit = 5
	}
	u := fmt.Sprintf("%s/api/traces?service=%s&limit=%d", n.QueryHostURL(), service, limit)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jaeger traces http %d", resp.StatusCode)
	}
	var out struct {
		Data []any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// WaitForTraces polls Jaeger for traces for the service until at least one trace is present or context is done.
func (n *Node) WaitForTraces(ctx context.Context, service string, min int, interval time.Duration) (bool, error) {
	if min <= 0 {
		min = 1
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-t.C:
			data, err := n.Traces(ctx, service, min)
			if err == nil && len(data) >= min {
				return true, nil
			}
		}
	}
}
