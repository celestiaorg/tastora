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
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

// Node represents a reth node container and its configuration.
type Node struct {
	*container.Node

	cfg      Config
	logger   *zap.Logger
	started  bool
	mu       sync.Mutex
	external types.Ports // RPC, P2P, API(ws), Engine, Metrics
}

// newNode creates a new Reth node instance with the provided configuration. This creates the underlying docker resources
// but does not start the container.
func newNode(ctx context.Context, cfg Config, testName string, index int) (*Node, error) {
	image := cfg.Image

	log := cfg.Logger.With(zap.String("component", "reth-node"), zap.Int("i", index))

	homeDir := "/home/ev-reth"
	n := &Node{
		cfg:    cfg,
		logger: log,
	}
	n.Node = container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, image, homeDir, index, NodeType, log)
	n.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, n.Name()))

	if err := n.CreateAndSetupVolume(ctx, n.Name()); err != nil {
		return nil, err
	}

	return n, nil
}

// Name returns a stable container name
func (n *Node) Name() string {
	return fmt.Sprintf("reth-%d-%s", n.Index, internal.SanitizeDockerResourceName(n.TestName))
}

// HostName returns a condensed hostname
func (n *Node) HostName() string {
	return internal.CondenseHostName(n.Name())
}

// JWTSecretHex returns the JWT secret used by this node in hex encoding.
func (n *Node) JWTSecretHex() string {
	return n.cfg.JWTSecretHex
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

// GetNetworkInfo returns internal/external network address information.
func (n *Node) GetNetworkInfo(ctx context.Context) (types.NetworkInfo, error) {
	internalIP, err := internal.GetContainerInternalIP(ctx, n.DockerClient, n.ContainerLifecycle.ContainerID())
	if err != nil {
		return types.NetworkInfo{}, err
	}
	return types.NetworkInfo{
		Internal: types.Network{Hostname: n.HostName(), IP: internalIP, Ports: defaultInternalPorts()},
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

	if n.cfg.JWTSecretHex == "" {
		s, err := generateJWTSecretHex(32)
		if err != nil {
			return fmt.Errorf("generate jwt secret: %w", err)
		}
		n.cfg.JWTSecretHex = s
	}

	//  skip genesis generation unless explicitly provided.
	// TODO: support genesis creation without a fixture.
	if len(n.cfg.GenesisFileBz) == 0 {
		return fmt.Errorf("error unimplemented: automatic genesis generation not yet supported")
	}

	if err := n.writeNodeFiles(ctx); err != nil {
		return fmt.Errorf("write node files: %w", err)
	}

	if err := n.createNodeContainer(ctx); err != nil {
		return fmt.Errorf("create node container: %w", err)
	}

	if err := n.ContainerLifecycle.StartContainer(ctx); err != nil {
		return fmt.Errorf("start node container: %w", err)
	}

	// resolve host ports
	p := defaultInternalPorts()
	hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, p.RPC+"/tcp", p.P2P+"/tcp", p.API+"/tcp", p.Engine+"/tcp", p.Metrics+"/tcp")
	if err != nil {
		return fmt.Errorf("get host ports: %w", err)
	}
	n.external = types.Ports{RPC: internal.MustExtractPort(hostPorts[0]), P2P: internal.MustExtractPort(hostPorts[1]), API: internal.MustExtractPort(hostPorts[2]), Engine: internal.MustExtractPort(hostPorts[3]), Metrics: internal.MustExtractPort(hostPorts[4])}

	n.started = true
	return nil
}

// jwtPath returns the path to the JWT secret file inside the container.
func (n *Node) jwtPath() string {
	return path.Join(n.HomeDir(), "jwt", "jwt.hex")
}

// genesisPath returns the path to the genesis file inside the container.
func (n *Node) genesisPath() string {
	return path.Join(n.HomeDir(), "chain", "genesis.json")
}

// dataDir returns the path to the node's data directory inside the container.
func (n *Node) dataDir() string {
	return path.Join(n.HomeDir(), "eth-home")
}

// writeNodeFiles writes necessary files (genesis, jwt) to the node's volume.
func (n *Node) writeNodeFiles(ctx context.Context) error {
	if err := n.WriteFile(ctx, path.Join("jwt", "jwt.hex"), []byte(n.cfg.JWTSecretHex)); err != nil {
		return fmt.Errorf("write jwt: %w", err)
	}
	if len(n.cfg.GenesisFileBz) > 0 {
		if err := n.WriteFile(ctx, path.Join("chain", "genesis.json"), n.cfg.GenesisFileBz); err != nil {
			return fmt.Errorf("write genesis: %w", err)
		}
	}
	return nil
}

// createNodeContainer constructs and creates the docker container for the node.
func (n *Node) createNodeContainer(ctx context.Context) error {
	internalPorts := defaultInternalPorts()
	cmd := []string{
		n.cfg.Bin, "node",
		"--chain", n.genesisPath(),
		"--datadir", n.dataDir(),
		"--metrics", "0.0.0.0:" + internalPorts.Metrics,
		"--authrpc.addr", "0.0.0.0",
		"--authrpc.port", internalPorts.Engine,
		"--authrpc.jwtsecret", n.jwtPath(),
		"--http", "--http.addr", "0.0.0.0", "--http.port", internalPorts.RPC,
		"--http.api", "eth,net,web3,txpool",
		"--ws", "--ws.addr", "0.0.0.0", "--ws.port", internalPorts.API,
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
		"--rpc.eth-proof-window", "120000",
	}

	// Use builder-level start args and env without per-node overrides.
	cmd = append(cmd, n.cfg.AdditionalStartArgs...)
	env := n.cfg.Env

	// ports to expose
	usingPorts := nat.PortMap{
		nat.Port(internalPorts.Metrics + "/tcp"): {},
		nat.Port(internalPorts.P2P + "/tcp"):     {},
		nat.Port(internalPorts.RPC + "/tcp"):     {},
		nat.Port(internalPorts.Engine + "/tcp"):  {},
		nat.Port(internalPorts.API + "/tcp"):     {},
	}

	return n.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, usingPorts, "", n.Bind(), nil, n.HostName(), cmd, env, []string{})
}

