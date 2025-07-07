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
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/docker/go-connections/nat"
	dockerclient "github.com/moby/moby/client"
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
	ChainNodeParams
	*ContainerNode
	Client   rpcclient.Client
	GrpcConn *grpc.ClientConn

	lock sync.Mutex

	// Ports set during startContainer.
	hostRPCPort  string
	hostAPIPort  string
	hostGRPCPort string
	hostP2PPort  string
}

// ChainNodeParams contains the chain-specific parameters needed to create a ChainNode
type ChainNodeParams struct {
	Validator           bool
	ChainID             string
	BinaryName          string
	CoinType            string
	GasPrices           string
	GasAdjustment       float64
	Env                 []string
	AdditionalStartArgs []string
	EncodingConfig      *testutil.TestEncodingConfig
	// Optional fields for key preloading
	GenesisKeyring   keyring.Keyring
	ValidatorIndex   int
	PrivValidatorKey []byte
	// PostInit functions are executed sequentially after the node is initialized.
	PostInit []func(ctx context.Context, node *ChainNode) error
}

// NewChainNode creates a new ChainNode with injected dependencies
func NewChainNode(
	logger *zap.Logger,
	dockerClient *dockerclient.Client,
	dockerNetworkID string,
	testName string,
	image DockerImage,
	homeDir string,
	index int,
	chainParams ChainNodeParams,
) *ChainNode {
	nodeType := "fn"
	if chainParams.Validator {
		nodeType = "val"
	}

	log := logger.With(
		zap.Bool("validator", chainParams.Validator),
		zap.Int("i", index),
	)

	tn := &ChainNode{
		ChainNodeParams: chainParams,
		ContainerNode:   newContainerNode(dockerNetworkID, dockerClient, testName, image, homeDir, index, nodeType, log),
	}

	tn.containerLifecycle = NewContainerLifecycle(logger, dockerClient, tn.Name())

	return tn
}

// GetInternalHostName retrieves the internal host name of the ChainNode instance. Returns the host name and an error if any.
func (cn *ChainNode) GetInternalHostName(ctx context.Context) (string, error) {
	return cn.HostName(), nil
}

// HostName returns the condensed hostname for the ChainNode, truncating if the name is 64 characters or longer.
func (cn *ChainNode) HostName() string {
	return CondenseHostName(cn.Name())
}

// GetType returns the type of the chain node as a string by invoking the NodeType method.
func (cn *ChainNode) GetType() string {
	return cn.NodeType()
}

// GetRPCClient returns the RPC client associated with the ChainNode instance.
func (cn *ChainNode) GetRPCClient() (rpcclient.Client, error) {
	return cn.Client, nil
}

// GetKeyring retrieves the keyring instance for the ChainNode. The keyring will be usable
// by the host running the test.
func (cn *ChainNode) GetKeyring() (keyring.Keyring, error) {
	containerKeyringDir := path.Join(cn.homeDir, "keyring-test")
	return dockerinternal.NewDockerKeyring(cn.DockerClient, cn.containerLifecycle.ContainerID(), containerKeyringDir, cn.EncodingConfig.Codec), nil
}

// Name of the test node container.
func (cn *ChainNode) Name() string {
	return fmt.Sprintf("%s-%s-%d-%s", cn.ChainID, cn.NodeType(), cn.Index, SanitizeContainerName(cn.TestName))
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
	command = append([]string{cn.BinaryName}, command...)
	return append(command,
		"--home", cn.homeDir,
	)
}

// execBin is a helper to execute a command for a chain node binary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to execute the command against the node.
// Will include additional flags for home directory and chain ID.
func (cn *ChainNode) execBin(ctx context.Context, command ...string) ([]byte, []byte, error) {
	return cn.exec(ctx, cn.logger(), cn.binCommand(command...), cn.Env)
}

