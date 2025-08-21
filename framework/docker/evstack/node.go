package evstack

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/types"
	libclient "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var _ types.EVStackNode = &Node{}

const (
	p2pPort         = "26656/tcp"
	grpcPort        = "9090/tcp" 
	apiPort         = "1317/tcp"
	privValPort     = "1234/tcp"
	evstackRpcPort  = "7331/tcp"
	evstackHttpPort = "8080/tcp"
)

var evstackSentryPorts = nat.PortMap{
	nat.Port(p2pPort):         {},
	nat.Port(evstackRpcPort):  {}, // evstack uses a different rpc port
	nat.Port(grpcPort):        {},
	nat.Port(apiPort):         {},
	nat.Port(privValPort):     {},
	nat.Port(evstackHttpPort): {},
}

type Node struct {
	*container.Node
	cfg Config
	mu  sync.Mutex

	GrpcConn *grpc.ClientConn

	// isAggregatorFlag determines if this node should act as an aggregator
	isAggregatorFlag bool

	// Ports set during startContainer.
	hostRPCPort  string
	hostAPIPort  string
	hostGRPCPort string
	hostP2PPort  string
	hostHTTPPort string
}

func NewNode(cfg Config, testName string, image container.Image, index int, isAggregator bool) *Node {
	logger := cfg.Logger.With(
		zap.Int("i", index),
		zap.Bool("aggregator", isAggregator),
	)
	node := &Node{
		cfg:              cfg,
		isAggregatorFlag: isAggregator,
		Node:             container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, image, path.Join("/var", "evstack"), index, EvstackType, logger),
	}

	node.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, node.Name()))
	return node
}

// Name of the test node container.
func (n *Node) Name() string {
	return fmt.Sprintf("%s-evstack-%d-%s", n.cfg.ChainConfig.ChainID, n.Index, internal.SanitizeContainerName(n.TestName))
}

// HostName returns the condensed hostname for the Node.
func (n *Node) HostName() string {
	return internal.CondenseHostName(n.Name())
}

func (n *Node) logger() *zap.Logger {
	return n.cfg.Logger.With(
		zap.String("chain_id", n.cfg.ChainConfig.ChainID),
		zap.String("test", n.TestName),
	)
}

// isAggregator returns true if the Node is the aggregator
func (n *Node) isAggregator() bool {
	return n.isAggregatorFlag
}

// Init initializes the Node.
func (n *Node) Init(ctx context.Context, initArguments ...string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	cmd := []string{n.cfg.ChainConfig.Bin, "--home", n.HomeDir(), "--chain_id", n.cfg.ChainConfig.ChainID, "init"}
	if n.isAggregator() {
		signerPath := filepath.Join(n.HomeDir(), "config")
		cmd = append(cmd,
			"--rollkit.node.aggregator",
			"--rollkit.signer.passphrase="+n.cfg.ChainConfig.AggregatorPassphrase, //nolint:gosec // used for testing only
			"--rollkit.signer.path="+signerPath)
	}

	cmd = append(cmd, initArguments...)

	_, _, err := n.Exec(ctx, n.Logger, cmd, n.cfg.ChainConfig.Env)
	if err != nil {
		return fmt.Errorf("failed to initialize evstack node: %w", err)
	}

	return nil
}

// Start starts an individual Node.
func (n *Node) Start(ctx context.Context, startArguments ...string) error {
	if err := n.createEvstackContainer(ctx, startArguments...); err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := n.startContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

// createEvstackContainer initializes but does not start a container for the Node with the specified configuration and context.
func (n *Node) createEvstackContainer(ctx context.Context, additionalStartArgs ...string) error {

	usingPorts := nat.PortMap{}
	for k, v := range evstackSentryPorts {
		usingPorts[k] = v
	}

	startCmd := []string{
		n.cfg.ChainConfig.Bin,
		"--home", n.HomeDir(),
		"start",
	}
	if n.isAggregator() {
		signerPath := filepath.Join(n.HomeDir(), "config")
		startCmd = append(startCmd,
			"--rollkit.node.aggregator",
			"--rollkit.signer.passphrase="+n.cfg.ChainConfig.AggregatorPassphrase, //nolint:gosec // used for testing only
			"--rollkit.signer.path="+signerPath)
	}

	// any custom arguments passed in on top of the required ones.
	startCmd = append(startCmd, additionalStartArgs...)

	return n.ContainerLifecycle.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, usingPorts, "", n.Bind(), nil, n.HostName(), startCmd, n.cfg.ChainConfig.Env, []string{})
}

