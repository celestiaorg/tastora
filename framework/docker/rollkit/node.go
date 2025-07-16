package rollkit

import (
	"bytes"
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/docker/go-connections/nat"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	rpcPort  = "7331/tcp"
	httpPort = "8080/tcp"
)

var sentryPorts = nat.PortMap{
	nat.Port("2121/tcp"):  {}, // p2p port
	nat.Port(rpcPort):     {},
	nat.Port("9090/tcp"):  {}, // grpc port
	nat.Port("1317/tcp"):  {}, // api port
	nat.Port("26658/tcp"): {}, // priv val port
	nat.Port(httpPort):    {},
}

type Node struct {
	*container.Node
	dockerClient         *client.Client
	dockerNetworkID      string
	log                  *zap.Logger
	mu                   sync.Mutex
	chainID              string
	binaryName           string
	aggregatorPassphrase string

	GrpcConn *grpc.ClientConn

	// Ports set during startContainer.
	hostRPCPort  string
	hostAPIPort  string
	hostGRPCPort string
	hostP2PPort  string
	hostHTTPPort string
}

func NewNode(dockerClient *client.Client, dockerNetworkID string, logger *zap.Logger, testName string, image container.Image, index int, chainID, binaryName, aggregatorPassphrase string) *Node {
	nodeLogger := logger.With(
		zap.Int("i", index),
		zap.Bool("aggregator", index == 0),
	)
	
	n := &Node{
		dockerClient:         dockerClient,
		dockerNetworkID:      dockerNetworkID,
		log:                  nodeLogger,
		chainID:              chainID,
		binaryName:           binaryName,
		aggregatorPassphrase: aggregatorPassphrase,
		Node:                 container.NewNode(dockerNetworkID, dockerClient, testName, image, path.Join("/var", "rollkit"), index, "rollkit", nodeLogger),
	}

	n.SetContainerLifecycle(container.NewLifecycle(logger, dockerClient, n.Name()))
	return n
}

// Name of the test node container.
func (n *Node) Name() string {
	return fmt.Sprintf("rollkit-%d-%s", n.Index, internal.SanitizeContainerName(n.TestName))
}

// HostName returns the condensed hostname for the Node.
func (n *Node) HostName() string {
	return internal.CondenseHostName(n.Name())
}

// isAggregator returns true if the Node is the aggregator
func (n *Node) isAggregator() bool {
	return n.Index == 0
}

// Init initializes the Node.
func (n *Node) Init(ctx context.Context, initArguments ...string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	cmd := []string{n.binaryName, "--home", n.HomeDir(), "--chain_id", n.chainID, "init"}
	if n.isAggregator() {
		signerPath := filepath.Join(n.HomeDir(), "config")
		cmd = append(cmd,
			"--rollkit.node.aggregator",
			"--rollkit.signer.passphrase="+n.aggregatorPassphrase,
			"--rollkit.signer.path="+signerPath)
	}

	cmd = append(cmd, initArguments...)

	_, _, err := n.Exec(ctx, n.log, cmd, []string{})
	if err != nil {
		return fmt.Errorf("failed to initialize rollkit node: %w", err)
	}

	return nil
}

// Start starts an individual Node.
func (n *Node) Start(ctx context.Context, startArguments ...string) error {
	if err := n.createContainer(ctx, startArguments...); err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := n.startContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

// createContainer initializes but does not start a container for the Node with the specified configuration and context.
func (n *Node) createContainer(ctx context.Context, additionalStartArgs ...string) error {
	usingPorts := nat.PortMap{}
	for k, v := range sentryPorts {
		usingPorts[k] = v
	}

	startCmd := []string{
		n.binaryName,
		"--home", n.HomeDir(),
		"--chain_id", n.chainID,
		"start",
	}
	if n.isAggregator() {
		signerPath := filepath.Join(n.HomeDir(), "config")
		startCmd = append(startCmd,
			"--rollkit.node.aggregator",
			"--rollkit.signer.passphrase="+n.aggregatorPassphrase,
			"--rollkit.signer.path="+signerPath)
	}

	startCmd = append(startCmd, additionalStartArgs...)

	containerLifecycle := container.NewLifecycle(n.log, n.dockerClient, n.Name())
	n.SetContainerLifecycle(containerLifecycle)

	return containerLifecycle.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, usingPorts, "", n.bind(), nil, n.HostName(), startCmd, []string{}, []string{})
}

// bind returns the home folder bind point for running the Node.
func (n *Node) bind() []string {
	return []string{fmt.Sprintf("%s:%s", n.VolumeName, n.HomeDir())}
}

// startContainer starts the container for the Node, initializes its ports, and ensures the node rpc is responding.
func (n *Node) startContainer(ctx context.Context) error {
	containerLifecycle := container.NewLifecycle(n.log, n.dockerClient, n.Name())
	n.SetContainerLifecycle(containerLifecycle)

	if err := containerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	hostPorts, err := containerLifecycle.GetHostPorts(ctx, rpcPort, "9090/tcp", "1317/tcp", "2121/tcp", httpPort)
	if err != nil {
		return err
	}
	n.hostRPCPort, n.hostGRPCPort, n.hostAPIPort, n.hostP2PPort, n.hostHTTPPort = hostPorts[0], hostPorts[1], hostPorts[2], hostPorts[3], hostPorts[4]

	err = n.initGRPCConnection("tcp://" + n.hostRPCPort)
	if err != nil {
		return err
	}

	time.Sleep(5 * time.Second)

	return n.waitForNodeReady(ctx, 60*time.Second)
}

// initGRPCConnection creates and assigns a new GRPC connection to the Node.
func (n *Node) initGRPCConnection(addr string) error {
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
	healthURL := fmt.Sprintf("http://%s/rollkit.v1.HealthService/Livez", n.hostRPCPort)
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
				n.log.Info("rollkit node is ready")
				return nil
			}
		}
	}
}

// isNodeHealthy checks if the node health endpoint returns 200
func (n *Node) isNodeHealthy(client *http.Client, healthURL string) bool {
	req, err := http.NewRequest("POST", healthURL, bytes.NewBufferString("{}"))
	if err != nil {
		n.log.Debug("failed to create health check request", zap.Error(err))
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		n.log.Debug("rollkit node not ready yet", zap.String("url", healthURL), zap.Error(err))
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 200 {
		return true
	}

	n.log.Debug("rollkit node not ready yet", zap.String("url", healthURL), zap.Int("status", resp.StatusCode))
	return false
}