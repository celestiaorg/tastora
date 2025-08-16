package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/celestiaorg/go-square/v2/share"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	addressutil "github.com/celestiaorg/tastora/framework/testutil/address"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dockerimagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var _ types.Chain = &Chain{}

var sentryPorts = nat.PortMap{
	nat.Port(p2pPort):     {},
	nat.Port(rpcPort):     {},
	nat.Port(grpcPort):    {},
	nat.Port(apiPort):     {},
	nat.Port(privValPort): {},
}

type Chain struct {
	t          *testing.T
	cfg        Config
	Validators ChainNodes
	FullNodes  ChainNodes
	cdc        *codec.ProtoCodec
	log        *zap.Logger

	mu          sync.Mutex
	broadcaster types.Broadcaster

	faucetWallet types.Wallet

	// started is a bool indicating if the Chain has been started or not.
	// it is used to determine if files should be initialized at startup or
	// if the nodes should just be started.
	started bool
}

func (c *Chain) GetRelayerConfig() types.ChainRelayerConfig {
	return types.ChainRelayerConfig{
		ChainID:      c.GetChainID(),
		Denom:        c.cfg.ChainConfig.Denom,
		GasPrices:    c.cfg.ChainConfig.GasPrices,
		Bech32Prefix: c.cfg.ChainConfig.Bech32Prefix,
		RPCAddress:   "http://" + c.GetNode().Name() + ":26657",
		GRPCAddress:  "http://" + c.GetNode().Name() + ":9090",
	}
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
	c.broadcaster = NewBroadcaster(c)
	return c.broadcaster
}

// BroadcastMessages broadcasts the given messages signed on behalf of the provided user.
func (c *Chain) BroadcastMessages(ctx context.Context, signingWallet types.Wallet, msgs ...sdk.Msg) (sdk.TxResponse, error) {
	if c.GetFaucetWallet() == nil {
		return sdk.TxResponse{}, fmt.Errorf("faucet wallet not initialized")
	}
	return c.getBroadcaster().BroadcastMessages(ctx, signingWallet, msgs...)
}

// BroadcastBlobMessage broadcasts the given messages signed on behalf of the provided user. The transaction bytes are wrapped
// using the MarshalBlobTx function before broadcasting.
func (c *Chain) BroadcastBlobMessage(ctx context.Context, signingWallet types.Wallet, msg sdk.Msg, blobs ...*share.Blob) (sdk.TxResponse, error) {
	return c.getBroadcaster().BroadcastBlobMessage(ctx, signingWallet, msg, blobs...)
}

