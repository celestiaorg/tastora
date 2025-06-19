package docker

import (
	"bytes"
	"context"
	sdkmath "cosmossdk.io/math"
	"fmt"
	"github.com/celestiaorg/go-square/v2/share"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	addressutil "github.com/celestiaorg/tastora/framework/testutil/address"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	dockerimagetypes "github.com/docker/docker/api/types/image"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"io"
	"sync"
	"testing"
)

var _ types.Chain = &Chain{}

const (
	defaultNumValidators = 2
	defaultNumFullNodes  = 1
)

var sentryPorts = nat.PortMap{
	nat.Port(p2pPort):     {},
	nat.Port(rpcPort):     {},
	nat.Port(grpcPort):    {},
	nat.Port(apiPort):     {},
	nat.Port(privValPort): {},
}

func newChain(ctx context.Context, t *testing.T, cfg Config) (types.Chain, error) {
	if cfg.ChainConfig == nil {
		return nil, fmt.Errorf("chain config must be set")
	}
	if cfg.ChainConfig.EncodingConfig == nil {
		return nil, fmt.Errorf("chain config must have an encoding config set")
	}

	registry := codectypes.NewInterfaceRegistry()
	cryptocodec.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)
	kr := keyring.NewInMemory(cdc)

	// If unspecified, NumValidators defaults to 2 and NumFullNodes defaults to 1.
	if cfg.ChainConfig.NumValidators == nil {
		nv := defaultNumValidators
		cfg.ChainConfig.NumValidators = &nv
	}
	if cfg.ChainConfig.NumFullNodes == nil {
		nf := defaultNumFullNodes
		cfg.ChainConfig.NumFullNodes = &nf
	}

	c := &Chain{
		t:       t,
		cfg:     cfg,
		cdc:     cdc,
		keyring: kr,
		log:     cfg.Logger,
	}

	// create the underlying docker resources for the chain.
	if err := c.initializeChainNodes(ctx, c.t.Name()); err != nil {
		return nil, err
	}

	return c, nil
}

type Chain struct {
	t            *testing.T
	cfg          Config
	Validators   ChainNodes
	FullNodes    ChainNodes
	numFullNodes int
	cdc          *codec.ProtoCodec
	log          *zap.Logger
	keyring      keyring.Keyring
	findTxMu     sync.Mutex
	faucetWallet types.Wallet
	broadcaster  types.Broadcaster

	// started is a bool indicating if the Chain has been started or not.
	// it is used to determine if files should be initialized at startup or
	// if the nodes should just be started.
	started bool
}

// GetFaucetWallet retrieves the faucet wallet for the chain.
func (c *Chain) GetFaucetWallet() types.Wallet {
	return c.faucetWallet
}

// GetChainID returns the chain ID.
func (c *Chain) GetChainID() string {
	return c.cfg.ChainConfig.ChainID
}

// getBroadcaster returns a broadcaster that can broadcast messages to this chain.
func (c *Chain) getBroadcaster() types.Broadcaster {
	if c.broadcaster != nil {
		return c.broadcaster
	}
	c.broadcaster = newBroadcaster(c.t, c)
	return c.broadcaster
}

// BroadcastMessages broadcasts the given messages signed on behalf of the provided user.
func (c *Chain) BroadcastMessages(ctx context.Context, signingWallet types.Wallet, msgs ...sdktypes.Msg) (sdktypes.TxResponse, error) {
	if c.faucetWallet.GetFormattedAddress() == "" {
		return sdktypes.TxResponse{}, fmt.Errorf("faucet wallet not initialized")
	}
	return c.getBroadcaster().BroadcastMessages(ctx, signingWallet, msgs...)
}

// BroadcastBlobMessage broadcasts the given messages signed on behalf of the provided user. The transaction bytes are wrapped
// using the MarshalBlobTx function before broadcasting.
func (c *Chain) BroadcastBlobMessage(ctx context.Context, signingWallet types.Wallet, msg sdk.Msg, blobs ...*share.Blob) (sdk.TxResponse, error) {
	return c.getBroadcaster().BroadcastBlobMessage(ctx, signingWallet, msg, blobs...)
}

func (c *Chain) AddNode(ctx context.Context, overrides map[string]any) error {
	return c.AddFullNodes(ctx, overrides, 1)
}

