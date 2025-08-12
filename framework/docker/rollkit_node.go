package docker

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

var _ types.RollkitNode = &RollkitNode{}

const (
	rollkitRpcPort  = "7331/tcp"
	rollkitHttpPort = "8080/tcp"
)

var rollkitSentryPorts = nat.PortMap{
	nat.Port(p2pPort):         {},
	nat.Port(rollkitRpcPort):  {}, // rollkit uses a different rpc port
	nat.Port(grpcPort):        {},
	nat.Port(apiPort):         {},
	nat.Port(privValPort):     {},
	nat.Port(rollkitHttpPort): {},
}

type RollkitNode struct {
	*container.Node
	cfg Config
	mu  sync.Mutex

	GrpcConn *grpc.ClientConn

	// Ports set during startContainer.
	hostRPCPort  string
	hostAPIPort  string
	hostGRPCPort string
	hostP2PPort  string
	hostHTTPPort string
}

func NewRollkitNode(cfg Config, testName string, image container.Image, index int) *RollkitNode {
	logger := cfg.Logger.With(
		zap.Int("i", index),
		zap.Bool("aggregator", index == 0),
	)
	rn := &RollkitNode{
		cfg:  cfg,
		Node: container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, image, path.Join("/var", "rollkit"), index, "rollkit", logger),
	}

	rn.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, rn.Name()))
	return rn
}

// Name of the test node container.
func (rn *RollkitNode) Name() string {
	return fmt.Sprintf("%s-rollkit-%d-%s", rn.cfg.RollkitChainConfig.ChainID, rn.Index, internal.SanitizeContainerName(rn.TestName))
}

// HostName returns the condensed hostname for the RollkitNode.
func (rn *RollkitNode) HostName() string {
	return internal.CondenseHostName(rn.Name())
}

func (rn *RollkitNode) logger() *zap.Logger {
	return rn.cfg.Logger.With(
		zap.String("chain_id", rn.cfg.RollkitChainConfig.ChainID),
		zap.String("test", rn.TestName),
	)
}

// isAggregator returns true if the RollkitNode is the aggregator
func (rn *RollkitNode) isAggregator() bool {
	return rn.Index == 0
}

// Init initializes the RollkitNode.
func (rn *RollkitNode) Init(ctx context.Context, initArguments ...string) error {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	cmd := []string{rn.cfg.RollkitChainConfig.Bin, "--home", rn.HomeDir(), "--chain_id", rn.cfg.RollkitChainConfig.ChainID, "init"}
	if rn.isAggregator() {
		signerPath := filepath.Join(rn.HomeDir(), "config")
		cmd = append(cmd,
			"--rollkit.node.aggregator",
			"--rollkit.signer.passphrase="+rn.cfg.RollkitChainConfig.AggregatorPassphrase, //nolint:gosec // used for testing only
			"--rollkit.signer.path="+signerPath)
	}

	cmd = append(cmd, initArguments...)

	_, _, err := rn.Exec(ctx, rn.Logger, cmd, rn.cfg.RollkitChainConfig.Env)
	if err != nil {
		return fmt.Errorf("failed to initialize rollkit node: %w", err)
	}

	return nil
}

