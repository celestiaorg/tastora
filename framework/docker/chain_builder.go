package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"os"
	"path"
	"testing"
)

type ChainNodeConfig struct {
	// nodeType specifies which type of node should be built.
	nodeType NodeType
	// Image overrides the chain's default image for this specific node (optional)
	Image *container.Image
	// AdditionalStartArgs overrides the chain-level AdditionalStartArgs for this specific node
	AdditionalStartArgs []string
	// Env overrides the chain-level Env for this specific node
	Env []string
	// privValidatorKey contains the private validator key bytes for this specific node
	privValidatorKey []byte
	// postInit functions are executed sequentially after the node is initialized.
	postInit []func(ctx context.Context, node *ChainNode) error
	// keyring specifies the keyring backend used for managing keys and signing transactions.
	keyring keyring.Keyring
	// accountName specifies the name of the account/key in the genesis keyring to use for this validator
	accountName string
}

// ChainNodeConfigBuilder provides a fluent interface for building ChainNodeConfig
type ChainNodeConfigBuilder struct {
	config *ChainNodeConfig
}

// NewChainNodeConfigBuilder creates a new ChainNodeConfigBuilder
func NewChainNodeConfigBuilder() *ChainNodeConfigBuilder {
	return &ChainNodeConfigBuilder{
		config: &ChainNodeConfig{
			nodeType:            ValidatorNodeType,
			AdditionalStartArgs: make([]string, 0),
			Env:                 make([]string, 0),
		},
	}
}

func (b *ChainNodeConfigBuilder) WithKeyring(kr keyring.Keyring) *ChainNodeConfigBuilder {
	b.config.keyring = kr
	return b
}

// WithImage sets the Docker image for the node (overrides chain default)
func (b *ChainNodeConfigBuilder) WithImage(image container.Image) *ChainNodeConfigBuilder {
	b.config.Image = &image
	return b
}

// WithAdditionalStartArgs sets the additional start arguments
func (b *ChainNodeConfigBuilder) WithAdditionalStartArgs(args ...string) *ChainNodeConfigBuilder {
	b.config.AdditionalStartArgs = args
	return b
}

// WithEnvVars sets the environment variables
func (b *ChainNodeConfigBuilder) WithEnvVars(envVars ...string) *ChainNodeConfigBuilder {
	b.config.Env = envVars
	return b
}

// WithPrivValidatorKey sets the private validator key bytes for this node
func (b *ChainNodeConfigBuilder) WithPrivValidatorKey(privValKey []byte) *ChainNodeConfigBuilder {
	b.config.privValidatorKey = privValKey
	return b
}

// WithPostInit sets the post init functions.
func (b *ChainNodeConfigBuilder) WithPostInit(postInitFns ...func(ctx context.Context, node *ChainNode) error) *ChainNodeConfigBuilder {
	b.config.postInit = postInitFns
	return b
}

// WithAccountName sets the account name to use from the genesis keyring for this validator
func (b *ChainNodeConfigBuilder) WithAccountName(accountName string) *ChainNodeConfigBuilder {
	b.config.accountName = accountName
	return b
}

// WithNodeType sets the type of blockchain node to be configured and returns the updated ChainNodeConfigBuilder.
func (b *ChainNodeConfigBuilder) WithNodeType(nodeType NodeType) *ChainNodeConfigBuilder {
	b.config.nodeType = nodeType
	return b
}

// Build returns the configured ChainNodeConfig
func (b *ChainNodeConfigBuilder) Build() ChainNodeConfig {
	return *b.config
}