// AddNode adds a single full node to the chain with the given configuration
func (c *Chain) AddNode(ctx context.Context, nodeConfig ChainNodeConfig) error {
	if nodeConfig.nodeType != types.NodeTypeConsensusFull {
		// TODO: this is preserving existing functionality, we can update this to support addition of validator nodes.
		return fmt.Errorf("node type must be %s", types.NodeTypeConsensusFull.String())
	}

	// get genesis.json
	genbz, err := c.Validators[0].genesisFileContent(ctx)
	if err != nil {
		return err
	}

	// create a builder to access newChainNode method
	builder := NewChainBuilderFromChain(c)

	existingNodeCount := len(c.Nodes())

	// create the node directly using builder's newChainNode method
	node, err := builder.newChainNode(ctx, nodeConfig, existingNodeCount)
	if err != nil {
		return err
	}

	if err := node.initNodeFiles(ctx); err != nil {
		return err
	}

	peers, err := addressutil.BuildInternalPeerAddressList(ctx, c.Nodes())
	if err != nil {
		return err
	}

	if err := node.setPeers(ctx, peers); err != nil {
		return err
	}

	if err := node.overwriteGenesisFile(ctx, genbz); err != nil {
		return err
	}

	// execute any custom post-init functions
	// these can modify config files or modify genesis etc.
	for _, fn := range node.PostInit {
		if err := fn(ctx, node); err != nil {
			return err
		}
	}

	if err := node.createNodeContainer(ctx); err != nil {
		return err
	}
	if err := node.startContainer(ctx); err != nil {
		return err
	}

	// add the new node to the chain
	c.mu.Lock()
	defer c.mu.Unlock()
	c.FullNodes = append(c.FullNodes, node)

	return nil
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
	c.started = true
	defaultGenesisAmount := sdk.NewCoins(sdk.NewCoin(c.cfg.ChainConfig.Denom, sdkmath.NewInt(10_000_000_000_000)))
	defaultGenesisSelfDelegation := sdk.NewCoin(c.cfg.ChainConfig.Denom, sdkmath.NewInt(5_000_000))

	eg := new(errgroup.Group)
	// initialize config and sign gentx for each validator.
	for _, v := range c.Validators {
		v.Validator = true
		eg.Go(func() error {
			if err := v.initNodeFiles(ctx); err != nil {
				return err
			}

			// we don't want to initialize the validator if it has a keyring.
			if v.GenesisKeyring != nil {
				return nil
			}

			return v.initValidatorGenTx(ctx, defaultGenesisAmount, defaultGenesisSelfDelegation)
		})
	}
	// initialize config for each full node.
	for _, n := range c.FullNodes {
		n.Validator = false
		eg.Go(func() error {
			return n.initNodeFiles(ctx)
		})
	}

	// wait for this to finish
	if err := eg.Wait(); err != nil {
		return err
	}

	genesisBz, err := c.getGenesisFileBz(ctx, defaultGenesisAmount)
	if err != nil {
		return err
	}

	chainNodes := c.Nodes()
	for _, cn := range chainNodes {

		if err := cn.overwriteGenesisFile(ctx, genesisBz); err != nil {
			return err
		}

		// test case has explicitly set a priv_validator_key.json contents.
		if cn.PrivValidatorKey != nil {
			if err := cn.overwritePrivValidatorKey(ctx, cn.PrivValidatorKey); err != nil {
				return err
			}
		}
	}

	// for all chain nodes, execute any functions provided.
	// these can do things like override config files or make any other modifications
	// before the chain node starts.
	for _, cn := range chainNodes {
		for _, fn := range cn.PostInit {
			if err := fn(ctx, cn); err != nil {
				return err
			}
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

	peers, err := addressutil.BuildInternalPeerAddressList(ctx, chainNodes)
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
	if err := wait.ForBlocks(ctx, 2, c.GetNode()); err != nil {
		return err
	}

	// copy faucet key to all other validators now that containers are running.
	// this ensures the faucet wallet can be used on all nodes.
	// since the faucet wallet is only created if a genesis keyring is not provided, we only copy it over if that's the case.
	if len(c.Validators) > 0 && c.Validators[0].GenesisKeyring == nil {
		c.Validators[0].faucetWallet = c.GetFaucetWallet()
		for i := 1; i < len(c.Validators); i++ {
			if err := c.copyFaucetKeyToValidator(c.GetFaucetWallet(), c.Validators[i]); err != nil {
				return fmt.Errorf("failed to copy faucet key to validator %d: %w", i, err)
			}
			c.Validators[i].faucetWallet = c.Validators[0].faucetWallet
		}
	}

	return nil
}

// initDefaultGenesis initializes the default genesis file with validators and a faucet account for funding test wallets.
// it distributes the default genesis amount to all validators and ensures gentx files are collected and included.
func (c *Chain) initDefaultGenesis(ctx context.Context, defaultGenesisAmount sdk.Coins) ([]byte, error) {
	validator0 := c.Validators[0]
	for i := 1; i < len(c.Validators); i++ {
		validatorN := c.Validators[i]

		bech32, err := validatorN.accountKeyBech32(ctx, valKey)
		if err != nil {
			return nil, err
		}

		if err := validator0.addGenesisAccount(ctx, bech32, defaultGenesisAmount); err != nil {
			return nil, err
		}

		if err := validatorN.copyGentx(ctx, validator0); err != nil {
			return nil, err
		}
	}

	// create the faucet wallet, this can be used to fund new wallets in the tests.
	wallet, err := c.CreateWallet(ctx, consts.FaucetAccountKeyName)
	if err != nil {
		return nil, fmt.Errorf("failed to create faucet wallet: %w", err)
	}
	c.faucetWallet = wallet

	if err := validator0.addGenesisAccount(ctx, wallet.GetFormattedAddress(), []sdk.Coin{{Denom: c.cfg.ChainConfig.Denom, Amount: sdkmath.NewInt(10_000_000_000_000)}}); err != nil {
		return nil, err
	}

	if err := validator0.collectGentxs(ctx); err != nil {
		return nil, err
	}

	genbz, err := validator0.genesisFileContent(ctx)
	if err != nil {
		return nil, err
	}
	genbz = bytes.ReplaceAll(genbz, []byte(`"stake"`), []byte(fmt.Sprintf(`"%s"`, c.cfg.ChainConfig.Denom)))
	return genbz, nil
}

func (c *Chain) GetNode() *ChainNode {
	return c.Nodes()[0]
}

// Nodes returns all nodes, including validators and fullnodes.
func (c *Chain) Nodes() ChainNodes {
	return append(c.Validators, c.FullNodes...)
}

// startAllNodes creates and starts new containers for each node.
// Should only be used if the chain has previously been started with .Start.
func (c *Chain) startAllNodes(ctx context.Context) error {
	// prevent client calls during this time
	c.mu.Lock()
	defer c.mu.Unlock()
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
			return n.RemoveContainer(ctx)
		})
	}
	return eg.Wait()
}