// GetHyperlaneChainMetadata returns the Hyperlane configuration metadata for this reth node.
func (n *Node) GetHyperlaneChainMetadata(ctx context.Context) (hyperlane.ChainMetadata, error) {
	ni, err := n.GetNetworkInfo(ctx)
	if err != nil {
		return hyperlane.ChainMetadata{}, fmt.Errorf("get network info: %w", err)
	}

	internalPorts := defaultInternalPorts()
	rpcURL := fmt.Sprintf("http://%s:%s", ni.Internal.Hostname, internalPorts.RPC)

	return hyperlane.ChainMetadata{
		ChainID:     1234,
		DomainID:    1234,
		Name:        "rethlocal",
		DisplayName: "Reth",
		Protocol:    "ethereum",
		IsTestnet:   true,
		NativeToken: hyperlane.NativeToken{
			Name:     "Ether",
			Symbol:   "ETH",
			Decimals: 18,
		},
		RpcURLs: []hyperlane.Endpoint{
			{HTTP: rpcURL},
		},
		Blocks: &hyperlane.BlockConfig{
			Confirmations:     1,
			EstimateBlockTime: 3,
			ReorgPeriod:       0,
		},
		SignerKey: "0x82bfcfadbf1712f6550d8d2c00a39f05b33ec78939d0167be2a737d691f33a6a",
		CoreContracts: &hyperlane.CoreContractAddresses{
			Mailbox:                  "0xb1c938F5BA4B3593377F399e12175e8db0C787Ff",
			InterchainSecurityModule: "0xa05915fD6E32A1AA7E67d800164CaCB12487142d",
			InterchainGasPaymaster:   "0x1D957dA7A6988f5a9d2D2454637B4B7fea0Aeea5",
			MerkleTreeHook:           "0xFCb1d485ef46344029D9E8A7925925e146B3430E",
			ProxyAdmin:               "0x7e7aD18Adc99b94d4c728fDf13D4dE97B926A0D8",
			ValidatorAnnounce:        "0x79ec7bF05AF122D3782934d4Fb94eE32f0C01c97",
			AggregationHook:          "0xe53275A1FcA119e1c5eeB32E7a72e54835A63936",
			DomainRoutingIsm:         "0xE2c1756b8825C54638f98425c113b51730cc47f6",
			FallbackRoutingHook:      "0xE2c1756b8825C54638f98425c113b51730cc47f6",
			ProtocolFee:              "0x8A93d247134d91e0de6f96547cB0204e5BE8e5D8",
			StorageGasOracle:         "0x457cCf29090fe5A24c19c1bc95F492168C0EaFdb",
			TestRecipient:            "0xd7958B336f0019081Ad2279B2B7B7c3f744Bce0a",
		},
	}, nil
}

// generateJWTSecretHex generates a random JWT secret of nbytes length and returns it as a hex-encoded string.
func generateJWTSecretHex(nbytes int) (string, error) {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