// ChainBuilder defines a builder for configuring and initializing a blockchain for testing purposes.
type ChainBuilder struct {
	// t is the testing context used for test assertions, container naming, and test lifecycle management
	t *testing.T
	// nodes is the array of node configurations that define the chain topology and individual node settings
	nodes []ChainNodeConfig
	// dockerClient is the Docker client instance used for all container operations (create, start, stop, etc.)
	dockerClient *client.Client
	// dockerNetworkID is the ID of the Docker network where all chain nodes are deployed
	dockerNetworkID string
	// genesisBz contains raw bytes that should be written as the config/genesis.json file for the chain (optional)
	genesisBz []byte
	// encodingConfig is the Cosmos SDK encoding configuration for protobuf and amino serialization/deserialization
	encodingConfig *testutil.TestEncodingConfig
	// binaryName is the name of the blockchain binary executable. Default: "celestia-appd"
	binaryName string
	// coinType is the BIP-44 coin type used for key derivation. Default: "118"
	coinType string
	// gasPrices is the gas price configuration for transactions. Default: "0.025utia"
	gasPrices string
	// gasAdjustment is the multiplier for gas estimation to prevent out-of-gas errors. Default: 1.3 (30% buffer)
	gasAdjustment float64
	// bech32Prefix is the address prefix for the blockchain. Default: "celestia"
	bech32Prefix string
	// denom is the native token denomination used in transactions and fees. Default: "utia"
	denom string
	// name is the chain name identifier used in container naming and home directory paths. Default: "celestia"
	name string
	// chainID is the blockchain chain ID for network identification (e.g., "test"). Default: "test"
	chainID string
	// logger is the structured logger for chain operations and debugging. Defaults to test logger.
	logger *zap.Logger
	// dockerImage is the default Docker image configuration for all nodes in the chain (can be overridden per node)
	dockerImage *container.Image
	// additionalStartArgs are the default additional command-line arguments for all nodes in the chain (can be overridden per node)
	additionalStartArgs []string
	// postInit are the default post-initialization functions for all nodes in the chain (can be overridden per node)
	postInit []func(ctx context.Context, node *ChainNode) error
	// env are the default environment variables for all nodes in the chain (can be overridden per node)
	env []string
	// faucetWallet is the wallet that should be used when broadcasting transactions when the sender doesn't matter
	// and the outcome is the only thing important. E.g. funding relayer wallets.
	faucetWallet Wallet
}

// NewChainBuilder initializes and returns a new ChainBuilder with default values for testing purposes.
func NewChainBuilder(t *testing.T) *ChainBuilder {
	t.Helper()
	cb := &ChainBuilder{}
	return cb.
		WithT(t).
		WithBinaryName("celestia-appd").
		WithCoinType("118").
		WithGasPrices("0.025utia").
		WithGasAdjustment(1.3).
		WithBech32Prefix("celestia").
		WithDenom("utia").
		WithChainID("test").
		WithLogger(zaptest.NewLogger(t)).
		WithName("celestia")
}

// NewChainBuilderFromChain initializes and returns a new ChainBuilder that copies the values from the given chain.
func NewChainBuilderFromChain(chain *Chain) *ChainBuilder {
	cfg := chain.cfg.ChainConfig
	return NewChainBuilder(chain.t).
		WithLogger(chain.log).
		WithEncodingConfig(cfg.EncodingConfig).
		WithName(cfg.Name).
		WithChainID(cfg.ChainID).
		WithBinaryName(cfg.Bin).
		WithCoinType(cfg.CoinType).
		WithGasPrices(cfg.GasPrices).
		WithGasAdjustment(cfg.GasAdjustment).
		WithBech32Prefix(cfg.Bech32Prefix).
		WithDenom(cfg.Denom).
		WithGenesis(cfg.GenesisFileBz).
		WithImage(cfg.Image).
		WithDockerClient(chain.cfg.DockerClient).
		WithDockerNetworkID(chain.cfg.DockerNetworkID).
		WithAdditionalStartArgs(cfg.AdditionalStartArgs...).
		WithEnv(cfg.Env...)
}

func (b *ChainBuilder) WithName(name string) *ChainBuilder {
	b.name = name
	return b
}

func (b *ChainBuilder) WithFaucetWallet(wallet Wallet) *ChainBuilder {
	b.faucetWallet = wallet
	return b
}

// WithChainID sets the chain ID
func (b *ChainBuilder) WithChainID(chainID string) *ChainBuilder {
	b.chainID = chainID
	return b
}

