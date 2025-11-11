package evmsingle

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

// Node represents an ev-node-evm-single node container and configuration.
type Node struct {
	*container.Node

	cfg     Config
	nodeCfg NodeConfig
	logger  *zap.Logger

	isInitialized bool
	mu            sync.Mutex
	internal      types.Ports
	external      types.Ports
}

func newNode(ctx context.Context, cfg Config, testName string, index int, nodeCfg NodeConfig) (*Node, error) {
	image := cfg.Image
	if nodeCfg.Image.Repository != "" {
		image = nodeCfg.Image
	}

	ports := defaultPorts()

	log := cfg.Logger.With(zap.String("component", "evm-single"), zap.Int("i", index))

	// This image expects the home at /root/.evm-single by convention
	homeDir := "/root/.evm-single"

	n := &Node{cfg: cfg, nodeCfg: nodeCfg, logger: log, internal: ports}
	n.Node = container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, image, homeDir, index, NodeType, log)
	n.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, n.Name()))
	if err := n.CreateAndSetupVolume(ctx, n.Name()); err != nil {
		return nil, err
	}
	return n, nil
}

// Name returns a stable container name
func (n *Node) Name() string {
	return fmt.Sprintf("evm-single-%d-%s", n.Index, internal.SanitizeDockerResourceName(n.TestName))
}

// HostName returns a condensed hostname
func (n *Node) HostName() string { return internal.CondenseHostName(n.Name()) }

// passphraseFilePath returns the path to the passphrase file
func (n *Node) passphraseFilePath() string {
	return filepath.Join(n.HomeDir(), "config", "passphrase.txt")
}

// GetNetworkInfo returns internal/external addressing
func (n *Node) GetNetworkInfo(ctx context.Context) (types.NetworkInfo, error) {
	internalIP, err := internal.GetContainerInternalIP(ctx, n.DockerClient, n.ContainerLifecycle.ContainerID())
	if err != nil {
		return types.NetworkInfo{}, err
	}
	return types.NetworkInfo{
		Internal: types.Network{Hostname: n.HostName(), IP: internalIP, Ports: n.internal},
		External: types.Network{Hostname: "0.0.0.0", Ports: n.external},
	}, nil
}

// Start creates and starts the container
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.isInitialized {
		if err := n.initContainer(ctx); err != nil {
			return fmt.Errorf("init container: %w", err)
		}
		n.isInitialized = true
	}

	if err := n.createNodeContainer(ctx); err != nil {
		return fmt.Errorf("create node container: %w", err)
	}

	if err := n.StartContainer(ctx); err != nil {
		return fmt.Errorf("start container: %w", err)
	}

	hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, n.internal.RPC+"/tcp", n.internal.P2P+"/tcp")
	if err != nil {
		return fmt.Errorf("get host ports: %w", err)
	}

	n.external = types.Ports{
		RPC: internal.MustExtractPort(hostPorts[0]),
		P2P: internal.MustExtractPort(hostPorts[1]),
	}

	// Wait for the node's own RPC to be responsive using its CLI
	if err := n.waitForSelfReady(ctx); err != nil {
		return fmt.Errorf("wait for self ready: %w", err)
	}

	return nil
}

// initContainer runs `evm-single init` inside the container to set up the config directory.
func (n *Node) initContainer(ctx context.Context) error {
	// Always run init to ensure config exists and is up to date
	initCmd := []string{n.cfg.Bin, "init", "--home", n.HomeDir()}
	if n.nodeCfg.EVMSignerPassphrase != "" {
		if err := n.WriteFile(ctx, filepath.Join("config", "passphrase.txt"), []byte(n.nodeCfg.EVMSignerPassphrase)); err != nil {
			return fmt.Errorf("failed to write passphrase file: %w", err)
		}

		initCmd = append(initCmd,
			"--evnode.node.aggregator=true",
			"--evnode.signer.passphrase_file", n.passphraseFilePath(),
		)
	}

	additionalInitArgs := n.cfg.AdditionalInitArgs
	if len(n.nodeCfg.AdditionalInitArgs) > 0 {
		additionalInitArgs = n.nodeCfg.AdditionalInitArgs
	}

	initCmd = append(initCmd, additionalInitArgs...)

	if _, _, err := n.Exec(ctx, n.Logger, initCmd, n.cfg.Env); err != nil {
		return fmt.Errorf("init evm-single: %w", err)
	}
	return nil
}