// startContainer starts the container for the Node, initializes its ports, and ensures the node rpc is responding returning.
// Returns an error if the container fails to start, ports cannot be set, or syncing is not completed within the timeout.
func (n *Node) startContainer(ctx context.Context) error {
	if err := n.ContainerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	// Set the host ports once since they will not change after the container has started.
	hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, evstackRpcPort, grpcPort, apiPort, p2pPort, evstackHttpPort)
	if err != nil {
		return err
	}
	n.hostRPCPort, n.hostGRPCPort, n.hostAPIPort, n.hostP2PPort, n.hostHTTPPort = hostPorts[0], hostPorts[1], hostPorts[2], hostPorts[3], hostPorts[4]

	err = n.initGRPCConnection("tcp://" + n.hostRPCPort)
	if err != nil {
		return err
	}

	// wait a short period of time for the node to come online.
	time.Sleep(5 * time.Second)

	return n.waitForNodeReady(ctx, 60*time.Second)
}

// initGRPCConnection creates and assigns a new GRPC connection to the Node.
func (n *Node) initGRPCConnection(addr string) error {
	httpClient, err := libclient.DefaultHTTPClient(addr)
	if err != nil {
		return err
	}

	httpClient.Timeout = 10 * time.Second
	grpcConn, err := grpc.NewClient(
		n.hostGRPCPort, grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("grpc dial: %w", err)
	}
	n.GrpcConn = grpcConn

	return nil
}

// GetHostName returns the hostname of the Node
func (n *Node) GetHostName() string {
	return n.HostName()
}

// GetHostRPCPort returns the host RPC port
func (n *Node) GetHostRPCPort() string {
	return strings.ReplaceAll(n.hostRPCPort, "0.0.0.0:", "")
}

// GetHostAPIPort returns the host API port
func (n *Node) GetHostAPIPort() string {
	return strings.ReplaceAll(n.hostAPIPort, "0.0.0.0:", "")
}

// GetHostGRPCPort returns the host GRPC port
func (n *Node) GetHostGRPCPort() string {
	return strings.ReplaceAll(n.hostGRPCPort, "0.0.0.0:", "")
}

// GetHostP2PPort returns the host P2P port
func (n *Node) GetHostP2PPort() string {
	return strings.ReplaceAll(n.hostP2PPort, "0.0.0.0:", "")
}

// GetHostHTTPPort returns the host HTTP port
func (n *Node) GetHostHTTPPort() string {
	return strings.ReplaceAll(n.hostHTTPPort, "0.0.0.0:", "")
}

// waitForNodeReady polls the health endpoint until the node is ready or timeout is reached
func (n *Node) waitForNodeReady(ctx context.Context, timeout time.Duration) error {
	healthURL := fmt.Sprintf("http://%s/evnode.v1.HealthService/Livez", n.hostRPCPort)
	client := &http.Client{Timeout: 5 * time.Second}

	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for node readiness: %w", ctx.Err())
		case <-timeoutCh:
			return fmt.Errorf("node did not become ready within timeout")
		case <-ticker.C:
			if n.isNodeHealthy(client, healthURL) {
				n.logger().Info("evstack node is ready")
				return nil
			}
		}
	}
}

// isNodeHealthy checks if the node health endpoint returns 200
func (n *Node) isNodeHealthy(client *http.Client, healthURL string) bool {
	req, err := http.NewRequest("POST", healthURL, bytes.NewBufferString("{}"))
	if err != nil {
		n.logger().Debug("failed to create health check request", zap.Error(err))
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		n.logger().Debug("evstack node not ready yet", zap.String("url", healthURL), zap.Error(err))
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 200 {
		return true
	}

	n.logger().Debug("evstack node not ready yet", zap.String("url", healthURL), zap.Int("status", resp.StatusCode))
	return false
}