func (b *ChainBuilder) WithT(t *testing.T) *ChainBuilder {
	t.Helper()
	b.t = t
	return b
}

// WithLogger sets the logger.
func (b *ChainBuilder) WithLogger(logger *zap.Logger) *ChainBuilder {
	b.logger = logger
	return b
}

// WithNode adds a node of the specified type to the chain
func (b *ChainBuilder) WithNode(config ChainNodeConfig) *ChainBuilder {
	b.nodes = append(b.nodes, config)
	return b
}

// WithNodes adds multiple node configurations
func (b *ChainBuilder) WithNodes(nodeConfigs ...ChainNodeConfig) *ChainBuilder {
	b.nodes = nodeConfigs
	return b
}

// WithDockerClient sets the Docker client
func (b *ChainBuilder) WithDockerClient(client *client.Client) *ChainBuilder {
	b.dockerClient = client
	return b
}

// WithDockerNetworkID sets the Docker network ID
func (b *ChainBuilder) WithDockerNetworkID(networkID string) *ChainBuilder {
	b.dockerNetworkID = networkID
	return b
}

// WithGenesis sets the raw genesis bytes
func (b *ChainBuilder) WithGenesis(genesisBz []byte) *ChainBuilder {
	b.genesisBz = genesisBz
	return b
}

// WithEncodingConfig sets the encoding configuration
func (b *ChainBuilder) WithEncodingConfig(config *testutil.TestEncodingConfig) *ChainBuilder {
	b.encodingConfig = config
	return b
}

// WithBinaryName sets the binary name
func (b *ChainBuilder) WithBinaryName(name string) *ChainBuilder {
	b.binaryName = name
	return b
}

// WithCoinType sets the coin type
func (b *ChainBuilder) WithCoinType(coinType string) *ChainBuilder {
	b.coinType = coinType
	return b
}

// WithGasPrices sets the gas prices
func (b *ChainBuilder) WithGasPrices(gasPrices string) *ChainBuilder {
	b.gasPrices = gasPrices
	return b
}

// WithGasAdjustment sets the gas adjustment.
func (b *ChainBuilder) WithGasAdjustment(gasAdjustment float64) *ChainBuilder {
	b.gasAdjustment = gasAdjustment
	return b
}

// WithBech32Prefix sets the bech32 prefix
func (b *ChainBuilder) WithBech32Prefix(bech32Prefix string) *ChainBuilder {
	b.bech32Prefix = bech32Prefix
	return b
}

// WithDenom sets the denomination
func (b *ChainBuilder) WithDenom(denom string) *ChainBuilder {
	b.denom = denom
	return b
}

// WithImage sets the default Docker image for all nodes in the chain
func (b *ChainBuilder) WithImage(image container.Image) *ChainBuilder {
	b.dockerImage = &image
	return b
}

// WithAdditionalStartArgs sets the default additional start arguments for all nodes in the chain
func (b *ChainBuilder) WithAdditionalStartArgs(args ...string) *ChainBuilder {
	b.additionalStartArgs = args
	return b
}

// WithPostInit sets the default post init functions for all nodes in the chain
func (b *ChainBuilder) WithPostInit(postInitFns ...func(ctx context.Context, node *ChainNode) error) *ChainBuilder {
	b.postInit = postInitFns
	return b
}

// WithEnv sets the default environment variables for all nodes in the chain
func (b *ChainBuilder) WithEnv(env ...string) *ChainBuilder {
	b.env = env
	return b
}

// getImage returns the appropriate Docker image for a node, using node-specific override if available,
// otherwise falling back to the chain's default image
func (b *ChainBuilder) getImage(nodeConfig ChainNodeConfig) container.Image {
	if nodeConfig.Image != nil {
		// Use node-specific image override
		return *nodeConfig.Image
	}
	if b.dockerImage != nil {
		// Use chain default image
		return *b.dockerImage
	}
	// this should not happen if the builder is used correctly
	panic("no image specified: neither node-specific nor chain default image provided")
}

