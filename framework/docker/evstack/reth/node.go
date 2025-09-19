package reth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"path"
	"sync"

	"github.com/ethereum/go-ethereum/ethclient"
	gethrpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

// NodeConfig is per-node configuration set by the builder.
type NodeConfig struct {
	// Image overrides chain-level image
	Image container.Image
	// AdditionalStartArgs overrides chain-level AdditionalStartArgs for this node
	AdditionalStartArgs []string
	// Env overrides chain-level Env for this node
	Env []string
	// InternalPorts allows overriding default container ports used by the node
	InternalPorts *types.Ports
	// GenesisBz overrides chain-level genesis for this node (optional)
	GenesisBz []byte
	// JWTSecretHex sets the node JWT secret in hex; if empty, it will be generated
	JWTSecretHex string
	// AdditionalInitArgs are appended to dump-genesis when generating a genesis
	AdditionalInitArgs []string
}

// NodeConfigBuilder provides a fluent builder for NodeConfig
type NodeConfigBuilder struct{ cfg *NodeConfig }

func NewNodeConfigBuilder() *NodeConfigBuilder {
	return &NodeConfigBuilder{cfg: &NodeConfig{}}
}

func (b *NodeConfigBuilder) WithImage(img container.Image) *NodeConfigBuilder {
	b.cfg.Image = img
	return b
}

func (b *NodeConfigBuilder) WithAdditionalStartArgs(args ...string) *NodeConfigBuilder {
	b.cfg.AdditionalStartArgs = args
	return b
}

func (b *NodeConfigBuilder) WithEnv(env ...string) *NodeConfigBuilder {
	b.cfg.Env = env
	return b
}

func (b *NodeConfigBuilder) WithInternalPorts(ports types.Ports) *NodeConfigBuilder {
	b.cfg.InternalPorts = &ports
	return b
}

func (b *NodeConfigBuilder) WithGenesis(genesis []byte) *NodeConfigBuilder {
	b.cfg.GenesisBz = genesis
	return b
}

func (b *NodeConfigBuilder) WithJWTSecretHex(secret string) *NodeConfigBuilder {
	b.cfg.JWTSecretHex = secret
	return b
}

func (b *NodeConfigBuilder) WithAdditionalInitArgs(args ...string) *NodeConfigBuilder {
	b.cfg.AdditionalInitArgs = args
	return b
}

func (b *NodeConfigBuilder) Build() NodeConfig {
	return *b.cfg
}

// Node represents a reth node container and its configuration.
type Node struct {
	*container.Node

	cfg       Config
	nodeCfg   NodeConfig
	logger    *zap.Logger
	started   bool
	mu        sync.Mutex
	internal  types.Ports
	external  types.Ports // RPC, P2P, API(ws), Engine, Metrics
	jwtHex    string
	genesisBz []byte
}

func newNode(ctx context.Context, cfg Config, testName string, index int, nodeCfg NodeConfig) (*Node, error) {
	image := cfg.Image
	if nodeCfg.Image.Repository != "" {
		image = nodeCfg.Image
	}

	ports := defaultPorts()
	if nodeCfg.InternalPorts != nil {
		ports = *nodeCfg.InternalPorts
	}

	log := cfg.Logger.With(zap.String("component", "reth-node"), zap.Int("i", index))

	homeDir := "/home/ev-reth"

	n := &Node{
		cfg:       cfg,
		nodeCfg:   nodeCfg,
		logger:    log,
		internal:  ports,
		genesisBz: nodeCfg.GenesisBz,
	}
    n.Node = container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, image, homeDir, index, rethNodeType("node"), log)
    n.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, n.Name()))

    // Create and setup volume now so Start doesn't need to
    if err := n.CreateAndSetupVolume(ctx, n.Name()); err != nil {
        return nil, err
    }
    return n, nil
}

// rethNodeType satisfies types.NodeType for container.Node
type rethNodeType string

func (t rethNodeType) String() string { return string(t) }

// Name returns a stable container name
func (n *Node) Name() string {
	return fmt.Sprintf("reth-%d-%s", n.Index, internal.SanitizeContainerName(n.TestName))
}

