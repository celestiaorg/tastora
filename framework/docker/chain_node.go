package docker

import (
	"bytes"
	"context"
	"fmt"
	dockerinternal "github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/types"
	tmjson "github.com/cometbft/cometbft/libs/json"
	"github.com/cometbft/cometbft/p2p"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	libclient "github.com/cometbft/cometbft/rpc/jsonrpc/client"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
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

func (n *ChainNode) GetInternalPeerAddress(ctx context.Context) (string, error) {
	id, err := n.nodeID(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%s:%d", id, n.HostName(), 26656), nil
}

func (n *ChainNode) GetInternalRPCAddress(ctx context.Context) (string, error) {
	id, err := n.nodeID(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%s:%d", id, n.HostName(), 26657), nil
}

type ChainNodes []*ChainNode

type ChainNode struct {
	*node
	Validator bool
	Client    rpcclient.Client
	GrpcConn  *grpc.ClientConn

	lock sync.Mutex

	// Ports set during startContainer.
	hostRPCPort  string
	hostAPIPort  string
	hostGRPCPort string
	hostP2PPort  string

	// Injected dependencies (previously from Config)
	chainID             string
	binaryName          string
	coinType            string
	gasPrices           string
	gasAdjustment       float64
	env                 []string
	additionalStartArgs []string
	encodingConfig      *testutil.TestEncodingConfig
	chainNodeConfig     *ChainNodeConfig
	zapLogger           *zap.Logger
	// For key preloading from celestia-app testnode
	genesisKeyring keyring.Keyring
	validatorIndex int
	// Private validator key bytes to overwrite after init
	privValidatorKey []byte
}

// ChainNodeParams contains all the parameters needed to create a ChainNode
type ChainNodeParams struct {
	Logger              *zap.Logger
	Validator           bool
	DockerClient        *dockerclient.Client
	DockerNetworkID     string
	TestName            string
	Image               DockerImage
	Index               int
	ChainID             string
	BinaryName          string
	CoinType            string
	GasPrices           string
	GasAdjustment       float64
	Env                 []string
	AdditionalStartArgs []string
	EncodingConfig      *testutil.TestEncodingConfig
	ChainNodeConfig     *ChainNodeConfig
	HomeDir             string
	// Optional fields for key preloading
	GenesisKeyring   keyring.Keyring
	ValidatorIndex   int
	PrivValidatorKey []byte
}

// NewChainNode creates a new ChainNode with injected dependencies
func NewChainNode(params ChainNodeParams) *ChainNode {
	nodeType := "fn"
	if params.Validator {
		nodeType = "val"
	}

	log := params.Logger.With(
		zap.Bool("validator", params.Validator),
		zap.Int("i", params.Index),
	)

	tn := &ChainNode{
		Validator:           params.Validator,
		chainID:             params.ChainID,
		binaryName:          params.BinaryName,
		coinType:            params.CoinType,
		gasPrices:           params.GasPrices,
		gasAdjustment:       params.GasAdjustment,
		env:                 params.Env,
		additionalStartArgs: params.AdditionalStartArgs,
		encodingConfig:      params.EncodingConfig,
		chainNodeConfig:     params.ChainNodeConfig,
		zapLogger:           params.Logger,
		genesisKeyring:      params.GenesisKeyring,
		validatorIndex:      params.ValidatorIndex,
		privValidatorKey:    params.PrivValidatorKey,
		node:                newNode(params.DockerNetworkID, params.DockerClient, params.TestName, params.Image, params.HomeDir, params.Index, nodeType, log),
	}

	tn.containerLifecycle = NewContainerLifecycle(params.Logger, params.DockerClient, tn.Name())

	return tn
}

func (tn *ChainNode) GetInternalHostName(ctx context.Context) (string, error) {
	return tn.HostName(), nil
}

func (tn *ChainNode) HostName() string {
	return CondenseHostName(tn.Name())
}

func (tn *ChainNode) GetType() string {
	return tn.NodeType()
}

func (tn *ChainNode) GetRPCClient() (rpcclient.Client, error) {
	return tn.Client, nil
}

// GetKeyring retrieves the keyring instance for the ChainNode. The keyring will be usable
// by the host running the test.
func (tn *ChainNode) GetKeyring() (keyring.Keyring, error) {
	containerKeyringDir := path.Join(tn.homeDir, "keyring-test")
	return dockerinternal.NewDockerKeyring(tn.DockerClient, tn.containerLifecycle.ContainerID(), containerKeyringDir, tn.encodingConfig.Codec), nil
}

// Name of the test node container.
func (tn *ChainNode) Name() string {
	return fmt.Sprintf("%s-%s-%d-%s", tn.chainID, tn.NodeType(), tn.Index, SanitizeContainerName(tn.TestName))
}

// NodeType returns the type of the ChainNode as a string: "fn" for full nodes and "val" for validator nodes.
func (tn *ChainNode) NodeType() string {
	nodeType := "fn"
	if tn.Validator {
		nodeType = "val"
	}
	return nodeType
}

// Height retrieves the latest block height of the chain node using the Tendermint RPC client.
func (tn *ChainNode) Height(ctx context.Context) (int64, error) {
	res, err := tn.Client.Status(ctx)
	if err != nil {
		return 0, fmt.Errorf("tendermint rpc client status: %w", err)
	}
	height := res.SyncInfo.LatestBlockHeight
	return height, nil
}

func (tn *ChainNode) initFullNodeFiles(ctx context.Context) error {
	if err := tn.initHomeFolder(ctx); err != nil {
		return err
	}

	return tn.setTestConfig(ctx)
}

// binCommand is a helper to retrieve a full command for a chain node binary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to return the full command.
// Will include additional flags for home directory and chain ID.
func (tn *ChainNode) binCommand(command ...string) []string {
	command = append([]string{tn.binaryName}, command...)
	return append(command,
		"--home", tn.homeDir,
	)
}

// execBin is a helper to execute a command for a chain node binary.
// For example, if chain node binary is `gaiad`, and desired command is `gaiad keys show key1`,
// pass ("keys", "show", "key1") for command to execute the command against the node.
// Will include additional flags for home directory and chain ID.
func (tn *ChainNode) execBin(ctx context.Context, command ...string) ([]byte, []byte, error) {
	return tn.exec(ctx, tn.logger(), tn.binCommand(command...), tn.env)
}

// initHomeFolder initializes a home folder for the given node.
func (tn *ChainNode) initHomeFolder(ctx context.Context) error {
	tn.lock.Lock()
	defer tn.lock.Unlock()

	_, _, err := tn.execBin(ctx,
		"init", CondenseMoniker(tn.Name()),
		"--chain-id", tn.chainID,
	)
	return err
}

// setTestConfig modifies the config to reasonable values for use within celestia-test.
func (tn *ChainNode) setTestConfig(ctx context.Context) error {
	c := make(toml.Toml)

	// Set Log Level to info
	c["log_level"] = "info"

	p2p := make(toml.Toml)

	// Allow p2p strangeness
	p2p["allow_duplicate_ip"] = true
	p2p["addr_book_strict"] = false

	c["p2p"] = p2p

	consensus := make(toml.Toml)

	blockT := (time.Duration(blockTime) * time.Second).String()
	consensus["timeout_commit"] = blockT
	consensus["timeout_propose"] = blockT

	c["consensus"] = consensus

	rpc := make(toml.Toml)

	// Enable public RPC
	rpc["laddr"] = "tcp://0.0.0.0:26657"
	rpc["allowed_origins"] = []string{"*"}
	c["rpc"] = rpc

	if err := ModifyConfigFile(
		ctx,
		tn.logger(),
		tn.DockerClient,
		tn.TestName,
		tn.VolumeName,
		"config/config.toml",
		c,
	); err != nil {
		return err
	}

	a := make(toml.Toml)
	a["minimum-gas-prices"] = tn.gasPrices

	grpc := make(toml.Toml)

	// Enable public GRPC
	grpc["address"] = "0.0.0.0:9090"

	a["grpc"] = grpc

	api := make(toml.Toml)

	// Enable public REST API
	api["enable"] = true
	api["swagger"] = true
	api["address"] = "tcp://0.0.0.0:1317"

	a["api"] = api

	return ModifyConfigFile(
		ctx,
		tn.logger(),
		tn.DockerClient,
		tn.TestName,
		tn.VolumeName,
		"config/app.toml",
		a,
	)
}

func (tn *ChainNode) logger() *zap.Logger {
	return tn.zapLogger.With(
		zap.String("chain_id", tn.chainID),
		zap.String("test", tn.TestName),
	)
}

// startContainer starts the container for the ChainNode, initializes its ports, and ensures the node is synced before returning.
// Returns an error if the container fails to start, ports cannot be set, or syncing is not completed within the timeout.
func (tn *ChainNode) startContainer(ctx context.Context) error {
	if err := tn.containerLifecycle.StartContainer(ctx); err != nil {
		return err
	}

	// Set the host ports once since they will not change after the container has started.
	hostPorts, err := tn.containerLifecycle.GetHostPorts(ctx, rpcPort, grpcPort, apiPort, p2pPort)
	if err != nil {
		return err
	}
	tn.hostRPCPort, tn.hostGRPCPort, tn.hostAPIPort, tn.hostP2PPort = hostPorts[0], hostPorts[1], hostPorts[2], hostPorts[3]

	err = tn.initClient("tcp://" + tn.hostRPCPort)
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
			stat, err := tn.Client.Status(ctx)
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
func (tn *ChainNode) initClient(addr string) error {
	httpClient, err := libclient.DefaultHTTPClient(addr)
	if err != nil {
		return err
	}

	httpClient.Timeout = 10 * time.Second
	rpcClient, err := rpchttp.NewWithClient(addr, "/websocket", httpClient)
	if err != nil {
		return err
	}

	tn.Client = rpcClient

	grpcConn, err := grpc.NewClient(
		tn.hostGRPCPort, grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("grpc dial: %w", err)
	}
	tn.GrpcConn = grpcConn

	return nil
}

// stop stops the underlying container.
func (tn *ChainNode) stop(ctx context.Context) error {
	return tn.containerLifecycle.StopContainer(ctx)
}

// setPeers modifies the config persistent_peers for a node.
func (tn *ChainNode) setPeers(ctx context.Context, peers string) error {
	c := make(toml.Toml)
	p2p := make(toml.Toml)

	// Set peers
	p2p["persistent_peers"] = peers
	c["p2p"] = p2p

	return ModifyConfigFile(
		ctx,
		tn.logger(),
		tn.DockerClient,
		tn.TestName,
		tn.VolumeName,
		"config/config.toml",
		c,
	)
}

// createNodeContainer initializes but does not start a container for the ChainNode with the specified configuration and context.
func (tn *ChainNode) createNodeContainer(ctx context.Context) error {
	cmd := []string{tn.binaryName, "start", "--home", tn.homeDir}
	if len(tn.additionalStartArgs) > 0 {
		cmd = append(cmd, tn.additionalStartArgs...)
	}
	usingPorts := nat.PortMap{}
	for k, v := range sentryPorts {
		usingPorts[k] = v
	}

	return tn.containerLifecycle.CreateContainer(ctx, tn.TestName, tn.NetworkID, tn.getImage(), usingPorts, "", tn.bind(), nil, tn.HostName(), cmd, tn.getEnv(), []string{})
}

func (tn *ChainNode) overwriteGenesisFile(ctx context.Context, content []byte) error {
	err := tn.WriteFile(ctx, "config/genesis.json", content)
	if err != nil {
		return fmt.Errorf("overwriting genesis.json: %w", err)
	}

	return nil
}

// overwritePrivValidatorKey overwrites the private validator key after init
func (tn *ChainNode) overwritePrivValidatorKey(ctx context.Context) error {
	if tn.privValidatorKey == nil {
		return nil // Skip if no private validator key provided
	}

	tn.logger().Info("overwriting private validator key after init",
		zap.Int("key_size", len(tn.privValidatorKey)),
	)

	err := tn.WriteFile(ctx, "config/priv_validator_key.json", tn.privValidatorKey)
	if err != nil {
		return fmt.Errorf("overwriting priv_validator_key.json: %w", err)
	}

	tn.logger().Info("successfully overwrote private validator key")
	return nil
}

// collectGentxs runs collect gentxs on the node's home folders.
func (tn *ChainNode) collectGentxs(ctx context.Context) error {
	tn.lock.Lock()
	defer tn.lock.Unlock()

	command := []string{tn.binaryName, "genesis", "collect-gentxs", "--home", tn.homeDir}

	_, _, err := tn.exec(ctx, tn.logger(), command, tn.env)
	return err
}

func (tn *ChainNode) genesisFileContent(ctx context.Context) ([]byte, error) {
	gen, err := tn.ReadFile(ctx, "config/genesis.json")
	if err != nil {
		return nil, fmt.Errorf("getting genesis.json content: %w", err)
	}

	return gen, nil
}

func (tn *ChainNode) copyGentx(ctx context.Context, destVal *ChainNode) error {
	nid, err := tn.nodeID(ctx)
	if err != nil {
		return fmt.Errorf("getting node ID: %w", err)
	}

	relPath := fmt.Sprintf("config/gentx/gentx-%s.json", nid)

	gentx, err := tn.ReadFile(ctx, relPath)
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
func (tn *ChainNode) nodeID(ctx context.Context) (string, error) {
	// This used to call p2p.LoadNodeKey against the file on the host,
	// but because we are transitioning to operating on Docker volumes,
	// we only have to tmjson.Unmarshal the raw content.
	j, err := tn.ReadFile(ctx, "config/node_key.json")
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
func (tn *ChainNode) addGenesisAccount(ctx context.Context, address string, genesisAmount []sdk.Coin) error {
	tn.lock.Lock()
	defer tn.lock.Unlock()

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
	_, _, err := tn.execBin(ctx, command...)

	return err
}

// initValidatorGenTx creates the node files and signs a genesis transaction.
func (tn *ChainNode) initValidatorGenTx(
	ctx context.Context,
	genesisAmounts []sdk.Coin,
	genesisSelfDelegation sdk.Coin,
) error {
	if err := tn.createKey(ctx, valKey); err != nil {
		return err
	}
	bech32, err := tn.accountKeyBech32(ctx, valKey)
	if err != nil {
		return err
	}
	if err := tn.addGenesisAccount(ctx, bech32, genesisAmounts); err != nil {
		return err
	}
	return tn.gentx(ctx, valKey, genesisSelfDelegation)
}

// createKey creates a key in the keyring backend test for the given node.
func (tn *ChainNode) createKey(ctx context.Context, name string) error {
	tn.lock.Lock()
	defer tn.lock.Unlock()

	_, _, err := tn.execBin(ctx,
		"keys", "add", name,
		"--coin-type", tn.coinType,
		"--keyring-backend", keyring.BackendTest,
	)
	return err
}

// Gentx generates the gentx for a given node.
func (tn *ChainNode) gentx(ctx context.Context, name string, genesisSelfDelegation sdk.Coin) error {
	tn.lock.Lock()
	defer tn.lock.Unlock()

	command := []string{"genesis", "gentx", name, fmt.Sprintf("%s%s", genesisSelfDelegation.Amount.String(), genesisSelfDelegation.Denom),
		"--gas-prices", tn.gasPrices,
		"--gas-adjustment", fmt.Sprint(tn.gasAdjustment),
		"--keyring-backend", keyring.BackendTest,
		"--chain-id", tn.chainID,
	}

	_, _, err := tn.execBin(ctx, command...)
	return err
}

// accountKeyBech32 retrieves the named key's address in bech32 account format.
func (tn *ChainNode) accountKeyBech32(ctx context.Context, name string) (string, error) {
	return tn.keyBech32(ctx, name, "")
}

// CliContext creates a new Cosmos SDK client context.
func (tn *ChainNode) CliContext() client.Context {
	return client.Context{
		Client:            tn.Client,
		GRPCClient:        tn.GrpcConn,
		ChainID:           tn.chainID,
		InterfaceRegistry: tn.encodingConfig.InterfaceRegistry,
		Input:             os.Stdin,
		Output:            os.Stdout,
		OutputFormat:      "json",
		LegacyAmino:       tn.encodingConfig.Amino,
		TxConfig:          tn.encodingConfig.TxConfig,
	}
}

// keyBech32 retrieves the named key's address in bech32 format from the node.
// bech is the bech32 prefix (acc|val|cons). If empty, defaults to the account key (same as "acc").
func (tn *ChainNode) keyBech32(ctx context.Context, name string, bech string) (string, error) {
	command := []string{
		tn.binaryName, "keys", "show", "--address", name,
		"--home", tn.homeDir,
		"--keyring-backend", keyring.BackendTest,
	}

	if bech != "" {
		command = append(command, "--bech", bech)
	}

	stdout, stderr, err := tn.exec(ctx, tn.logger(), command, tn.env)
	if err != nil {
		return "", fmt.Errorf("failed to show key %q (stderr=%q): %w", name, stderr, err)
	}

	return string(bytes.TrimSuffix(stdout, []byte("\n"))), nil
}

// getNodeConfig returns the per-node configuration if it exists
func (tn *ChainNode) getNodeConfig() *ChainNodeConfig {
	return tn.chainNodeConfig
}

// getAdditionalStartArgs returns the start arguments for this node, preferring per-node config over chain config
func (tn *ChainNode) getAdditionalStartArgs() []string {
	if tn.chainNodeConfig != nil && tn.chainNodeConfig.AdditionalStartArgs != nil {
		return tn.chainNodeConfig.AdditionalStartArgs
	}
	return tn.additionalStartArgs
}

// getImage returns the Docker image for this node, preferring per-node config over the default image
func (tn *ChainNode) getImage() DockerImage {
	if tn.chainNodeConfig != nil && tn.chainNodeConfig.Image != nil {
		return *tn.chainNodeConfig.Image
	}
	return tn.Image
}

// getEnv returns the environment variables for this node, preferring per-node config over chain config
func (tn *ChainNode) getEnv() []string {
	if tn.chainNodeConfig != nil && tn.chainNodeConfig.Env != nil {
		return tn.chainNodeConfig.Env
	}
	return tn.env
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
