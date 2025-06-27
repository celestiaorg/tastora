package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"os"
	"path"
	"testing"
)

type ChainNodeConfig struct {
	// validator is a flag indicating whether the node is a validator or a full node.
	validator bool
	// Image overrides the chain's default image for this specific node (optional)
	Image *DockerImage
	// AdditionalStartArgs overrides the chain-level AdditionalStartArgs for this specific node
	AdditionalStartArgs []string
	// Env overrides the chain-level Env for this specific node
	Env []string
	// privValidatorKey contains the private validator key bytes for this specific node
	privValidatorKey []byte
	// postInit functions are executed sequentially after the node is initialized.
	postInit []func(ctx context.Context, node *ChainNode) error
}

// ChainNodeConfigBuilder provides a fluent interface for building ChainNodeConfig
type ChainNodeConfigBuilder struct {
	config *ChainNodeConfig
}

// NewChainNodeConfigBuilder creates a new ChainNodeConfigBuilder
func NewChainNodeConfigBuilder() *ChainNodeConfigBuilder {
	return &ChainNodeConfigBuilder{
		config: &ChainNodeConfig{
			validator:           false,
			AdditionalStartArgs: make([]string, 0),
			Env:                 make([]string, 0),
		},
	}
}

// WithImage sets the Docker image for the node (overrides chain default)
func (b *ChainNodeConfigBuilder) WithImage(image DockerImage) *ChainNodeConfigBuilder {
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

// Build returns the configured ChainNodeConfig
func (b *ChainNodeConfigBuilder) Build() ChainNodeConfig {
	return *b.config
}

type ChainBuilder struct {
	t               *testing.T
	validators      []ChainNodeConfig
	fullNodes       []ChainNodeConfig
	dockerClient    *client.Client
	dockerNetworkID string
	// raw bytes that should be written as the config/genesis.json file for the chain.
	genesisBz      []byte
	encodingConfig *testutil.TestEncodingConfig
	binaryName     string
	coinType       string
	gasPrices      string
	name           string
	chainID        string
	logger         *zap.Logger
	// optional keyring with pre-generated keys (e.g., from celestia-app testnode)
	genesisKeyring keyring.Keyring
	// default Docker image for all nodes in the chain (can be overridden per node)
	defaultImage *DockerImage
	// default additional start arguments for all nodes in the chain (can be overridden per node)
	defaultAdditionalStartArgs []string
	// default post init functions for all nodes in the chain (can be overridden per node)
	defaultPostInit []func(ctx context.Context, node *ChainNode) error
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
		WithChainID("test").
		WithLogger(zaptest.NewLogger(t)).
		WithName("celestia")
}

func (b *ChainBuilder) WithName(name string) *ChainBuilder {
	b.name = name
	return b
}

// WithChainID sets the chain ID
func (b *ChainBuilder) WithChainID(chainID string) *ChainBuilder {
	b.chainID = chainID
	return b
}

// WithGenesisKeyring sets a keyring containing keys that match the genesis
// This is useful when using celestia-app's testnode package which pre-generates keys
func (b *ChainBuilder) WithGenesisKeyring(kr keyring.Keyring) *ChainBuilder {
	b.genesisKeyring = kr
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

// WithValidators sets the validator node configurations
func (b *ChainBuilder) WithValidators(validators ...ChainNodeConfig) *ChainBuilder {
	b.validators = make([]ChainNodeConfig, 0, len(validators))
	for _, validator := range validators {
		validator.validator = true
		b.validators = append(b.validators, validator)
	}
	return b
}

// WithFullNodes sets the full node configurations
func (b *ChainBuilder) WithFullNodes(fullNodes ...ChainNodeConfig) *ChainBuilder {
	b.fullNodes = fullNodes
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

// WithDefaultImage sets the default Docker image for all nodes in the chain
func (b *ChainBuilder) WithDefaultImage(image DockerImage) *ChainBuilder {
	b.defaultImage = &image
	return b
}

// WithDefaultAdditionalStartArgs sets the default additional start arguments for all nodes in the chain
func (b *ChainBuilder) WithDefaultAdditionalStartArgs(args ...string) *ChainBuilder {
	b.defaultAdditionalStartArgs = args
	return b
}

// WithDefaultPostInit sets the default post init functions for all nodes in the chain
func (b *ChainBuilder) WithDefaultPostInit(postInitFns ...func(ctx context.Context, node *ChainNode) error) *ChainBuilder {
	b.defaultPostInit = postInitFns
	return b
}

// getImage returns the appropriate Docker image for a node, using node-specific override if available,
// otherwise falling back to the chain's default image
func (b *ChainBuilder) getImage(nodeConfig ChainNodeConfig) DockerImage {
	if nodeConfig.Image != nil {
		// Use node-specific image override
		return *nodeConfig.Image
	}
	if b.defaultImage != nil {
		// Use chain default image
		return *b.defaultImage
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
	return b.defaultAdditionalStartArgs
}

// getPostInit returns the appropriate post init functions for a node, using node-specific override if available,
// otherwise falling back to the chain's default post init functions
func (b *ChainBuilder) getPostInit(nodeConfig ChainNodeConfig) []func(ctx context.Context, node *ChainNode) error {
	if len(nodeConfig.postInit) > 0 {
		// use node-specific post init override
		return nodeConfig.postInit
	}
	// use chain default post init functions (may be empty)
	return b.defaultPostInit
}

// AddValidator adds a single validator node configuration
func (b *ChainBuilder) AddValidator(validator ChainNodeConfig) *ChainBuilder {
	validator.validator = true
	b.validators = append(b.validators, validator)
	return b
}

// AddFullNode adds a single full node configuration
func (b *ChainBuilder) AddFullNode(fullNode ChainNodeConfig) *ChainBuilder {
	fullNode.validator = false
	b.fullNodes = append(b.fullNodes, fullNode)
	return b
}

func (b *ChainBuilder) Build(ctx context.Context) (*Chain, error) {
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	nodes, err := b.initializeChainNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize chain nodes: %w", err)
	}

	chain := &Chain{
		cfg: Config{
			Logger:          b.logger,
			DockerClient:    b.dockerClient,
			DockerNetworkID: b.dockerNetworkID,
			ChainConfig: &ChainConfig{
				Type:           "cosmos",
				Name:           "celestia",
				Version:        "v4.0.0-rc6",
				ChainID:        b.chainID,
				Image:          *b.defaultImage, // default image must be provided, can be overridden per node.
				Bin:            b.binaryName,
				Bech32Prefix:   "celestia",
				Denom:          "utia",
				CoinType:       b.coinType,
				GasPrices:      b.gasPrices,
				GasAdjustment:  1.3,
				EncodingConfig: b.encodingConfig,
				GenesisFileBz:  b.genesisBz,
			},
		},
		t:          b.t,
		Validators: nodes,
		cdc:        cdc,
		log:        b.logger,
	}

	return chain, nil
}

func (b *ChainBuilder) initializeChainNodes(ctx context.Context) ([]*ChainNode, error) {
	var nodes []*ChainNode
	for i, val := range b.validators {
		n, err := b.newChainNode(ctx, val, i)
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

	v, err := b.dockerClient.VolumeCreate(ctx, volumetypes.CreateOptions{
		Labels: map[string]string{
			consts.CleanupLabel:   b.t.Name(),
			consts.NodeOwnerLabel: tn.Name(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume for chain node: %w", err)
	}
	tn.VolumeName = v.Name

	// Get the appropriate image using fallback logic
	imageToUse := b.getImage(nodeConfig)

	if err := SetVolumeOwner(ctx, VolumeOwnerOptions{
		Log:        b.logger,
		Client:     b.dockerClient,
		VolumeName: v.Name,
		ImageRef:   imageToUse.Ref(),
		TestName:   b.t.Name(),
		UidGid:     imageToUse.UIDGID,
	}); err != nil {
		return nil, fmt.Errorf("set volume owner: %w", err)
	}

	// If this is a validator and we have a genesis keyring, preload the keys using a one-shot container
	if nodeConfig.validator && b.genesisKeyring != nil {
		if err := b.preloadKeyringToVolume(ctx, tn, index); err != nil {
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
		Validator:           nodeConfig.validator,
		ChainID:             b.chainID,
		BinaryName:          b.binaryName,
		CoinType:            b.coinType,
		GasPrices:           b.gasPrices,
		GasAdjustment:       1.0, // Default gas adjustment
		Env:                 nodeConfig.Env,
		AdditionalStartArgs: b.getAdditionalStartArgs(nodeConfig),
		EncodingConfig:      b.encodingConfig,
		GenesisKeyring:      nil, // Will be set below if validator
		ValidatorIndex:      index,
		PrivValidatorKey:    nodeConfig.privValidatorKey, // Set from node config
		PostInit:            b.getPostInit(nodeConfig),
	}

	// Set genesis keyring if this is a validator
	if nodeConfig.validator && b.genesisKeyring != nil {
		chainParams.GenesisKeyring = b.genesisKeyring
	}

	// Get the appropriate image using fallback logic
	imageToUse := b.getImage(nodeConfig)

	return NewChainNode(log, b.dockerClient, b.dockerNetworkID, b.t.Name(), imageToUse, homeDir, index, chainParams)
}

// preloadKeyringToVolume copies validator keys from genesis keyring to the node's volume
func (b *ChainBuilder) preloadKeyringToVolume(ctx context.Context, node *ChainNode, validatorIndex int) error {
	// For celestia-app testnode, the default validator is named "validator"
	validatorKeyName := "validator"
	if validatorIndex > 0 {
		// If there are multiple validators, they might be named differently
		validatorKeyName = fmt.Sprintf("validator-%d", validatorIndex)
	}

	// Check if the key exists in the genesis keyring
	key, err := b.genesisKeyring.Key(validatorKeyName)
	if err != nil {
		// Try just "validator" as fallback
		validatorKeyName = "validator"
		key, err = b.genesisKeyring.Key(validatorKeyName)
		if err != nil {
			return fmt.Errorf("validator key %q not found in genesis keyring: %w", validatorKeyName, err)
		}
	}

	// Log the key details for debugging
	pubKey, _ := key.GetPubKey()
	if pubKey != nil {
		b.logger.Info("found validator key in genesis keyring",
			zap.String("key_name", validatorKeyName),
			zap.String("pubkey_type", fmt.Sprintf("%T", pubKey)),
			zap.String("address", sdk.AccAddress(pubKey.Address()).String()),
		)
	}

	// Create a temporary directory to hold the keyring files
	tempDir, err := os.MkdirTemp("", "keyring-export-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a temporary keyring in the temp directory
	tempKeyring, err := keyring.New("test", keyring.BackendTest, tempDir, nil, b.encodingConfig.Codec)
	if err != nil {
		return fmt.Errorf("failed to create temp keyring: %w", err)
	}

	// Export the key from genesis keyring
	armor, err := b.genesisKeyring.ExportPrivKeyArmor(validatorKeyName, "")
	if err != nil {
		return fmt.Errorf("failed to export validator key: %w", err)
	}

	// Import the key into the temp keyring
	err = tempKeyring.ImportPrivKey(validatorKeyName, armor, "")
	if err != nil {
		return fmt.Errorf("failed to import key into temp keyring: %w", err)
	}

	// Copy keyring files to the volume using existing file utilities
	return b.copyKeyringFilesToVolume(ctx, node, tempDir)
}

// copyKeyringFilesToVolume copies keyring files from host temp directory to container volume
func (b *ChainBuilder) copyKeyringFilesToVolume(ctx context.Context, node *ChainNode, hostKeyringDir string) error {
	// The cosmos keyring creates files in a keyring-test subdirectory
	keyringSubDir := path.Join(hostKeyringDir, "keyring-test")

	b.logger.Info("copying keyring files from host directory",
		zap.String("host_keyring_dir", keyringSubDir),
	)

	// List files in the keyring subdirectory
	files, err := os.ReadDir(keyringSubDir)
	if err != nil {
		return fmt.Errorf("failed to read keyring directory: %w", err)
	}

	b.logger.Info("found files in host keyring directory",
		zap.Int("file_count", len(files)),
	)

	// Copy each keyring file to the volume
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		hostFilePath := path.Join(keyringSubDir, file.Name())
		b.logger.Info("processing keyring file",
			zap.String("file_name", file.Name()),
			zap.String("host_file_path", hostFilePath),
		)

		// Read the file content
		content, err := os.ReadFile(hostFilePath)
		if err != nil {
			return fmt.Errorf("failed to read keyring file %s: %w", file.Name(), err)
		}

		// Write the file to the volume using the existing file utilities
		// writeFile expects a relative path from the home directory
		relativePath := path.Join("keyring-test", file.Name())
		err = node.WriteFile(ctx, relativePath, content)
		if err != nil {
			return fmt.Errorf("failed to write keyring file %s to volume: %w", file.Name(), err)
		}

		previewLen := len(content)
		if previewLen > 100 {
			previewLen = 100
		}
		b.logger.Info("wrote keyring file to volume",
			zap.String("file", file.Name()),
			zap.String("relative_path", relativePath),
			zap.Int("size", len(content)),
			zap.String("content_preview", string(content[:previewLen])),
		)
	}

	if b.logger != nil {
		b.logger.Info("preloaded keyring files to volume",
			zap.String("node", node.Name()),
			zap.Int("files", len(files)),
		)
	}

	return nil
}
