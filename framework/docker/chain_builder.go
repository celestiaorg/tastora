package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
	"path"
	"testing"
)

type NodeConfig struct {
	// validator is a flag indicating whether the node is a validator or a full node.
	validator           bool
	image               DockerImage
	additionalStartArgs []string
	envVars             []string
}

// NodeConfigBuilder provides a fluent interface for building NodeConfig
type NodeConfigBuilder struct {
	config NodeConfig
}

// NewNodeConfigBuilder creates a new NodeConfigBuilder
func NewNodeConfigBuilder() *NodeConfigBuilder {
	return &NodeConfigBuilder{
		config: NodeConfig{
			validator:           false,
			additionalStartArgs: make([]string, 0),
			envVars:             make([]string, 0),
		},
	}
}

// WithImage sets the Docker image for the node
func (b *NodeConfigBuilder) WithImage(image DockerImage) *NodeConfigBuilder {
	b.config.image = image
	return b
}

// WithAdditionalStartArgs sets the additional start arguments
func (b *NodeConfigBuilder) WithAdditionalStartArgs(args ...string) *NodeConfigBuilder {
	b.config.additionalStartArgs = args
	return b
}

// WithEnvVars sets the environment variables
func (b *NodeConfigBuilder) WithEnvVars(envVars ...string) *NodeConfigBuilder {
	b.config.envVars = envVars
	return b
}

// Build returns the configured NodeConfig
func (b *NodeConfigBuilder) Build() NodeConfig {
	return b.config
}

type ChainBuilder struct {
	t               *testing.T
	validators      []NodeConfig
	fullNodes       []NodeConfig
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
}

func NewChainBuilder(t *testing.T) *ChainBuilder {
	t.Helper()
	cb := &ChainBuilder{}
	return cb.
		WithT(t).
		WithBinaryName("celestia-appd").
		WithCoinType("118").
		WithGasPrices("0.025utia").
		WithChainID("test")
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
func (b *ChainBuilder) WithValidators(validators ...NodeConfig) *ChainBuilder {
	b.validators = validators
	return b
}

// WithFullNodes sets the full node configurations
func (b *ChainBuilder) WithFullNodes(fullNodes ...NodeConfig) *ChainBuilder {
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

// AddValidator adds a single validator node configuration
func (b *ChainBuilder) AddValidator(validator NodeConfig) *ChainBuilder {
	validator.validator = true
	b.validators = append(b.validators, validator)
	return b
}

// AddFullNode adds a single full node configuration
func (b *ChainBuilder) AddFullNode(fullNode NodeConfig) *ChainBuilder {
	fullNode.validator = false
	b.fullNodes = append(b.fullNodes, fullNode)
	return b
}

func (b *ChainBuilder) Build(ctx context.Context) (*Chain, error) {
	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kr := keyring.NewInMemory(cdc)

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
				Images:         []DockerImage{b.validators[0].image},
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
		keyring:    kr,
	}

	return chain, nil
}

func (b *ChainBuilder) initializeChainNodes(ctx context.Context) ([]*ChainNode, error) {
	var nodes []*ChainNode
	for i, val := range b.validators {
		n, err := b.newChainNode(ctx, val.image, true, i, val.additionalStartArgs...)
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
	image DockerImage,
	validator bool,
	index int,
	additionalStartArgs ...string,
) (*ChainNode, error) {
	// Construct the ChainNode first so we can access its name.
	// The ChainNode's VolumeName cannot be set until after we create the volume.
	tn := b.newDockerChainNode(b.logger, validator, image, index, additionalStartArgs)

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

	if err := SetVolumeOwner(ctx, VolumeOwnerOptions{
		Log:        b.logger,
		Client:     b.dockerClient,
		VolumeName: v.Name,
		ImageRef:   image.Ref(),
		TestName:   b.t.Name(),
		UidGid:     image.UIDGID,
	}); err != nil {
		return nil, fmt.Errorf("set volume owner: %w", err)
	}

	return tn, nil
}

func (b *ChainBuilder) newDockerChainNode(log *zap.Logger, validator bool, image DockerImage, index int, additionalStartArgs []string) *ChainNode {
	params := ChainNodeParams{
		Logger:              log,
		Validator:           validator,
		DockerClient:        b.dockerClient,
		DockerNetworkID:     b.dockerNetworkID,
		TestName:            b.t.Name(),
		Image:               image,
		Index:               index,
		ChainID:             b.chainID,
		BinaryName:          b.binaryName,
		CoinType:            b.coinType,
		GasPrices:           b.gasPrices,
		GasAdjustment:       1.0,        // Default gas adjustment
		Env:                 []string{}, // Default empty env
		AdditionalStartArgs: additionalStartArgs,
		EncodingConfig:      b.encodingConfig,
		ChainNodeConfig:     nil, // No per-node config by default
		HomeDir:             path.Join("/var/cosmos-chain", b.name),
	}

	return NewChainNode(params)
}