// HostName returns a condensed hostname
func (n *Node) HostName() string { return internal.CondenseHostName(n.Name()) }

// JWTSecretHex returns the JWT secret used by this node in hex encoding.
func (n *Node) JWTSecretHex() string { return n.jwtHex }

// InternalEngineURL returns the internal AuthRPC (Engine API) URL for this node using the container name.
// Using the container name ensures Docker DNS resolution across the test network.
func (n *Node) InternalEngineURL() string {
	return fmt.Sprintf("http://%s:%s", n.Name(), n.internal.Engine)
}

// InternalRPCURL returns the internal JSON-RPC URL for this node using the container name.
func (n *Node) InternalRPCURL() string {
	return fmt.Sprintf("http://%s:%s", n.Name(), n.internal.RPC)
}

// GenesisHash queries the node's JSON-RPC for the genesis block (0x0) hash.
// Requires the node to be started.
func (n *Node) GenesisHash(ctx context.Context) (string, error) {
	if !n.started {
		return "", fmt.Errorf("reth node not started")
	}
	ec, err := n.GetEthClient(ctx)
	if err != nil {
		return "", err
	}
	hdr, err := ec.HeaderByNumber(ctx, big.NewInt(0))
	if err != nil {
		return "", err
	}
	return hdr.Hash().Hex(), nil
}

// GetRPCClient returns a go-ethereum RPC client connected to this node's host-mapped RPC URL.
func (n *Node) GetRPCClient(ctx context.Context) (*gethrpc.Client, error) {
	if !n.started {
		return nil, fmt.Errorf("reth node not started")
	}
	ni, err := n.GetNetworkInfo(ctx)
	if err != nil {
		return nil, err
	}
	rpcURL := fmt.Sprintf("http://0.0.0.0:%s", ni.External.Ports.RPC)
	return gethrpc.DialContext(ctx, rpcURL)
}

// GetEthClient returns a go-ethereum ethclient.Client constructed from the underlying RPC client.
func (n *Node) GetEthClient(ctx context.Context) (*ethclient.Client, error) {
	rpcCli, err := n.GetRPCClient(ctx)
	if err != nil {
		return nil, err
	}
	return ethclient.NewClient(rpcCli), nil
}

// GetNetworkInfo returns internal/external network addressing for select ports
func (n *Node) GetNetworkInfo(ctx context.Context) (types.NetworkInfo, error) {
	internalIP, err := internal.GetContainerInternalIP(ctx, n.DockerClient, n.ContainerLifecycle.ContainerID())
	if err != nil {
		return types.NetworkInfo{}, err
	}
	return types.NetworkInfo{
		Internal: types.Network{Hostname: n.HostName(), IP: internalIP, Ports: types.Ports{RPC: n.internal.RPC, P2P: n.internal.P2P, API: n.internal.API, Engine: n.internal.Engine, Metrics: n.internal.Metrics}},
		External: types.Network{Hostname: "0.0.0.0", Ports: n.external},
	}, nil
}

// Start initializes required files, creates and starts the container
func (n *Node) Start(ctx context.Context) error {
    n.mu.Lock()
    defer n.mu.Unlock()
    if n.started {
        return n.StartContainer(ctx)
    }

	// Prepare JWT
	if n.nodeCfg.JWTSecretHex != "" {
		n.jwtHex = n.nodeCfg.JWTSecretHex
	} else {
		s, err := generateJWTSecretHex(32)
		if err != nil {
			return fmt.Errorf("generate jwt secret: %w", err)
		}
		n.jwtHex = s
	}

	// Always use built-in dev chain; skip genesis generation unless explicitly provided.
	if len(n.genesisBz) == 0 && len(n.cfg.GenesisFileBz) > 0 {
		n.genesisBz = n.cfg.GenesisFileBz
	}

	// Write files into the volume
	if err := n.writeNodeFiles(ctx); err != nil {
		return err
	}

	if err := n.createNodeContainer(ctx); err != nil {
		return err
	}
	if err := n.ContainerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	// Resolve host ports
	hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, n.internal.RPC+"/tcp", n.internal.P2P+"/tcp", n.internal.API+"/tcp", n.internal.Engine+"/tcp", n.internal.Metrics+"/tcp")
	if err != nil {
		return err
	}
	n.external = types.Ports{RPC: internal.MustExtractPort(hostPorts[0]), P2P: internal.MustExtractPort(hostPorts[1]), API: internal.MustExtractPort(hostPorts[2]), Engine: internal.MustExtractPort(hostPorts[3]), Metrics: internal.MustExtractPort(hostPorts[4])}

	n.started = true
	return nil
}