// getAdditionalStartArgs returns the appropriate additional start arguments for a node, using node-specific override if available,
// otherwise falling back to the chain's default additional start arguments
func (b *ChainBuilder) getAdditionalStartArgs(nodeConfig ChainNodeConfig) []string {
	if len(nodeConfig.AdditionalStartArgs) > 0 {
		// use node-specific additional start args override
		return nodeConfig.AdditionalStartArgs
	}
	// use chain default additional start args (may be empty)
	return b.additionalStartArgs
}

// getPostInit returns the appropriate post init functions for a node, using node-specific override if available,
// otherwise falling back to the chain's default post init functions
func (b *ChainBuilder) getPostInit(nodeConfig ChainNodeConfig) []func(ctx context.Context, node *ChainNode) error {
	if len(nodeConfig.postInit) > 0 {
		// use node-specific post init override
		return nodeConfig.postInit
	}
	// use chain default post init functions (may be empty)
	return b.postInit
}

// getEnv returns the appropriate environment variables for a node, using node-specific override if available,
// otherwise falling back to the chain's default environment variables
func (b *ChainBuilder) getEnv(nodeConfig ChainNodeConfig) []string {
	if len(nodeConfig.Env) > 0 {
		// use node-specific env override
		return nodeConfig.Env
	}
	// use chain default env variables (may be empty)
	return b.env
}

func (b *ChainBuilder) Build(ctx context.Context) (*Chain, error) {
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	nodes, err := b.initializeChainNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chain nodes: %w", err)
	}

	// separate validators and full nodes
	var validators, fullNodes []*ChainNode
	for _, node := range nodes {
		if node.Validator {
			validators = append(validators, node)
		} else {
			fullNodes = append(fullNodes, node)
		}
	}

	chain := &Chain{
		cfg: Config{
			Logger:          b.logger,
			DockerClient:    b.dockerClient,
			DockerNetworkID: b.dockerNetworkID,
			ChainConfig: &ChainConfig{
				Name:                b.name,
				ChainID:             b.chainID,
				Image:               *b.dockerImage, // default image must be provided, can be overridden per node.
				Bin:                 b.binaryName,
				Bech32Prefix:        b.bech32Prefix,
				Denom:               b.denom,
				CoinType:            b.coinType,
				GasPrices:           b.gasPrices,
				GasAdjustment:       b.gasAdjustment,
				PostInit:            b.postInit,
				EncodingConfig:      b.encodingConfig,
				AdditionalStartArgs: b.additionalStartArgs,
				Env:                 b.env,
				GenesisFileBz:       b.genesisBz,
			},
		},
		t:            b.t,
		Validators:   validators,
		FullNodes:    fullNodes,
		cdc:          cdc,
		log:          b.logger,
		faucetWallet: &b.faucetWallet,
	}

	return chain, nil
}

func (b *ChainBuilder) initializeChainNodes(ctx context.Context) ([]*ChainNode, error) {
	var nodes []*ChainNode

	for i, nodeConfig := range b.nodes {
		n, err := b.newChainNode(ctx, nodeConfig, i)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}

	return nodes, nil
}

// newChainNode constructs a new cosmos chain node with a docker volume.
func (b *ChainBuilder) newChainNode(
	ctx context.Context,
	nodeConfig ChainNodeConfig,
	index int,
) (*ChainNode, error) {
	// Construct the ChainNode first so we can access its name.
	// The ChainNode's VolumeName cannot be set until after we create the volume.
	tn := b.newDockerChainNode(b.logger, nodeConfig, index)

	// create and setup volume using shared logic
	if err := tn.CreateAndSetupVolume(ctx, tn.Name()); err != nil {
		return nil, err
	}

	// if this is a validator and we have a genesis keyring, preload the keys using a one-shot container
	if nodeConfig.nodeType == ValidatorNodeType && tn.GenesisKeyring != nil {
		if err := preloadKeyringToVolume(ctx, tn, nodeConfig); err != nil {
			return nil, fmt.Errorf("failed to preload keyring to volume: %w", err)
		}
	}

	return tn, nil
}