// createNodeContainer creates the evm-single container with the appropriate start command and ports.
func (n *Node) createNodeContainer(ctx context.Context) error {
	// Build start command using CLI flags (no entrypoint)
	cmd := []string{n.cfg.Bin, "start", "--home", n.HomeDir()}

	// Require engine/eth URLs and JWT
	if n.nodeCfg.EVMEngineURL == "" || n.nodeCfg.EVMETHURL == "" || n.nodeCfg.EVMJWTSecret == "" {
		return fmt.Errorf("missing EVM connection details: engine-url, eth-url, and jwt-secret are required")
	}

	// write the contents of the provided secret.
	if err := n.WriteFile(ctx, "jwt-secret.txt", []byte(n.nodeCfg.EVMJWTSecret)); err != nil {
		return fmt.Errorf("failed to write jwt-secret.txt: %w", err)
	}

	cmd = append(cmd, "--evm.engine-url", n.nodeCfg.EVMEngineURL)
	cmd = append(cmd, "--evm.eth-url", n.nodeCfg.EVMETHURL)
	cmd = append(cmd, "--evm.jwt-secret-file", path.Join(n.HomeDir(), "jwt-secret.txt"))

	if n.nodeCfg.EVMGenesisHash == "" {
		return fmt.Errorf("missing --evm.genesis-hash. must match block 0 hash of execution client")
	}

	cmd = append(cmd, "--evm.genesis-hash", n.nodeCfg.EVMGenesisHash)
	if n.nodeCfg.EVMBlockTime != "" {
		cmd = append(cmd, "--evnode.node.block_time", n.nodeCfg.EVMBlockTime)
	}
	if n.nodeCfg.EVMSignerPassphrase != "" {
		cmd = append(cmd, "--evnode.node.aggregator=true", "--evnode.signer.passphrase_file", n.passphraseFilePath())
	}

	if n.nodeCfg.DAAddress != "" {
		cmd = append(cmd, "--evnode.da.address", n.nodeCfg.DAAddress)
	}

	if n.nodeCfg.DAAuthToken != "" {
		cmd = append(cmd, "--evnode.da.auth_token", n.nodeCfg.DAAuthToken)
	}

	if n.nodeCfg.DANamespace != "" {
		cmd = append(cmd, "--evnode.da.namespace", n.nodeCfg.DANamespace)
	}

	// Ensure RPC listens on all interfaces so other containers/host can reach it
	cmd = append(cmd, "--evnode.rpc.address", fmt.Sprintf("0.0.0.0:%s", n.internal.RPC))

	additionalStartArgs := n.cfg.AdditionalStartArgs
	if len(n.nodeCfg.AdditionalStartArgs) > 0 {
		additionalStartArgs = n.nodeCfg.AdditionalStartArgs
	}

	cmd = append(cmd, additionalStartArgs...)

	usingPorts := nat.PortMap{
		nat.Port(n.internal.RPC + "/tcp"): {},
		nat.Port(n.internal.P2P + "/tcp"): {},
	}

	return n.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, usingPorts, "", n.Bind(), nil, n.HostName(), cmd, n.cfg.Env, []string{})
}

// waitForSelfReady runs `evm-single net-info` inside the container until it succeeds,
// indicating the internal RPC (127.0.0.1:7331) is serving.
func (n *Node) waitForSelfReady(ctx context.Context) error {
	deadline := time.Now().Add(120 * time.Second)
	httpURL := fmt.Sprintf("http://0.0.0.0:%s/evnode.v1.HealthService/Livez", n.external.RPC)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("evm-single health not ready within timeout at %s", httpURL)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled: %w", ctx.Err())
		default:
		}

		if c, err := net.DialTimeout("tcp", fmt.Sprintf("0.0.0.0:%s", n.external.RPC), 2*time.Second); err == nil {
			_ = c.Close()
		} else {
			time.Sleep(1 * time.Second)
			continue
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, httpURL, bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}
}