// Stop stops the node container
func (n *Node) Stop(ctx context.Context) error { return n.Node.Stop(ctx) }

// Remove stops and removes the node container and resources
func (n *Node) Remove(ctx context.Context, opts ...types.RemoveOption) error {
	return n.Node.Remove(ctx, opts...)
}

// Internal locations for jwt/genesis and datadir within the mounted home
func (n *Node) jwtPath() string     { return path.Join(n.HomeDir(), "jwt", "jwt.hex") }
func (n *Node) genesisPath() string { return path.Join(n.HomeDir(), "chain", "genesis.json") }
func (n *Node) dataDir() string     { return path.Join(n.HomeDir(), "eth-home") }

func (n *Node) writeNodeFiles(ctx context.Context) error {
	if err := n.WriteFile(ctx, path.Join("jwt", "jwt.hex"), []byte(n.jwtHex)); err != nil {
		return fmt.Errorf("write jwt: %w", err)
	}
	if len(n.genesisBz) > 0 {
		if err := n.WriteFile(ctx, path.Join("chain", "genesis.json"), n.genesisBz); err != nil {
			return fmt.Errorf("write genesis: %w", err)
		}
	}
	return nil
}

func (n *Node) createNodeContainer(ctx context.Context) error {
	cmd := []string{
		n.cfg.Bin, "node",
		"--chain", n.genesisPath(),
		"--datadir", n.dataDir(),
		"--metrics", "0.0.0.0:" + n.internal.Metrics,
		"--authrpc.addr", "0.0.0.0",
		"--authrpc.port", n.internal.Engine,
		"--authrpc.jwtsecret", n.jwtPath(),
		"--http", "--http.addr", "0.0.0.0", "--http.port", n.internal.RPC,
		"--http.api", "eth,net,web3,txpool",
		"--ws", "--ws.addr", "0.0.0.0", "--ws.port", n.internal.API,
		"--ws.api", "eth,net,web3",
		"--engine.persistence-threshold", "0",
		"--engine.memory-block-buffer-target", "0",
		"--disable-discovery",
		"--txpool.pending-max-count", "200000",
		"--txpool.pending-max-size", "200",
		"--txpool.queued-max-count", "200000",
		"--txpool.queued-max-size", "200",
		"--txpool.max-account-slots", "2048",
		"--txpool.max-new-txns", "2048",
		"--txpool.additional-validation-tasks", "16",
		"--ev-reth.enable",
	}

	// Merge chain and per-node additional args
	if len(n.cfg.AdditionalStartArgs) > 0 {
		cmd = append(cmd, n.cfg.AdditionalStartArgs...)
	}
	
	if len(n.nodeCfg.AdditionalStartArgs) > 0 {
		cmd = append(cmd, n.nodeCfg.AdditionalStartArgs...)
	}

	// Merge env
	var env []string
	env = append(env, n.cfg.Env...)
	env = append(env, n.nodeCfg.Env...)

	// Ports to expose
	usingPorts := nat.PortMap{
		nat.Port(n.internal.Metrics + "/tcp"): {},
		nat.Port(n.internal.P2P + "/tcp"):     {},
		nat.Port(n.internal.RPC + "/tcp"):     {},
		nat.Port(n.internal.Engine + "/tcp"):  {},
		nat.Port(n.internal.API + "/tcp"):     {},
	}

	return n.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, usingPorts, "", n.Bind(), nil, n.HostName(), cmd, env, []string{})
}

func generateJWTSecretHex(nbytes int) (string, error) {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