// AddFullNodes adds new fullnodes to the network, peering with the existing nodes.
func (c *Chain) AddFullNodes(ctx context.Context, configFileOverrides map[string]any, inc int) error {
	// Get peer string for existing nodes

	var peerAddressers []addressutil.PeerAddresser
	for _, n := range c.Nodes() {
		peerAddressers = append(peerAddressers, n)
	}

	peers, err := addressutil.BuildInternalPeerAddressList(ctx, peerAddressers)
	if err != nil {
		return err
	}

	// Get genesis.json
	genbz, err := c.Validators[0].genesisFileContent(ctx)
	if err != nil {
		return err
	}

	prevCount := c.numFullNodes
	c.numFullNodes += inc
	if err := c.initializeChainNodes(ctx, c.t.Name()); err != nil {
		return err
	}

	var eg errgroup.Group
	for i := prevCount; i < c.numFullNodes; i++ {
		eg.Go(func() error {
			fn := c.FullNodes[i]
			if err := fn.initFullNodeFiles(ctx); err != nil {
				return err
			}
			if err := fn.setPeers(ctx, peers); err != nil {
				return err
			}
			if err := fn.overwriteGenesisFile(ctx, genbz); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(toml.Toml)
				if !ok {
					return fmt.Errorf("provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
				}
				if err := ModifyConfigFile(
					ctx,
					fn.logger(),
					fn.DockerClient,
					fn.TestName,
					fn.VolumeName,
					configFile,
					modifiedToml,
				); err != nil {
					return err
				}
			}
			if err := fn.createNodeContainer(ctx); err != nil {
				return err
			}
			return fn.startContainer(ctx)
		})
	}
	return eg.Wait()
}

func (c *Chain) GetNodes() []types.ChainNode {
	var nodes []types.ChainNode
	for _, n := range c.Nodes() {
		nodes = append(nodes, n)
	}
	return nodes
}

func (c *Chain) GetGRPCAddress() string {
	return c.GetNode().hostGRPCPort
}

func (c *Chain) GetVolumeName() string {
	return c.GetNode().VolumeName
}

// GetHostRPCAddress returns the address of the RPC server accessible by the host.
// This will not return a valid address until the chain has been started.
func (c *Chain) GetHostRPCAddress() string {
	return "http://" + c.GetNode().hostRPCPort
}

func (c *Chain) Height(ctx context.Context) (int64, error) {
	return c.GetNode().Height(ctx)
}

// Start initializes and starts all nodes in the chain if not already started, otherwise starts all nodes without initialization.
func (c *Chain) Start(ctx context.Context) error {
	if c.started {
		return c.startAllNodes(ctx)
	}
	return c.startAndInitializeNodes(ctx)
}

