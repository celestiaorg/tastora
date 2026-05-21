package spamoor

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/types"
	"net/netip"

	"github.com/moby/moby/api/types/network"
	"go.uber.org/zap"
)

type nodeType int

func (nodeType) String() string { return "spamoor" }

type Ports struct {
	Web string // web UI + /metrics
}

const defaultWebPort = "8080"

func defaultInternalPorts() Ports { return Ports{Web: defaultWebPort} }

type Config struct {
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string
	Logger          *zap.Logger
	Image           container.Image

	RPCHosts            []string
	PrivateKey          string
	AdditionalStartArgs []string
	HostNetwork         bool
}

type Node struct {
	*container.Node

	cfg      Config
	logger   *zap.Logger
	started  bool
	mu       sync.Mutex
	external types.Ports // HTTP field stores web/metrics host port
	name     string
}

// DefaultHomeDir returns the default home directory for spamoor containers.
func DefaultHomeDir() string {
	return "/home/spamoor"
}

func newNode(ctx context.Context, cfg Config, testName string, index int, name string, homeDir string) (*Node, error) {
	if homeDir == "" {
		homeDir = DefaultHomeDir()
	}
	log := cfg.Logger.With(zap.String("component", "spamoor-daemon"), zap.Int("i", index))
	containerName := spamoorNodeName(testName, index, name)
	node, err := container.NewNodeBuilder(cfg.DockerClient, testName, cfg.Image, log).
		WithNetworkID(cfg.DockerNetworkID).
		WithHomeDir(homeDir).
		WithIndex(index).
		WithNodeType(nodeType(0)).
		WithHostNetwork(cfg.HostNetwork).
		Build(ctx, containerName)
	if err != nil {
		return nil, err
	}
	n := &Node{cfg: cfg, logger: log, name: name}
	n.Node = node
	return n, nil
}

func spamoorNodeName(testName string, index int, name string) string {
	if name != "" {
		return fmt.Sprintf("spamoor-%s-%d-%s", name, index, internal.SanitizeDockerResourceName(testName))
	}
	return fmt.Sprintf("spamoor-%d-%s", index, internal.SanitizeDockerResourceName(testName))
}

func (n *Node) Name() string {
	return spamoorNodeName(n.TestName, n.Index, n.name)
}

func (n *Node) HostName() string { return internal.CondenseHostName(n.Name()) }

func (n *Node) GetNetworkInfo(ctx context.Context) (types.NetworkInfo, error) {
	if n.cfg.HostNetwork {
		p := types.Ports{HTTP: defaultInternalPorts().Web}
		return types.NetworkInfo{
			Internal: types.Network{Hostname: "127.0.0.1", IP: "127.0.0.1", Ports: p},
			External: types.Network{Hostname: "127.0.0.1", Ports: p},
		}, nil
	}
	internalIP, err := internal.GetContainerInternalIP(ctx, n.DockerClient, n.ContainerLifecycle.ContainerID())
	if err != nil {
		return types.NetworkInfo{}, err
	}
	return types.NetworkInfo{
		Internal: types.Network{Hostname: n.HostName(), IP: internalIP, Ports: types.Ports{HTTP: defaultInternalPorts().Web}},
		External: types.Network{Hostname: "0.0.0.0", Ports: n.external},
	}, nil
}

func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.started {
		return n.StartContainer(ctx)
	}
	if err := n.createNodeContainer(ctx); err != nil {
		return err
	}
	if err := n.ContainerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	var mapped string
	if n.cfg.HostNetwork {
		mapped = defaultInternalPorts().Web
	} else {
		hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, defaultInternalPorts().Web+"/tcp")
		if err != nil {
			return err
		}
		mapped = internal.MustExtractPort(hostPorts[0])
	}
	n.external = types.Ports{HTTP: mapped}
	n.started = true
	waitHTTP(fmt.Sprintf("http://127.0.0.1:%s/metrics", n.external.HTTP), 20*time.Second)
	return nil
}

// API returns a client bound to this node's exposed HTTP port.
func (n *Node) API() *API {
	base := fmt.Sprintf("http://127.0.0.1:%s", n.external.HTTP)
	return NewAPI(base)
}

func (n *Node) createNodeContainer(ctx context.Context) error {
	p := defaultInternalPorts()

	dbPath := fmt.Sprintf("%s/%s", n.HomeDir(), "spamoor.db")
	binds := n.Bind()
	cmd := []string{
		"--privkey", n.cfg.PrivateKey,
		"--port", p.Web,
		"--db", dbPath,
	}
	for _, h := range n.cfg.RPCHosts {
		if s := strings.TrimSpace(h); s != "" {
			cmd = append(cmd, "--rpchost", s)
		}
	}
	for _, arg := range n.cfg.AdditionalStartArgs {
		if arg == "--port" || strings.HasPrefix(arg, "--port=") {
			return fmt.Errorf("additional start args must not override --port; it is managed internally")
		}
	}
	cmd = append(cmd, n.cfg.AdditionalStartArgs...)

	port := network.MustParsePort(p.Web + "/tcp")
	ports := network.PortMap{
		port: []network.PortBinding{{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: ""}},
	}

	// IMPORTANT: override entrypoint to the daemon (absolute path inside image)
	return n.CreateContainer(
		ctx,
		n.TestName,
		n.NetworkID,
		n.cfg.Image,
		ports,
		"",
		binds,
		nil,
		n.HostName(),
		cmd,
		nil,
		[]string{"/app/spamoor-daemon"}, // entrypoint override
	)
}

// waitHTTP polls a URL until it succeeds or the timeout elapses.
func waitHTTP(url string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 500 {
			_ = resp.Body.Close()
			return
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
}
