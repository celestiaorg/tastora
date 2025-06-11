package docker

import (
	"context"
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
		cmd = append(cmd, "--rollkit.node.aggregator", "--rollkit.signer.passphrase="+rn.cfg.RollkitChainConfig.AggregatorPassphrase)
	}

	cmd = append(cmd, initArguments...)

	_, _, err := rn.exec(ctx, rn.logger(), cmd, rn.cfg.RollkitChainConfig.Env)
	if err != nil {
		return fmt.Errorf("failed to initialize rollkit node: %w", err)
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

// InitAddress extracts the Cosmos (Celestia) address from signer.json
func (rn *RollkitNode) initAddress(ctx context.Context) error {
	// signer.json path → <homeDir>/signer/signer.json
	signerFilePath := filepath.Join(rn.homeDir, "signer", "signer.json")

	// Read the file from the Docker volume
	content, err := rn.readFile(ctx, signerFilePath)
	if err != nil {
		return fmt.Errorf("failed to read signer.json: %w", err)
	}

	// Unmarshal into keyData struct
	var signer keyData
	if err := json.Unmarshal(content, &signer); err != nil {
		return fmt.Errorf("failed to unmarshal signer.json: %w", err)
	}

	// Derive address from PubKeyBytes
	pubKey := ed25519.PubKey{Key: signer.PubKeyBytes}
	addr := sdk.AccAddress(pubKey.Address())

	rn.CelestiaAddress = addr.String()
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
		startCmd = append(startCmd, "--rollkit.node.aggregator", "--rollkit.signer.passphrase="+rn.cfg.RollkitChainConfig.AggregatorPassphrase)
	}

	// any custom arguments passed in on top of the required ones.
	startCmd = append(startCmd, additionalStartArgs...)

	return rn.containerLifecycle.CreateContainer(ctx, rn.TestName, rn.NetworkID, rn.Image, usingPorts, "", rn.bind(), nil, rn.HostName(), startCmd, rn.cfg.RollkitChainConfig.Env, []string{})
}

// startContainer starts the container for the RollkitNode, initializes its ports, and ensures the node is synced before returning.
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

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	timeout := time.After(2 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for node sync: %w", ctx.Err())
		case <-timeout:
			return fmt.Errorf("node did not finish syncing within timeout")
		case <-ticker.C:
			stat, err := rn.Client.Status(ctx)
			if err != nil {
				continue // retry on transient error
			}

			if stat != nil && stat.SyncInfo.CatchingUp {
				continue // still catching up, wait for next tick.
			}
			// node is synced
			return nil
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