// startAndInitializeNodes initializes and starts all chain nodes, configures genesis files, and ensures proper setup for the chain.
func (c *Chain) startAndInitializeNodes(ctx context.Context) error {
	cfg := c.cfg
	c.started = true

	genesisAmounts := make([][]sdk.Coin, len(c.Validators))
	genesisSelfDelegation := make([]sdk.Coin, len(c.Validators))

	for i := range c.Validators {
		genesisAmounts[i] = []sdk.Coin{{Amount: sdkmath.NewInt(10_000_000_000_000), Denom: cfg.ChainConfig.Denom}}
		genesisSelfDelegation[i] = sdk.Coin{Amount: sdkmath.NewInt(5_000_000), Denom: cfg.ChainConfig.Denom}
	}

	configFileOverrides := cfg.ChainConfig.ConfigFileOverrides

	eg := new(errgroup.Group)
	// Initialize config and sign gentx for each validator.
	for i, v := range c.Validators {
		v.Validator = true
		eg.Go(func() error {
			if err := v.initFullNodeFiles(ctx); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(toml.Toml)
				if !ok {
					return fmt.Errorf("provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
				}
				if err := ModifyConfigFile(
					ctx,
					c.cfg.Logger,
					v.DockerClient,
					v.TestName,
					v.VolumeName,
					configFile,
					modifiedToml,
				); err != nil {
					return fmt.Errorf("failed to modify toml config file: %w", err)
				}
			}
			return v.initValidatorGenTx(ctx, genesisAmounts[i], genesisSelfDelegation[i])
		})
	}

	// Initialize config for each full node.
	for _, n := range c.FullNodes {
		n.Validator = false
		eg.Go(func() error {
			if err := n.initFullNodeFiles(ctx); err != nil {
				return err
			}
			for configFile, modifiedConfig := range configFileOverrides {
				modifiedToml, ok := modifiedConfig.(toml.Toml)
				if !ok {
					return fmt.Errorf("provided toml override for file %s is of type (%T). Expected (DecodedToml)", configFile, modifiedConfig)
				}
				if err := ModifyConfigFile(
					ctx,
					n.logger(),
					n.DockerClient,
					n.TestName,
					n.VolumeName,
					configFile,
					modifiedToml,
				); err != nil {
					return err
				}
			}
			return nil
		})
	}

	// wait for this to finish
	if err := eg.Wait(); err != nil {
		return err
	}

	// for the validators we need to collect the gentxs and the accounts
	// to the first node's genesis file
	validator0 := c.Validators[0]
	for i := 1; i < len(c.Validators); i++ {
		validatorN := c.Validators[i]

		bech32, err := validatorN.accountKeyBech32(ctx, valKey)
		if err != nil {
			return err
		}

		if err := validator0.addGenesisAccount(ctx, bech32, genesisAmounts[0]); err != nil {
			return err
		}

		if err := validatorN.copyGentx(ctx, validator0); err != nil {
			return err
		}
	}

	// create the faucet wallet, this can be used to fund new wallets in the tests.
	wallet, err := c.CreateWallet(ctx, consts.FaucetAccountKeyName)
	if err != nil {
		return fmt.Errorf("failed to create faucet wallet: %w", err)
	}
	c.faucetWallet = wallet

	if err := validator0.addGenesisAccount(ctx, wallet.GetFormattedAddress(), []sdk.Coin{{Denom: c.cfg.ChainConfig.Denom, Amount: sdkmath.NewInt(10_000_000_000_000)}}); err != nil {
		return err
	}

	if err := validator0.collectGentxs(ctx); err != nil {
		return err
	}

	genbz, err := validator0.genesisFileContent(ctx)
	if err != nil {
		return err
	}

	genbz = bytes.ReplaceAll(genbz, []byte(`"stake"`), []byte(fmt.Sprintf(`"%s"`, cfg.ChainConfig.Denom)))

	if c.cfg.ChainConfig.ModifyGenesis != nil {
		genbz, err = c.cfg.ChainConfig.ModifyGenesis(cfg, genbz)
		if err != nil {
			return err
		}
	}

	chainNodes := c.Nodes()

	for _, cn := range chainNodes {
		if err := cn.overwriteGenesisFile(ctx, genbz); err != nil {
			return err
		}
	}

	eg, egCtx := errgroup.WithContext(ctx)
	for _, n := range chainNodes {
		eg.Go(func() error {
			return n.createNodeContainer(egCtx)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	peerAddressers := make([]addressutil.PeerAddresser, len(chainNodes))
	for i, n := range chainNodes {
		peerAddressers[i] = n
	}

	peers, err := addressutil.BuildInternalPeerAddressList(ctx, peerAddressers)
	if err != nil {
		return err
	}

	eg, egCtx = errgroup.WithContext(ctx)
	for _, n := range chainNodes {
		c.log.Info("Starting container", zap.String("container", n.Name()), zap.String("peers", peers))
		eg.Go(func() error {
			if err := n.setPeers(egCtx, peers); err != nil {
				return err
			}
			return n.startContainer(egCtx)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	// Wait for blocks before considering the chains "started"
	return wait.ForBlocks(ctx, 2, c.GetNode())
}

func (c *Chain) GetNode() *ChainNode {
	return c.Validators[0]
}

// Nodes returns all nodes, including validators and fullnodes.
func (c *Chain) Nodes() ChainNodes {
	return append(c.Validators, c.FullNodes...)
}

// startAllNodes creates and starts new containers for each node.
// Should only be used if the chain has previously been started with .Start.
func (c *Chain) startAllNodes(ctx context.Context) error {
	// prevent client calls during this time
	c.findTxMu.Lock()
	defer c.findTxMu.Unlock()
	var eg errgroup.Group
	for _, n := range c.Nodes() {
		eg.Go(func() error {
			if err := n.createNodeContainer(ctx); err != nil {
				return err
			}
			return n.startContainer(ctx)
		})
	}
	return eg.Wait()
}

func (c *Chain) Stop(ctx context.Context) error {
	var eg errgroup.Group
	for _, n := range c.Nodes() {
		eg.Go(func() error {
			if err := n.stop(ctx); err != nil {
				return err
			}
			return n.removeContainer(ctx)
		})
	}
	return eg.Wait()
}

// UpgradeVersion updates the chain's version across all components, including validators and full nodes, and pulls new images.
func (c *Chain) UpgradeVersion(ctx context.Context, version string) {
	c.cfg.ChainConfig.Images[0].Version = version
	for _, n := range c.Validators {
		n.Image.Version = version
	}
	for _, n := range c.FullNodes {
		n.Image.Version = version
	}
	c.pullImages(ctx)
}

// creates the test node objects required for bootstrapping tests.
func (c *Chain) initializeChainNodes(
	ctx context.Context,
	testName string,
) error {

	numValidators := *c.cfg.ChainConfig.NumValidators

	chainCfg := c.cfg
	c.pullImages(ctx)
	image := chainCfg.ChainConfig.Images[0]

	newVals := make(ChainNodes, numValidators)
	copy(newVals, c.Validators)
	newFullNodes := make(ChainNodes, c.numFullNodes)
	copy(newFullNodes, c.FullNodes)

	eg, egCtx := errgroup.WithContext(ctx)
	for i := len(c.Validators); i < numValidators; i++ {
		eg.Go(func() error {
			val, err := c.newChainNode(egCtx, testName, image, true, i)
			if err != nil {
				return err
			}
			newVals[i] = val
			return nil
		})
	}
	for i := len(c.FullNodes); i < c.numFullNodes; i++ {
		eg.Go(func() error {
			fn, err := c.newChainNode(egCtx, testName, image, false, i)
			if err != nil {
				return err
			}
			newFullNodes[i] = fn
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	c.findTxMu.Lock()
	defer c.findTxMu.Unlock()
	c.Validators = newVals
	c.FullNodes = newFullNodes
	return nil
}

// newChainNode constructs a new cosmos chain node with a docker volume.
func (c *Chain) newChainNode(
	ctx context.Context,
	testName string,
	image DockerImage,
	validator bool,
	index int,
) (*ChainNode, error) {
	// Construct the ChainNode first so we can access its name.
	// The ChainNode's VolumeName cannot be set until after we create the volume.
	tn := NewDockerChainNode(c.log, validator, c.cfg, testName, image, index)

	v, err := c.cfg.DockerClient.VolumeCreate(ctx, volumetypes.CreateOptions{
		Labels: map[string]string{
			consts.CleanupLabel:   testName,
			consts.NodeOwnerLabel: tn.Name(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume for chain node: %w", err)
	}
	tn.VolumeName = v.Name

	if err := SetVolumeOwner(ctx, VolumeOwnerOptions{
		Log:        c.log,
		Client:     c.cfg.DockerClient,
		VolumeName: v.Name,
		ImageRef:   image.Ref(),
		TestName:   testName,
		UidGid:     image.UIDGID,
	}); err != nil {
		return nil, fmt.Errorf("set volume owner: %w", err)
	}

	return tn, nil
}

func (c *Chain) pullImages(ctx context.Context) {
	for _, image := range c.cfg.ChainConfig.Images {
		if image.Version == "local" {
			continue
		}
		rc, err := c.cfg.DockerClient.ImagePull(
			ctx,
			image.Repository+":"+image.Version,
			dockerimagetypes.PullOptions{},
		)
		if err != nil {
			c.log.Error("Failed to pull image",
				zap.Error(err),
				zap.String("repository", image.Repository),
				zap.String("tag", image.Version),
			)
		} else {
			_, _ = io.Copy(io.Discard, rc)
			_ = rc.Close()
		}
	}
}

// CreateWallet will creates a new wallet.
func (c *Chain) CreateWallet(ctx context.Context, keyName string) (types.Wallet, error) {
	if err := c.createKey(ctx, keyName); err != nil {
		return nil, fmt.Errorf("failed to create key with name %q on chain %s: %w", keyName, c.cfg.ChainConfig.Name, err)
	}

	addrBytes, err := c.getAddress(ctx, keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to get account address for key %q on chain %s: %w", keyName, c.cfg.ChainConfig.Name, err)
	}

	formattedAddres := sdktypes.MustBech32ifyAddressBytes(c.cfg.ChainConfig.Bech32Prefix, addrBytes)

	w := NewWallet(addrBytes, formattedAddres, c.cfg.ChainConfig.Bech32Prefix, keyName)
	return &w, nil
}

func (c *Chain) createKey(ctx context.Context, keyName string) error {
	return c.GetNode().createKey(ctx, keyName)
}

func (c *Chain) getAddress(ctx context.Context, keyName string) ([]byte, error) {
	b32Addr, err := c.GetNode().accountKeyBech32(ctx, keyName)
	if err != nil {
		return nil, err
	}
	return sdk.GetFromBech32(b32Addr, c.cfg.ChainConfig.Bech32Prefix)
}