// Start starts an individual RollkitNode.
func (rn *RollkitNode) Start(ctx context.Context, startArguments ...string) error {
	if err := rn.createRollkitContainer(ctx, startArguments...); err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := rn.startContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

// createRollkitContainer initializes but does not start a container for the RollkitNode with the specified configuration and context.
func (rn *RollkitNode) createRollkitContainer(ctx context.Context, additionalStartArgs ...string) error {

	usingPorts := nat.PortMap{}
	for k, v := range rollkitSentryPorts {
		usingPorts[k] = v
	}

	startCmd := []string{
		rn.cfg.RollkitChainConfig.Bin,
		"--home", rn.HomeDir(),
		"start",
	}
	if rn.isAggregator() {
		signerPath := filepath.Join(rn.HomeDir(), "config")
		startCmd = append(startCmd,
			"--rollkit.node.aggregator",
			"--rollkit.signer.passphrase="+rn.cfg.RollkitChainConfig.AggregatorPassphrase, //nolint:gosec // used for testing only
			"--rollkit.signer.path="+signerPath)
	}

	// any custom arguments passed in on top of the required ones.
	startCmd = append(startCmd, additionalStartArgs...)

	return rn.ContainerLifecycle.CreateContainer(ctx, rn.TestName, rn.NetworkID, rn.Image, usingPorts, "", rn.Bind(), nil, rn.HostName(), startCmd, rn.cfg.RollkitChainConfig.Env, []string{})
}

// startContainer starts the container for the RollkitNode, initializes its ports, and ensures the node rpc is responding returning.
// Returns an error if the container fails to start, ports cannot be set, or syncing is not completed within the timeout.
func (rn *RollkitNode) startContainer(ctx context.Context) error {
	if err := rn.ContainerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	// Set the host ports once since they will not change after the container has started.
	hostPorts, err := rn.ContainerLifecycle.GetHostPorts(ctx, rollkitRpcPort, grpcPort, apiPort, p2pPort, rollkitHttpPort)
	if err != nil {
		return err
	}
	rn.hostRPCPort, rn.hostGRPCPort, rn.hostAPIPort, rn.hostP2PPort, rn.hostHTTPPort = hostPorts[0], hostPorts[1], hostPorts[2], hostPorts[3], hostPorts[4]

	err = rn.initGRPCConnection("tcp://" + rn.hostRPCPort)
	if err != nil {
		return err
	}

	// wait a short period of time for the node to come online.
	time.Sleep(5 * time.Second)

	return rn.waitForNodeReady(ctx, 60*time.Second)
}

// initGRPCConnection creates and assigns a new GRPC connection to the RollkitNode.
func (rn *RollkitNode) initGRPCConnection(addr string) error {
	httpClient, err := libclient.DefaultHTTPClient(addr)
	if err != nil {
		return err
	}

	httpClient.Timeout = 10 * time.Second
	grpcConn, err := grpc.NewClient(
		rn.hostGRPCPort, grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("grpc dial: %w", err)
	}
	rn.GrpcConn = grpcConn

	return nil
}

// GetHostName returns the hostname of the RollkitNode
func (rn *RollkitNode) GetHostName() string {
	return rn.HostName()
}

// GetHostRPCPort returns the host RPC port
func (rn *RollkitNode) GetHostRPCPort() string {
	return strings.ReplaceAll(rn.hostRPCPort, "0.0.0.0:", "")
}

// GetHostAPIPort returns the host API port
func (rn *RollkitNode) GetHostAPIPort() string {
	return strings.ReplaceAll(rn.hostAPIPort, "0.0.0.0:", "")
}

// GetHostGRPCPort returns the host GRPC port
func (rn *RollkitNode) GetHostGRPCPort() string {
	return strings.ReplaceAll(rn.hostGRPCPort, "0.0.0.0:", "")
}

// GetHostP2PPort returns the host P2P port
func (rn *RollkitNode) GetHostP2PPort() string {
	return strings.ReplaceAll(rn.hostP2PPort, "0.0.0.0:", "")
}

// GetHostHTTPPort returns the host HTTP port
func (rn *RollkitNode) GetHostHTTPPort() string {
	return strings.ReplaceAll(rn.hostHTTPPort, "0.0.0.0:", "")
}

// waitForNodeReady polls the health endpoint until the node is ready or timeout is reached
func (rn *RollkitNode) waitForNodeReady(ctx context.Context, timeout time.Duration) error {
	healthURL := fmt.Sprintf("http://%s/evnode.v1.HealthService/Livez", rn.hostRPCPort)
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
			if rn.isNodeHealthy(client, healthURL) {
				rn.logger().Info("rollkit node is ready")
				return nil
			}
		}
	}
}

// isNodeHealthy checks if the node health endpoint returns 200
func (rn *RollkitNode) isNodeHealthy(client *http.Client, healthURL string) bool {
	req, err := http.NewRequest("POST", healthURL, bytes.NewBufferString("{}"))
	if err != nil {
		rn.logger().Debug("failed to create health check request", zap.Error(err))
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		rn.logger().Debug("rollkit node not ready yet", zap.String("url", healthURL), zap.Error(err))
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 200 {
		return true
	}

	rn.logger().Debug("rollkit node not ready yet", zap.String("url", healthURL), zap.Int("status", resp.StatusCode))
	return false
}
