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

// Default ports for Jaeger all-in-one (without /tcp suffix)
const (
	defaultOTLPGRPCPort = "4317"
	defaultOTLPHTTPPort = "4318"
	defaultQueryPort    = "16686"
)

type Config struct {
	Logger          *zap.Logger
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string
	Image           container.Image
}

// Node represents a Jaeger all-in-one container
type Node struct {
	*container.Node

	cfg     Config
	logger  *zap.Logger
	started bool
	mu      sync.Mutex

	internalPorts types.Ports
	externalPorts types.Ports

	Internal queryScope
	External queryScope
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
	n.Internal = queryScope{n: n, hostname: func() string { return n.Name() }, ports: &n.internalPorts}
	n.External = queryScope{n: n, hostname: func() string { return "0.0.0.0" }, ports: &n.externalPorts}
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
	hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, n.internalPorts.GRPC+"/tcp", n.internalPorts.HTTP+"/tcp", n.internalPorts.API+"/tcp")
	if err != nil {
		return err
	}
	n.externalPorts = types.Ports{
		GRPC: internal.MustExtractPort(hostPorts[0]),
		HTTP: internal.MustExtractPort(hostPorts[1]),
		API:  internal.MustExtractPort(hostPorts[2]),
	}
	n.started = true
	return nil
}

func (n *Node) createContainer(ctx context.Context) error {
	// Initialize internal ports with defaults if not set
	if n.internalPorts.GRPC == "" {
		n.internalPorts.GRPC = defaultOTLPGRPCPort
	}
	if n.internalPorts.HTTP == "" {
		n.internalPorts.HTTP = defaultOTLPHTTPPort
	}
	if n.internalPorts.API == "" {
		n.internalPorts.API = defaultQueryPort
	}

	ports := nat.PortMap{
		nat.Port(n.internalPorts.GRPC + "/tcp"): {},
		nat.Port(n.internalPorts.HTTP + "/tcp"): {},
		nat.Port(n.internalPorts.API + "/tcp"):  {},
	}
	// Enable OTLP receivers, keep default entrypoint/cmd
	env := []string{"COLLECTOR_OTLP_ENABLED=true"}
	return n.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, ports, "", n.Bind(), nil, n.HostName(), nil, env, nil)
}

// GetNetworkInfo returns internal/external network information in the common format
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

// queryScope provides scoped (internal/external) access to Jaeger endpoints.
// It holds a pointer to the parent Node's ports so values are always current.
type queryScope struct {
	n        *Node
	hostname func() string
	ports    *types.Ports
}

func (s queryScope) QueryURL() string {
	return fmt.Sprintf("http://%s:%s", s.hostname(), s.ports.API)
}

func (s queryScope) IngestGRPCEndpoint() string {
	return fmt.Sprintf("%s:%s", s.hostname(), s.ports.GRPC)
}

func (s queryScope) IngestHTTPEndpoint() string {
	return fmt.Sprintf("http://%s:%s", s.hostname(), s.ports.HTTP)
}

// HasService polls /api/services until the given service appears or context is done.
func (s queryScope) HasService(ctx context.Context, service string, interval time.Duration) (bool, error) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-t.C:
			svcs, err := s.Services(ctx)
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

// Services queries Jaeger's /api/services for the scope.
func (s queryScope) Services(ctx context.Context) ([]string, error) {
	u := s.QueryURL() + "/api/services"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
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

// Traces queries Jaeger for traces for the scope.
func (s queryScope) Traces(ctx context.Context, service string, limit int) ([]any, error) {
	if limit <= 0 {
		limit = 5
	}
	u := fmt.Sprintf("%s/api/traces?service=%s&limit=%d", s.QueryURL(), service, limit)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
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

// WaitForTraces polls Jaeger for traces for the scope until at least min traces are present.
func (s queryScope) WaitForTraces(ctx context.Context, service string, min int, interval time.Duration) (bool, error) {
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
			data, err := s.Traces(ctx, service, min)
			if err == nil && len(data) >= min {
				return true, nil
			}
		}
	}
}