// UpgradeVersion updates the chain's version across all components, including validators and full nodes, and pulls new images.
func (c *Chain) UpgradeVersion(ctx context.Context, version string) {
	c.cfg.ChainConfig.Image.Version = version
	for _, n := range c.Validators {
		n.Image.Version = version
	}
	for _, n := range c.FullNodes {
		n.Image.Version = version
	}
	c.pullImages(ctx)
}

// pullImages pulls all images used by the chain chains.
func (c *Chain) pullImages(ctx context.Context) {
	pulled := make(map[string]struct{})
	for _, n := range c.Nodes() {
		image := n.Image
		if _, ok := pulled[image.Ref()]; ok {
			continue
		}

		pulled[image.Ref()] = struct{}{}
		rc, err := c.cfg.DockerClient.ImagePull(
			ctx,
			image.Ref(),
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

// CreateWallet creates a new wallet using Validator[0] to maintain backward compatibility.
func (c *Chain) CreateWallet(ctx context.Context, keyName string) (types.Wallet, error) {
	return c.GetNode().CreateWallet(ctx, keyName, c.cfg.ChainConfig.Bech32Prefix)
}

// copyFaucetKeyToValidator copies the faucet key from validator[0] to the specified validator.
func (c *Chain) copyFaucetKeyToValidator(faucetWallet types.Wallet, targetValidator *ChainNode) error {
	faucetKeyName := faucetWallet.GetKeyName()

	// as part of setup, the faucet wallet was created on Validators[0]
	sourceKeyring, err := c.Validators[0].GetKeyring()
	if err != nil {
		return fmt.Errorf("failed to get source keyring: %w", err)
	}

	// get the keyring from target validator
	targetKeyring, err := targetValidator.GetKeyring()
	if err != nil {
		return fmt.Errorf("failed to get target keyring: %w", err)
	}

	// export the key from source
	armoredKey, err := sourceKeyring.ExportPrivKeyArmor(faucetKeyName, "")
	if err != nil {
		return fmt.Errorf("failed to export faucet key: %w", err)
	}

	// import the key to target
	if err := targetKeyring.ImportPrivKey(faucetKeyName, armoredKey, ""); err != nil {
		return fmt.Errorf("failed to import faucet key: %w", err)
	}

	return nil
}

// getGenesisFileBz retrieves the genesis file bytes for the chain, generating a default genesis if none is specified.
func (c *Chain) getGenesisFileBz(ctx context.Context, defaultGenesisAmount sdk.Coins) ([]byte, error) {
	if c.cfg.ChainConfig.GenesisFileBz != nil {
		return c.cfg.ChainConfig.GenesisFileBz, nil
	}
	// only perform initial genesis and faucet account creation if no genesis keyring is provided.
	if len(c.Validators) > 0 && c.Validators[0].GenesisKeyring == nil {
		return c.initDefaultGenesis(ctx, defaultGenesisAmount)
	}

	panic("genesis file must be specified no validator nodes are present")
}
