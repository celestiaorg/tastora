package docker

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/file"
	"github.com/celestiaorg/tastora/framework/types"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	libclient "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"path"
	"path/filepath"
	"sync"
	"time"
)

var _ types.RollkitNode = &RollkitNode{}

type RollkitNode struct {
	*node
	cfg Config
	log *zap.Logger
	mu  sync.Mutex

	Client   rpcclient.Client
	GrpcConn *grpc.ClientConn

	CelestiaAddress string

	// Ports set during startContainer.
	hostRPCPort  string
	hostAPIPort  string
	hostGRPCPort string
	hostP2PPort  string
}

func NewRollkitNode(cfg Config, testName string, image DockerImage, index int) *RollkitNode {
	rn := &RollkitNode{
		log: cfg.Logger.With(
			zap.Int("i", index),
			zap.Bool("aggregator", index == 0),
		),
		cfg:  cfg,
		node: newNode(cfg.DockerNetworkID, cfg.DockerClient, testName, image, path.Join("/var", "rollkit"), index, "rollkit"),
	}

	rn.containerLifecycle = NewContainerLifecycle(cfg.Logger, cfg.DockerClient, rn.Name())
	return rn
}

// Name of the test node container.
func (rn *RollkitNode) Name() string {
	return fmt.Sprintf("%s-rollkit-%d-%s", rn.cfg.RollkitChainConfig.ChainID, rn.Index, SanitizeContainerName(rn.TestName))
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

// Init initializes the RollkitNode
func (rn *RollkitNode) Init(ctx context.Context, initArguments ...string) error {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	cmd := []string{rn.cfg.RollkitChainConfig.Bin, "--home", rn.homeDir, "--chain_id", rn.cfg.RollkitChainConfig.ChainID, "init"}
	if rn.isAggregator() {
		signerPath := filepath.Join(rn.homeDir, "config")
		cmd = append(cmd, "--rollkit.node.aggregator", "--rollkit.signer.passphrase="+rn.cfg.RollkitChainConfig.AggregatorPassphrase, "--rollkit.signer.path="+signerPath)
	}

	cmd = append(cmd, initArguments...)

	stdout, stderr, err := rn.exec(ctx, rn.logger(), cmd, rn.cfg.RollkitChainConfig.Env)
	rn.logger().Info("rollkit init command output",
		zap.String("command", fmt.Sprintf("%v", cmd)),
		zap.String("stdout", string(stdout)),
		zap.String("stderr", string(stderr)),
		zap.Bool("is_aggregator", rn.isAggregator()),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize rollkit node: %w", err)
	}

	// Debug: Find signer.json by searching the entire container filesystem
	if err := rn.findSignerFile(ctx); err != nil {
		rn.logger().Error("failed to find signer file", zap.Error(err))
	}

	if err := rn.initAddress(ctx); err != nil {
		return fmt.Errorf("failed to initialize address: %w", err)
	}
	return nil
}

// keyData matches Rollkit signer.json exactly
type keyData struct {
	PrivKeyEncrypted []byte `json:"priv_key_encrypted"`
	Nonce            []byte `json:"nonce"`
	PubKeyBytes      []byte `json:"pub_key"`
	Salt             []byte `json:"salt,omitempty"`
}

func (rn *RollkitNode) readFile(ctx context.Context, relPath string) ([]byte, error) {
	fr := file.NewRetriever(rn.logger(), rn.DockerClient, rn.TestName)
	content, err := fr.SingleFileContent(ctx, rn.VolumeName, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file at %s: %w", relPath, err)
	}
	return content, nil
}

func (rn *RollkitNode) findSignerFile(ctx context.Context) error {
	// Search entire container filesystem for signer.json
	cmd := []string{"find", "/", "-name", "signer.json", "-type", "f", "2>/dev/null"}
	stdout, stderr, err := rn.exec(ctx, rn.logger(), cmd, nil)
	if err != nil {
		rn.logger().Info("find command failed", zap.Error(err), zap.String("stderr", string(stderr)))
	} else {
		rn.logger().Info("signer.json search results", zap.String("found_files", string(stdout)))
	}
	return nil
}

func (rn *RollkitNode) listDirectory(ctx context.Context, dirPath string) error {
	// Use exec to list directory contents
	cmd := []string{"ls", "-la", dirPath}
	stdout, stderr, err := rn.exec(ctx, rn.logger(), cmd, nil)
	if err != nil {
		return fmt.Errorf("failed to list directory %s: %w (stderr: %s)", dirPath, err, stderr)
	}
	rn.logger().Info("directory contents", zap.String("path", dirPath), zap.String("contents", string(stdout)))
	return nil
}

// InitAddress extracts the Cosmos (Celestia) address from signer.json
func (rn *RollkitNode) initAddress(ctx context.Context) error {
	// Debug: List directory contents to find where signer.json is actually created
	rn.logger().Info("Debugging rollkit directory structure")

	// List home directory
	if err := rn.listDirectory(ctx, rn.homeDir); err != nil {
		rn.logger().Error("failed to list home directory", zap.Error(err))
	}

	// List config directory if it exists
	configDir := filepath.Join(rn.homeDir, "config")
	if err := rn.listDirectory(ctx, configDir); err != nil {
		rn.logger().Info("config directory not found or empty", zap.Error(err))
	}

	// Try to find signer.json in various locations
	possiblePaths := []string{
		filepath.Join("config", "signer.json"),
	}

	var content []byte
	var err error
	var usedPath string

	for _, signerFilePath := range possiblePaths {
		content, err = rn.readFile(ctx, signerFilePath)
		if err == nil {
			usedPath = signerFilePath
			rn.logger().Info("found signer.json", zap.String("path", usedPath))
			break
		}
		rn.logger().Info("tried path", zap.String("path", signerFilePath), zap.Error(err))
	}

	if err != nil {
		return fmt.Errorf("failed to read signer.json from any location (tried %v): %w", possiblePaths, err)
	}

	// Unmarshal into keyData struct
	var signer keyData
	if err := json.Unmarshal(content, &signer); err != nil {
		return fmt.Errorf("failed to unmarshal signer.json: %w", err)
	}

	// Debug: Log the signer data
	rn.logger().Info("signer.json contents",
		zap.String("pub_key_hex", hex.EncodeToString(signer.PubKeyBytes)),
		zap.Int("pub_key_len", len(signer.PubKeyBytes)),
		zap.String("raw_content", string(content)),
	)

	// Derive address from PubKeyBytes
	pubKey := ed25519.PubKey{Key: signer.PubKeyBytes}
	addr := sdk.AccAddress(pubKey.Address())

	rn.CelestiaAddress = addr.String()
	rn.logger().Info("derived celestia address", 
		zap.String("address", rn.CelestiaAddress),
		zap.String("pub_key_address_hex", hex.EncodeToString(pubKey.Address())),
	)
	return nil
}

// Start starts an individual rollkit node.
func (rn *RollkitNode) Start(ctx context.Context, startArguments ...string) error {
	if err := rn.createRollkitContainer(ctx, startArguments...); err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := rn.startContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

// createRollkitContainer initializes but does not start a container for the ChainNode with the specified configuration and context.
func (rn *RollkitNode) createRollkitContainer(ctx context.Context, additionalStartArgs ...string) error {

	usingPorts := nat.PortMap{}
	for k, v := range sentryPorts {
		usingPorts[k] = v
	}

	startCmd := []string{
		rn.cfg.RollkitChainConfig.Bin,
		"--home", rn.homeDir,
		"--chain_id", rn.cfg.RollkitChainConfig.ChainID,
		"start",
	}
	if rn.isAggregator() {
		signerPath := filepath.Join(rn.homeDir, "config")
		startCmd = append(startCmd, "--rollkit.node.aggregator", "--rollkit.signer.passphrase="+rn.cfg.RollkitChainConfig.AggregatorPassphrase, "--rollkit.signer.path="+signerPath)
	}

	// any custom arguments passed in on top of the required ones.
	startCmd = append(startCmd, additionalStartArgs...)

	return rn.containerLifecycle.CreateContainer(ctx, rn.TestName, rn.NetworkID, rn.Image, usingPorts, "", rn.bind(), nil, rn.HostName(), startCmd, rn.cfg.RollkitChainConfig.Env, []string{})
}

// startContainer starts the container for the RollkitNode, initializes its ports, and ensures the node rpc is responding returning.
// Returns an error if the container fails to start, ports cannot be set, or syncing is not completed within the timeout.
func (rn *RollkitNode) startContainer(ctx context.Context) error {
	if err := rn.containerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	// Set the host ports once since they will not change after the container has started.
	hostPorts, err := rn.containerLifecycle.GetHostPorts(ctx, rpcPort, grpcPort, apiPort, p2pPort)
	if err != nil {
		return err
	}
	rn.hostRPCPort, rn.hostGRPCPort, rn.hostAPIPort, rn.hostP2PPort = hostPorts[0], hostPorts[1], hostPorts[2], hostPorts[3]

	err = rn.initClient("tcp://" + rn.hostRPCPort)
	if err != nil {
		return err
	}

	// wait a short period of time for the node to come online.
	time.Sleep(5 * time.Second)

	// simple readiness check - just verify RPC is responding
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for node readiness: %w", ctx.Err())
		case <-timeout:
			return fmt.Errorf("node did not become ready within timeout")
		case <-ticker.C:
			_, err := rn.Client.Status(ctx)
			if err == nil {
				// RPC is responding, node is ready
				return nil
			}
		}
	}
}

// initClient creates and assigns a new Tendermint RPC client to the ChainNode.
func (rn *RollkitNode) initClient(addr string) error {
	httpClient, err := libclient.DefaultHTTPClient(addr)
	if err != nil {
		return err
	}

	httpClient.Timeout = 10 * time.Second
	rpcClient, err := rpchttp.NewWithClient(addr, "/websocket", httpClient)
	if err != nil {
		return err
	}

	rn.Client = rpcClient

	grpcConn, err := grpc.NewClient(
		rn.hostGRPCPort, grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("grpc dial: %w", err)
	}
	rn.GrpcConn = grpcConn

	return nil
}
