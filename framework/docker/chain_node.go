package docker

import (
	"bytes"
	"context"
	"fmt"
	dockerinternal "github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/testutil/config"
	"github.com/celestiaorg/tastora/framework/types"
	cometcfg "github.com/cometbft/cometbft/config"
	tmjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/p2p"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	libclient "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	servercfg "github.com/cosmos/cosmos-sdk/server/config"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"hash/fnv"
	"os"
	"path"
	"strconv"
	"sync"
	"time"
)

var _ types.ChainNode = &ChainNode{}

const (
	valKey      = "validator"
	blockTime   = 2 // seconds
	p2pPort     = "26656/tcp"
	rpcPort     = "26657/tcp"
	grpcPort    = "9090/tcp"
	apiPort     = "1317/tcp"
	privValPort = "1234/tcp"
)

// GetInternalPeerAddress retrieves the internal peer address for the ChainNode using its ID, hostname, and default port.
func (cn *ChainNode) GetInternalPeerAddress(ctx context.Context) (string, error) {
	id, err := cn.nodeID(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%s:%d", id, cn.HostName(), 26656), nil
}

// GetInternalRPCAddress returns the internal RPC address of the chain node in the format "nodeID@hostname:port".
func (cn *ChainNode) GetInternalRPCAddress(ctx context.Context) (string, error) {
	id, err := cn.nodeID(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%s:%d", id, cn.HostName(), 26657), nil
}

type ChainNodes []*ChainNode

type ChainNode struct {
	*ContainerNode
	cfg       Config
	Validator bool
	Client    rpcclient.Client
	GrpcConn  *grpc.ClientConn

	lock sync.Mutex

	// Ports set during startContainer.
	hostRPCPort  string
	hostAPIPort  string
	hostGRPCPort string
	hostP2PPort  string
}

func (cn *ChainNode) GetInternalHostName(ctx context.Context) (string, error) {
	return cn.HostName(), nil
}

func (cn *ChainNode) HostName() string {
	return CondenseHostName(cn.Name())
}

func (cn *ChainNode) GetType() string {
	return cn.NodeType()
}

func (cn *ChainNode) GetRPCClient() (rpcclient.Client, error) {
	return cn.Client, nil
}

// GetKeyring retrieves the keyring instance for the ChainNode. The keyring will be usable
// by the host running the test.
func (cn *ChainNode) GetKeyring() (keyring.Keyring, error) {
	containerKeyringDir := path.Join(cn.homeDir, "keyring-test")
	return dockerinternal.NewDockerKeyring(cn.DockerClient, cn.containerLifecycle.ContainerID(), containerKeyringDir, cn.cfg.ChainConfig.EncodingConfig.Codec), nil
}

func NewDockerChainNode(log *zap.Logger, validator bool, cfg Config, testName string, image DockerImage, index int) *ChainNode {
	nodeType := "fn"
	if validator {
		nodeType = "val"
	}

	log = log.With(
		zap.Bool("validator", validator),
		zap.Int("i", index),
	)
	tn := &ChainNode{
		Validator:     validator,
		cfg:           cfg,
		ContainerNode: newContainerNode(cfg.DockerNetworkID, cfg.DockerClient, testName, image, path.Join("/var/cosmos-chain", cfg.ChainConfig.Name), index, nodeType, log),
	}

	tn.containerLifecycle = NewContainerLifecycle(log, cfg.DockerClient, tn.Name())

	return tn
}

// Name of the test node container.
func (cn *ChainNode) Name() string {
	return fmt.Sprintf("%s-%s-%d-%s", cn.cfg.ChainConfig.ChainID, cn.NodeType(), cn.Index, SanitizeContainerName(cn.TestName))
}

// NodeType returns the type of the ChainNode as a string: "fn" for full nodes and "val" for validator nodes.
func (cn *ChainNode) NodeType() string {
	nodeType := "fn"
	if cn.Validator {
		nodeType = "val"
	}
	return nodeType
}

// Height retrieves the latest block height of the chain node using the Tendermint RPC client.
func (cn *ChainNode) Height(ctx context.Context) (int64, error) {
	res, err := cn.Client.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("tendermint rpc client status: %w", err)
	}
	height := res.SyncInfo.LatestBlockHeight
	return height, nil
}

// Exec runs a command in the node's container.
// returns stdout, stdin, and an error if one occurred.
func (cn *ChainNode) Exec(ctx context.Context, cmd []string, env []string) ([]byte, []byte, error) {
	return cn.exec(ctx, cn.logger(), cmd, env)
}

// initNodeFiles initializes essential file structures for a chain node by creating its home folder and setting configurations.
func (cn *ChainNode) initNodeFiles(ctx context.Context) error {
	if err := cn.initHomeFolder(ctx); err != nil {
		return err
	}

	return cn.setTestConfig(ctx)
}

// binCommand is a helper to retrieve a full command for a chain node binary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to return the full command.
// Will include additional flags for home directory and chain ID.
func (cn *ChainNode) binCommand(command ...string) []string {
	command = append([]string{cn.cfg.ChainConfig.Bin}, command...)
	return append(command,
		"--home", cn.homeDir,
	)
}

// execBin is a helper to execute a command for a chain node binary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to execute the command against the node.
// Will include additional flags for home directory and chain ID.
func (cn *ChainNode) execBin(ctx context.Context, command ...string) ([]byte, []byte, error) {
	return cn.exec(ctx, cn.logger(), cn.binCommand(command...), cn.cfg.ChainConfig.Env)
}

// initHomeFolder initializes a home folder for the given node.
func (cn *ChainNode) initHomeFolder(ctx context.Context) error {
	cn.lock.Lock()
	defer cn.lock.Unlock()

	_, _, err := cn.execBin(ctx,
		"init", CondenseMoniker(cn.Name()),
		"--chain-id", cn.cfg.ChainConfig.ChainID,
	)
	return err
}

// setTestConfig modifies the config to reasonable values for use within celestia-test.
func (cn *ChainNode) setTestConfig(ctx context.Context) error {
	err := config.Modify(ctx, cn, "config/config.toml", func(cfg *cometcfg.Config) {
		cfg.LogLevel = "info"
		cfg.TxIndex.Indexer = "kv"
		cfg.P2P.AllowDuplicateIP = true
		cfg.P2P.AddrBookStrict = false

		blockTime := time.Duration(blockTime) * time.Second

		cfg.Consensus.TimeoutCommit = blockTime
		cfg.Consensus.TimeoutPropose = blockTime

		cfg.RPC.ListenAddress = "tcp://0.0.0.0:26657"
		cfg.RPC.CORSAllowedOrigins = []string{"*"}
	})

	if err != nil {
		return fmt.Errorf("modifying config/config.toml: %w", err)
	}

	err = config.Modify(ctx, cn, "config/app.toml", func(cfg *servercfg.Config) {
		cfg.MinGasPrices = cn.cfg.ChainConfig.GasPrices
		cfg.GRPC.Address = "0.0.0.0:9090"
		cfg.API.Enable = true
		cfg.API.Swagger = true
		cfg.API.Address = "tcp://0.0.0.0:1317"
	})

	if err != nil {
		return fmt.Errorf("modifying config/app.toml: %w", err)
	}

	return nil
}

func (cn *ChainNode) logger() *zap.Logger {
	return cn.ContainerNode.logger.With(
		zap.String("chain_id", cn.cfg.ChainConfig.ChainID),
		zap.String("test", cn.TestName),
	)
}

// startContainer starts the container for the ChainNode, initializes its ports, and ensures the node is synced before returning.
// Returns an error if the container fails to start, ports cannot be set, or syncing is not completed within the timeout.
func (cn *ChainNode) startContainer(ctx context.Context) error {
	if err := cn.containerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	// Set the host ports once since they will not change after the container has started.
	hostPorts, err := cn.containerLifecycle.GetHostPorts(ctx, rpcPort, grpcPort, apiPort, p2pPort)
	if err != nil {
		return err
	}
	cn.hostRPCPort, cn.hostGRPCPort, cn.hostAPIPort, cn.hostP2PPort = hostPorts[0], hostPorts[1], hostPorts[2], hostPorts[3]

	err = cn.initClient("tcp://" + cn.hostRPCPort)
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
			stat, err := cn.Client.Status(ctx)
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
func (cn *ChainNode) initClient(addr string) error {
	httpClient, err := libclient.DefaultHTTPClient(addr)
	if err != nil {
		return err
	}

	httpClient.Timeout = 10 * time.Second
	rpcClient, err := rpchttp.NewWithClient(addr, "/websocket", httpClient)
	if err != nil {
		return err
	}

	cn.Client = rpcClient

	grpcConn, err := grpc.NewClient(
		cn.hostGRPCPort, grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("grpc dial: %w", err)
	}
	cn.GrpcConn = grpcConn

	return nil
}

// stop stops the underlying container.
func (cn *ChainNode) stop(ctx context.Context) error {
	return cn.containerLifecycle.StopContainer(ctx)
}

// setPeers modifies the config persistent_peers for a node.
func (cn *ChainNode) setPeers(ctx context.Context, peers string) error {
	return config.Modify(ctx, cn, "config/config.toml", func(cfg *cometcfg.Config) {
		cfg.P2P.PersistentPeers = peers
	})
}

// createNodeContainer initializes but does not start a container for the ChainNode with the specified configuration and context.
func (cn *ChainNode) createNodeContainer(ctx context.Context) error {
	chainCfg := cn.cfg.ChainConfig

	cmd := []string{chainCfg.Bin, "start", "--home", cn.homeDir}
	if startArgs := cn.getAdditionalStartArgs(); len(startArgs) > 0 {
		cmd = append(cmd, startArgs...)
	}

	usingPorts := nat.PortMap{}
	for k, v := range sentryPorts {
		usingPorts[k] = v
	}

	return cn.containerLifecycle.CreateContainer(ctx, cn.TestName, cn.NetworkID, cn.getImage(), usingPorts, "", cn.bind(), nil, cn.HostName(), cmd, cn.getEnv(), []string{})
}

// overwriteGenesisFile writes the provided content to the genesis.json file in the container's configuration directory.
// returns an error if writing the file fails.
func (cn *ChainNode) overwriteGenesisFile(ctx context.Context, content []byte) error {
	err := cn.WriteFile(ctx, "config/genesis.json", content)
	if err != nil {
		return fmt.Errorf("overwriting genesis.json: %w", err)
	}

	return nil
}

// collectGentxs runs collect gentxs on the node's home folders.
func (cn *ChainNode) collectGentxs(ctx context.Context) error {
	cn.lock.Lock()
	defer cn.lock.Unlock()

	command := []string{cn.cfg.ChainConfig.Bin, "genesis", "collect-gentxs", "--home", cn.homeDir}

	_, _, err := cn.exec(ctx, cn.logger(), command, cn.cfg.ChainConfig.Env)
	return err
}

// genesisFileContent retrieves the contents of the "config/genesis.json" file from the node's container volume.
func (cn *ChainNode) genesisFileContent(ctx context.Context) ([]byte, error) {
	gen, err := cn.ReadFile(ctx, "config/genesis.json")
	if err != nil {
		return nil, fmt.Errorf("getting genesis.json content: %w", err)
	}

	return gen, nil
}

// copyGentx transfers a gentx file from the source ChainNode to the destination ChainNode by reading and writing the file.
func (cn *ChainNode) copyGentx(ctx context.Context, destVal *ChainNode) error {
	nid, err := cn.nodeID(ctx)
	if err != nil {
		return fmt.Errorf("getting node ID: %w", err)
	}

	relPath := fmt.Sprintf("config/gentx/gentx-%s.json", nid)

	gentx, err := cn.ReadFile(ctx, relPath)
	if err != nil {
		return fmt.Errorf("getting gentx content: %w", err)
	}

	err = destVal.WriteFile(ctx, relPath, gentx)
	if err != nil {
		return fmt.Errorf("overwriting gentx: %w", err)
	}

	return nil
}

// nodeID returns the persistent ID of a given node.
func (cn *ChainNode) nodeID(ctx context.Context) (string, error) {
	// This used to call p2p.LoadNodeKey against the file on the host,
	// but because we are transitioning to operating on Docker volumes,
	// we only have to tmjson.Unmarshal the raw content.
	j, err := cn.ReadFile(ctx, "config/node_key.json")
	if err != nil {
		return "", fmt.Errorf("getting node_key.json content: %w", err)
	}

	var nk p2p.NodeKey
	if err := tmjson.Unmarshal(j, &nk); err != nil {
		return "", fmt.Errorf("unmarshaling node_key.json: %w", err)
	}

	return string(nk.ID()), nil
}

// addGenesisAccount adds a genesis account for each key.
func (cn *ChainNode) addGenesisAccount(ctx context.Context, address string, genesisAmount []sdk.Coin) error {
	cn.lock.Lock()
	defer cn.lock.Unlock()

	amount := ""
	for i, coin := range genesisAmount {
		if i != 0 {
			amount += ","
		}
		amount += fmt.Sprintf("%s%s", coin.Amount.String(), coin.Denom)
	}

	// Adding a genesis account should complete instantly,
	// so use a 1-minute timeout to more quickly detect if Docker has locked up.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	command := []string{"genesis", "add-genesis-account", address, amount}
	_, _, err := cn.execBin(ctx, command...)

	return err
}

// initValidatorGenTx creates the node files and signs a genesis transaction.
func (cn *ChainNode) initValidatorGenTx(
	ctx context.Context,
	genesisAmounts []sdk.Coin,
	genesisSelfDelegation sdk.Coin,
) error {
	if err := cn.createKey(ctx, valKey); err != nil {
		return err
	}
	bech32, err := cn.accountKeyBech32(ctx, valKey)
	if err != nil {
		return err
	}
	if err := cn.addGenesisAccount(ctx, bech32, genesisAmounts); err != nil {
		return err
	}
	return cn.gentx(ctx, valKey, genesisSelfDelegation)
}

// createKey creates a key in the keyring backend test for the given node.
func (cn *ChainNode) createKey(ctx context.Context, name string) error {
	cn.lock.Lock()
	defer cn.lock.Unlock()

	_, _, err := cn.execBin(ctx,
		"keys", "add", name,
		"--coin-type", cn.cfg.ChainConfig.CoinType,
		"--keyring-backend", keyring.BackendTest,
	)
	return err
}

// Gentx generates the gentx for a given node.
func (cn *ChainNode) gentx(ctx context.Context, name string, genesisSelfDelegation sdk.Coin) error {
	cn.lock.Lock()
	defer cn.lock.Unlock()

	command := []string{"genesis", "gentx", name, fmt.Sprintf("%s%s", genesisSelfDelegation.Amount.String(), genesisSelfDelegation.Denom),
		"--gas-prices", cn.cfg.ChainConfig.GasPrices,
		"--gas-adjustment", fmt.Sprint(cn.cfg.ChainConfig.GasAdjustment),
		"--keyring-backend", keyring.BackendTest,
		"--chain-id", cn.cfg.ChainConfig.ChainID,
	}

	_, _, err := cn.execBin(ctx, command...)
	return err
}

// accountKeyBech32 retrieves the named key's address in bech32 account format.
func (cn *ChainNode) accountKeyBech32(ctx context.Context, name string) (string, error) {
	return cn.keyBech32(ctx, name, "")
}

// CliContext creates a new Cosmos SDK client context.
func (cn *ChainNode) CliContext() client.Context {
	cfg := cn.cfg.ChainConfig
	return client.Context{
		Client:            cn.Client,
		GRPCClient:        cn.GrpcConn,
		ChainID:           cfg.ChainID,
		InterfaceRegistry: cfg.EncodingConfig.InterfaceRegistry,
		Input:             os.Stdin,
		Output:            os.Stdout,
		OutputFormat:      "json",
		LegacyAmino:       cfg.EncodingConfig.Amino,
		TxConfig:          cfg.EncodingConfig.TxConfig,
	}
}

// keyBech32 retrieves the named key's address in bech32 format from the node.
// bech is the bech32 prefix (acc|val|cons). If empty, defaults to the account key (same as "acc").
func (cn *ChainNode) keyBech32(ctx context.Context, name string, bech string) (string, error) {
	command := []string{
		cn.cfg.ChainConfig.Bin, "keys", "show", "--address", name,
		"--home", cn.homeDir,
		"--keyring-backend", keyring.BackendTest,
	}

	if bech != "" {
		command = append(command, "--bech", bech)
	}

	stdout, stderr, err := cn.exec(ctx, cn.logger(), command, cn.cfg.ChainConfig.Env)
	if err != nil {
		return "", fmt.Errorf("failed to show key %q (stderr=%q): %w", name, stderr, err)
	}

	return string(bytes.TrimSuffix(stdout, []byte("\n"))), nil
}

// getNodeConfig returns the per-node configuration if it exists
func (cn *ChainNode) getNodeConfig() *ChainNodeConfig {
	if cn.cfg.ChainConfig.ChainNodeConfigs == nil {
		return nil
	}

	cfg, ok := cn.cfg.ChainConfig.ChainNodeConfigs[cn.Index]
	if !ok {
		cn.logger().Warn("no node config found for node", zap.Int("index", cn.Index))
	}

	return cfg
}

// getAdditionalStartArgs returns the start arguments for this node, preferring per-node config over chain config
func (cn *ChainNode) getAdditionalStartArgs() []string {
	if nodeConfig := cn.getNodeConfig(); nodeConfig != nil && nodeConfig.AdditionalStartArgs != nil {
		return nodeConfig.AdditionalStartArgs
	}
	return cn.cfg.ChainConfig.AdditionalStartArgs
}

// getImage returns the Docker image for this node, preferring per-node config over the default image
func (cn *ChainNode) getImage() DockerImage {
	if nodeConfig := cn.getNodeConfig(); nodeConfig != nil && nodeConfig.Image != nil {
		return *nodeConfig.Image
	}
	return cn.Image
}

// getEnv returns the environment variables for this node, preferring per-node config over chain config
func (cn *ChainNode) getEnv() []string {
	if nodeConfig := cn.getNodeConfig(); nodeConfig != nil && nodeConfig.Env != nil {
		return nodeConfig.Env
	}
	return cn.cfg.ChainConfig.Env
}

// CondenseMoniker fits a moniker into the cosmos character limit for monikers.
// If the moniker already fits, it is returned unmodified.
// Otherwise, the middle is truncated, and a hash is appended to the end
// in case the only unique data was in the middle.
func CondenseMoniker(m string) string {
	if len(m) <= stakingtypes.MaxMonikerLength {
		return m
	}

	// Get the hash suffix, a 32-bit uint formatted in base36.
	// fnv32 was chosen because a 32-bit number ought to be sufficient
	// as a distinguishing suffix, and it will be short enough so that
	// less of the middle will be truncated to fit in the character limit.
	// It's also non-cryptographic, not that this function will ever be a bottleneck in tests.
	h := fnv.New32()
	h.Write([]byte(m))
	suffix := "-" + strconv.FormatUint(uint64(h.Sum32()), 36)

	wantLen := stakingtypes.MaxMonikerLength - len(suffix)

	// Half of the want length, minus 2 to account for half of the ... we add in the middle.
	keepLen := (wantLen / 2) - 2

	return m[:keepLen] + "..." + m[len(m)-keepLen:] + suffix
}