// initHomeFolder initializes a home folder for the given node.
func (cn *ChainNode) initHomeFolder(ctx context.Context) error {
	cn.lock.Lock()
	defer cn.lock.Unlock()

	_, _, err := cn.execBin(ctx,
		"init", CondenseMoniker(cn.Name()),
		"--chain-id", cn.ChainID,
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
		cfg.MinGasPrices = cn.GasPrices
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
		zap.String("chain_id", cn.ChainID),
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
	cmd := []string{cn.BinaryName, "start", "--home", cn.homeDir}
	if len(cn.AdditionalStartArgs) > 0 {
		cmd = append(cmd, cn.AdditionalStartArgs...)
	}
	usingPorts := nat.PortMap{}
	for k, v := range sentryPorts {
		usingPorts[k] = v
	}

	return cn.containerLifecycle.CreateContainer(ctx, cn.TestName, cn.NetworkID, cn.Image, usingPorts, "", cn.bind(), nil, cn.HostName(), cmd, cn.Env, []string{})
}

// overwriteGenesisFile writes the provided content to the genesis.json file in the container's configuration directory.
// returns an error if writing the file fails.
func (cn *ChainNode) overwriteGenesisFile(ctx context.Context, content []byte) error {
	err := cn.WriteFile(ctx, "config/genesis.json", content)
	if err != nil {
		return fmt.Errorf("overwriting genesis.json: %w", err)
	}
	cn.logger().Debug("successfully wrote genesis file contents")
	return nil
}

// overwritePrivValidatorKey overwrites the private validator key after init
func (cn *ChainNode) overwritePrivValidatorKey(ctx context.Context, contents []byte) error {
	err := cn.WriteFile(ctx, "config/priv_validator_key.json", contents)
	if err != nil {
		return fmt.Errorf("overwriting priv_validator_key.json: %w", err)
	}
	cn.logger().Debug("successfully wrote private validator key")
	return nil
}

// collectGentxs runs collect gentxs on the node's home folders.
func (cn *ChainNode) collectGentxs(ctx context.Context) error {
	cn.lock.Lock()
	defer cn.lock.Unlock()

	command := []string{cn.BinaryName, "genesis", "collect-gentxs", "--home", cn.homeDir}

	_, _, err := cn.exec(ctx, cn.logger(), command, cn.Env)
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
		"--coin-type", cn.CoinType,
		"--keyring-backend", keyring.BackendTest,
	)
	return err
}

// Gentx generates the gentx for a given node.
func (cn *ChainNode) gentx(ctx context.Context, name string, genesisSelfDelegation sdk.Coin) error {
	cn.lock.Lock()
	defer cn.lock.Unlock()

	command := []string{"genesis", "gentx", name, fmt.Sprintf("%s%s", genesisSelfDelegation.Amount.String(), genesisSelfDelegation.Denom),
		"--gas-prices", cn.GasPrices,
		"--gas-adjustment", fmt.Sprint(cn.GasAdjustment),
		"--keyring-backend", keyring.BackendTest,
		"--chain-id", cn.ChainID,
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
	return client.Context{
		Client:            cn.Client,
		GRPCClient:        cn.GrpcConn,
		ChainID:           cn.ChainID,
		InterfaceRegistry: cn.EncodingConfig.InterfaceRegistry,
		Input:             os.Stdin,
		Output:            os.Stdout,
		OutputFormat:      "json",
		LegacyAmino:       cn.EncodingConfig.Amino,
		TxConfig:          cn.EncodingConfig.TxConfig,
	}
}

// keyBech32 retrieves the named key's address in bech32 format from the node.
// bech is the bech32 prefix (acc|val|cons). If empty, defaults to the account key (same as "acc").
func (cn *ChainNode) keyBech32(ctx context.Context, name string, bech string) (string, error) {
	command := []string{
		cn.BinaryName, "keys", "show", "--address", name,
		"--home", cn.homeDir,
		"--keyring-backend", keyring.BackendTest,
	}

	if bech != "" {
		command = append(command, "--bech", bech)
	}

	stdout, stderr, err := cn.exec(ctx, cn.logger(), command, cn.Env)
	if err != nil {
		return "", fmt.Errorf("failed to show key %q (stderr=%q): %w", name, stderr, err)
	}

	return string(bytes.TrimSuffix(stdout, []byte("\n"))), nil
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