func (b *ChainBuilder) newDockerChainNode(log *zap.Logger, nodeConfig ChainNodeConfig, index int) *ChainNode {
	// use a default home directory if name is not set
	homeDir := "/var/cosmos-chain"
	if b.name != "" {
		homeDir = path.Join("/var/cosmos-chain", b.name)
	}

	chainParams := ChainNodeParams{
		Validator:           nodeConfig.nodeType == ValidatorNodeType,
		ChainID:             b.chainID,
		BinaryName:          b.binaryName,
		CoinType:            b.coinType,
		GasPrices:           b.gasPrices,
		GasAdjustment:       b.gasAdjustment,
		Env:                 b.getEnv(nodeConfig),
		AdditionalStartArgs: b.getAdditionalStartArgs(nodeConfig),
		EncodingConfig:      b.encodingConfig,
		GenesisKeyring:      nodeConfig.keyring,
		ValidatorIndex:      index,
		PrivValidatorKey:    nodeConfig.privValidatorKey,
		PostInit:            b.getPostInit(nodeConfig),
	}

	// Get the appropriate image using fallback logic
	imageToUse := b.getImage(nodeConfig)

	return NewChainNode(log, b.dockerClient, b.dockerNetworkID, b.t.Name(), imageToUse, homeDir, index, chainParams)
}

// preloadKeyringToVolume copies validator keys from genesis keyring to the node's volume
func preloadKeyringToVolume(ctx context.Context, node *ChainNode, nodeConfig ChainNodeConfig) error {
	// check if AccountName is specified
	if nodeConfig.accountName == "" {
		return fmt.Errorf("accountName must be specified for validator nodes when using a genesis keyring")
	}

	validatorKeyName := nodeConfig.accountName

	// get the key from the genesis keyring
	_, err := node.GenesisKeyring.Key(validatorKeyName)
	if err != nil {
		return fmt.Errorf("validator key %q not found in genesis keyring: %w", validatorKeyName, err)
	}

	// create a temporary directory to hold the keyring files locally.
	tempDir, err := os.MkdirTemp("", "keyring-export-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	// create a temporary keyring in the temp directory
	tempKeyring, err := keyring.New("test", keyring.BackendTest, tempDir, nil, node.EncodingConfig.Codec)
	if err != nil {
		return fmt.Errorf("failed to create temp keyring: %w", err)
	}

	// export the key from genesis keyring
	armor, err := node.GenesisKeyring.ExportPrivKeyArmor(validatorKeyName, "")
	if err != nil {
		return fmt.Errorf("failed to export validator key: %w", err)
	}

	// import the key into the temp keyring
	err = tempKeyring.ImportPrivKey(validatorKeyName, armor, "")
	if err != nil {
		return fmt.Errorf("failed to import key into temp keyring: %w", err)
	}

	// copy keyring files to the volume.
	return copyKeyringFilesToVolume(ctx, node, tempDir)
}

// copyKeyringFilesToVolume copies keyring files from host temp directory to container volume
func copyKeyringFilesToVolume(ctx context.Context, node *ChainNode, hostKeyringDir string) error {
	// The cosmos keyring creates files in a keyring-test subdirectory
	keyringSubDir := path.Join(hostKeyringDir, "keyring-test")

	// list files in the keyring subdirectory
	files, err := os.ReadDir(keyringSubDir)
	if err != nil {
		return fmt.Errorf("failed to read keyring directory: %w", err)
	}

	// Copy each keyring file to the volume
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		hostFilePath := path.Join(keyringSubDir, file.Name())
		content, err := os.ReadFile(hostFilePath)
		if err != nil {
			return fmt.Errorf("failed to read keyring file %s: %w", file.Name(), err)
		}

		relativePath := path.Join("keyring-test", file.Name())
		err = node.WriteFile(ctx, relativePath, content)
		if err != nil {
			return fmt.Errorf("failed to write keyring file %s to volume: %w", file.Name(), err)
		}
	}
	return nil
}
