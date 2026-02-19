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
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

type nodeType int

func (nodeType) String() string { return "spamoor" }

type Ports struct {
	Web string // web UI + /metrics
}

func defaultInternalPorts() Ports { return Ports{Web: "8080"} }

type Config struct {
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string
	Logger          *zap.Logger
	Image           container.Image

	RPCHosts   []string
	PrivateKey string
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

func newNode(ctx context.Context, cfg Config, testName string, index int, name string) (*Node, error) {
	log := cfg.Logger.With(zap.String("component", "spamoor-daemon"), zap.Int("i", index))
	n := &Node{cfg: cfg, logger: log, name: name}
	n.Node = container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, cfg.Image, "/home/spamoor", index, nodeType(0), log)
	n.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, n.Name()))
	if err := n.CreateAndSetupVolume(ctx, n.Name()); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *Node) Name() string {
	if n.name != "" {
		return fmt.Sprintf("spamoor-%s-%d-%s", n.name, n.Index, internal.SanitizeDockerResourceName(n.TestName))
	}
	return fmt.Sprintf("spamoor-%d-%s", n.Index, internal.SanitizeDockerResourceName(n.TestName))
}

func (n *Node) HostName() string { return internal.CondenseHostName(n.Name()) }

func (n *Node) GetNetworkInfo(ctx context.Context) (types.NetworkInfo, error) {
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
	hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, defaultInternalPorts().Web+"/tcp")
	if err != nil {
		return err
	}
    mapped := internal.MustExtractPort(hostPorts[0])
    n.external = types.Ports{HTTP: mapped}
	n.started = true
	// readiness wait for /metrics endpoint (best-effort)
    waitHTTP(fmt.Sprintf("http://127.0.0.1:%s/metrics", n.external.HTTP), 20*time.Second)
    return nil
}

func (n *Node) createNodeContainer(ctx context.Context) error {
	p := defaultInternalPorts()

	// Daemon flags only; entrypoint will be spamoor-daemon
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

	port := nat.Port(p.Web + "/tcp")
	ports := nat.PortMap{
		port: []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: ""}},
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
