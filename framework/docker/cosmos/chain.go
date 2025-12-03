package cosmos

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"io"
	"path"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/testutil/maps"
	govv1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	sdkmath "cosmossdk.io/math"
	"github.com/celestiaorg/go-square/v3/share"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	addressutil "github.com/celestiaorg/tastora/framework/testutil/address"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	dockerimagetypes "github.com/docker/docker/api/types/image"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var _ types.Chain = &Chain{}

type Chain struct {
	t          *testing.T
	Config     ChainConfig
	Validators ChainNodes
	FullNodes  ChainNodes
	cdc        *codec.ProtoCodec
	log        *zap.Logger

	mu          sync.Mutex
	broadcaster types.Broadcaster

	faucetWallet *types.Wallet

	// started is a bool indicating if the Chain has been started or not.
	// it is used to determine if files should be initialized at startup or
	// if the nodes should just be started.
	started bool
	// skipInit indicates whether to skip initialization when starting
	skipInit bool
}

// GetHyperlaneChainMetadata returns the ChainMetadata configuration required to configure hyperlane for this
// instance.
func (c *Chain) GetHyperlaneChainMetadata(ctx context.Context) (hyperlane.ChainMetadata, error) {
	networkInfo, err := c.GetNetworkInfo(ctx)
	if err != nil {
		return hyperlane.ChainMetadata{}, err
	}

	signerKey, err := c.getFaucetPrivateKeyHex()
	if err != nil {
		return hyperlane.ChainMetadata{}, fmt.Errorf("failed to get faucet private key: %w", err)
	}

	return hyperlane.ChainMetadata{
		ChainID:     c.GetChainID(),
		DomainID:    69420,
		Name:        c.Config.Name,
		DisplayName: c.Config.Name,
		Protocol:    "cosmosnative",
		IsTestnet:   true,
		NativeToken: hyperlane.NativeToken{
			Name:     "TIA",
			Symbol:   "TIA",
			Decimals: 6,
			Denom:    c.Config.Denom,
		},
		RpcURLs: []hyperlane.Endpoint{
			{
				HTTP: fmt.Sprintf("http://%s", networkInfo.Internal.RPCAddress()),
			},
		},
		RestURLs: []hyperlane.Endpoint{
			{
				HTTP: fmt.Sprintf("http://%s", networkInfo.Internal.APIAddress()),
			},
		},
		Blocks: &hyperlane.BlockConfig{
			Confirmations:     1,
			EstimateBlockTime: 6,
			ReorgPeriod:       1,
		},
		TechnicalStack:       "other",
		Bech32Prefix:         c.Config.Bech32Prefix,
		CanonicalAsset:       c.Config.Denom,
		ContractAddressBytes: 0,
		GasPrice: &hyperlane.GasPrice{
			Denom:  c.Config.Denom,
			Amount: c.Config.GasPrices,
		},
		Slip44:    118,
		SignerKey: signerKey,
		CoreContracts: &hyperlane.CoreContractAddresses{
			Mailbox:                  "0x68797065726c616e650000000000000000000000000000000000000000000000",
			InterchainSecurityModule: "0x726f757465725f69736d00000000000000000000000000000000000000000000",
			InterchainGasPaymaster:   "0x726f757465725f706f73745f6469737061746368000000000000000000000000",
			MerkleTreeHook:           "0x726f757465725f706f73745f6469737061746368000000030000000000000001",
			ValidatorAnnounce:        "0x68797065726c616e650000000000000000000000000000000000000000000000",
		},
		IndexConfig: &hyperlane.IndexConfig{
			From:  1150,
			Chunk: 10,
		},
	}, nil
}

func (c *Chain) GetRelayerConfig() types.ChainRelayerConfig {
	return types.ChainRelayerConfig{
		ChainID:      c.GetChainID(),
		Denom:        c.Config.Denom,
		GasPrices:    c.Config.GasPrices,
		Bech32Prefix: c.Config.Bech32Prefix,
		RPCAddress:   "http://" + c.GetNode().Name() + ":26657",
		GRPCAddress:  "http://" + c.GetNode().Name() + ":9090",
	}
}

// GetFaucetWallet retrieves the faucet wallet for the chain.
// If the wallet is not initialized, it attempts to load it from the first node.
func (c *Chain) GetFaucetWallet() *types.Wallet {
	if c.faucetWallet == nil {
		// attempt to load the faucet wallet from the first node
		if err := c.loadFaucetWallet(context.Background()); err != nil {
			c.log.Error("failed to load faucet wallet", zap.Error(err))
			return nil
		}
	}
	return c.faucetWallet
}

// getFaucetPrivateKeyHex retrieves the faucet wallet's private key in hex format.
func (c *Chain) getFaucetPrivateKeyHex() (string, error) {
	if c.GetFaucetWallet() == nil {
		return "", fmt.Errorf("faucet wallet not initialized")
	}

	if len(c.Validators) == 0 {
		return "", fmt.Errorf("no validators available")
	}

	node := c.GetNode()
	kr, err := node.GetKeyring()
	if err != nil {
		return "", fmt.Errorf("failed to get keyring: %w", err)
	}

	armoredKey, err := kr.ExportPrivKeyArmor(consts.FaucetAccountKeyName, "")
	if err != nil {
		return "", fmt.Errorf("failed to export faucet key: %w", err)
	}

	privKey, _, err := crypto.UnarmorDecryptPrivKey(armoredKey, "")
	if err != nil {
		return "", fmt.Errorf("failed to decrypt armored key: %w", err)
	}

	privKeyBytes := privKey.Bytes()
	return "0x" + hex.EncodeToString(privKeyBytes), nil
}

// GetChainID returns the chain ID.
func (c *Chain) GetChainID() string {
	return c.Config.ChainID
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
func (c *Chain) BroadcastMessages(ctx context.Context, signingWallet *types.Wallet, msgs ...sdk.Msg) (sdk.TxResponse, error) {
	if c.GetFaucetWallet() == nil {
		return sdk.TxResponse{}, fmt.Errorf("faucet wallet not initialized")
	}
	return c.getBroadcaster().BroadcastMessages(ctx, signingWallet, msgs...)
}

// BroadcastBlobMessage broadcasts the given messages signed on behalf of the provided user. The transaction bytes are wrapped
// using the MarshalBlobTx function before broadcasting.
func (c *Chain) BroadcastBlobMessage(ctx context.Context, signingWallet *types.Wallet, msg sdk.Msg, blobs ...*share.Blob) (sdk.TxResponse, error) {
	if signingWallet == nil {
		return sdk.TxResponse{}, fmt.Errorf("signing wallet is nil")
	}
	return c.getBroadcaster().BroadcastBlobMessage(ctx, signingWallet, msg, blobs...)
}

// AddNode adds a single full node to the chain with the given configuration
func (c *Chain) AddNode(ctx context.Context, nodeConfig ChainNodeConfig) error {
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

	if err := c.copyWalletToValidator(c.GetFaucetWallet(), node); err != nil {
		return fmt.Errorf("failed to copy faucet key to new node: %w", err)
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

func (c *Chain) GetNetworkInfo(ctx context.Context) (types.NetworkInfo, error) {
	node := c.GetNode()
	return node.GetNetworkInfo(ctx)
}

func (c *Chain) GetVolumeName() string {
	return c.GetNode().VolumeName
}

func (c *Chain) Height(ctx context.Context) (int64, error) {
	return c.GetNode().Height(ctx)
}

// Start initializes and starts all nodes in the chain if not already started, otherwise starts all nodes without initialization.
func (c *Chain) Start(ctx context.Context) error {
	if c.started || c.skipInit {
		return c.startAllNodes(ctx)
	}
	return c.startAndInitializeNodes(ctx)
}

// startAndInitializeNodes initializes and starts all chain nodes, configures genesis files, and ensures proper setup for the chain.
func (c *Chain) startAndInitializeNodes(ctx context.Context) error {
	c.started = true
	defaultGenesisAmount := sdk.NewCoins(sdk.NewCoin(c.Config.Denom, sdkmath.NewInt(10_000_000_000_000)))
	defaultGenesisSelfDelegation := sdk.NewCoin(c.Config.Denom, sdkmath.NewInt(5_000_000))

	eg := new(errgroup.Group)
	// initialize config and sign gentx for each validator.
	for _, v := range c.Validators {
		v := v
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
		n := n
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
		return fmt.Errorf("failed to get genesis file: %w", err)
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
		n := n
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
		n := n
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
	// Use a longer timeout for block waiting to handle slow chain startup
	blockWaitCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := wait.ForBlocks(blockWaitCtx, 2, c.GetNode()); err != nil {
		return err
	}

	// copy faucet key to all other validators now that containers are running.
	// this ensures the faucet wallet can be used on all nodes.
	// since the faucet wallet is only created if a genesis keyring is not provided, we only copy it over if that's the case.
	if len(c.Validators) > 0 && c.Validators[0].GenesisKeyring == nil {
		c.Validators[0].faucetWallet = c.GetFaucetWallet()
		for i := 1; i < len(c.Validators); i++ {
			if err := c.copyWalletToValidator(c.GetFaucetWallet(), c.Validators[i]); err != nil {
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

	if err := validator0.addGenesisAccount(ctx, wallet.GetFormattedAddress(), []sdk.Coin{{Denom: c.Config.Denom, Amount: sdkmath.NewInt(10_000_000_000_000)}}); err != nil {
		return nil, err
	}

	if err := validator0.collectGentxs(ctx); err != nil {
		return nil, err
	}

	genbz, err := validator0.genesisFileContent(ctx)
	if err != nil {
		return nil, err
	}

	// modify the genesis to have short voting and deposit periods for gov proposals
	// to make possible to test gov proposals within tests without additional  configuration.
	genbz, err = maps.SetFields(genbz,
		maps.Entry{
			Path:  "app_state.gov.params.voting_period",
			Value: "30s",
		},
		maps.Entry{
			Path:  "app_state.gov.params.max_deposit_period",
			Value: "10s",
		},
		maps.Entry{
			Path: "app_state.gov.params.min_deposit",
			Value: []map[string]interface{}{
				{
					"denom":  c.Config.Denom,
					"amount": "1",
				},
			},
		},
	)

	if err != nil {
		return nil, err
	}

	genbz = bytes.ReplaceAll(genbz, []byte(`"stake"`), []byte(fmt.Sprintf(`"%s"`, c.Config.Denom)))

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
		n := n
		eg.Go(func() error {
			if err := n.createNodeContainer(ctx); err != nil {
				return err
			}
			return n.startContainer(ctx)
		})
	}
	return eg.Wait()
}

// Stop stops all nodes in the chain without removing them.
func (c *Chain) Stop(ctx context.Context) error {
	var eg errgroup.Group
	for _, n := range c.Nodes() {
		n := n
		eg.Go(func() error {
			return n.Stop(ctx)
		})
	}
	return eg.Wait()
}

// Remove stops and removes all nodes in the chain.
func (c *Chain) Remove(ctx context.Context, opts ...types.RemoveOption) error {
	var eg errgroup.Group
	for _, n := range c.Nodes() {
		n := n
		eg.Go(func() error {
			return n.Remove(ctx, opts...)
		})
	}
	return eg.Wait()
}

// UpgradeVersion updates the chain's version across all components, including validators and full nodes, and pulls new images.
// It removes containers while preserving volumes, updates images, and restarts the chain with the new version.
func (c *Chain) UpgradeVersion(ctx context.Context, version string) error {
	// remove containers but preserve volumes for upgrade
	if err := c.Remove(ctx, types.WithPreserveVolumes()); err != nil {
		return fmt.Errorf("failed to remove containers for upgrade: %w", err)
	}

	// Update image versions
	c.Config.Image.Version = version
	for _, n := range c.Validators {
		n.Image.Version = version
	}
	for _, n := range c.FullNodes {
		n.Image.Version = version
	}

	c.pullImages(ctx)

	// Start the chain with the new version
	if err := c.Start(ctx); err != nil {
		return fmt.Errorf("failed to start chain after upgrade: %w", err)
	}

	return nil
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
		rc, err := c.Config.DockerClient.ImagePull(
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

// CreateWallet creates a new wallet on the first node and copies the key to all other nodes.
func (c *Chain) CreateWallet(ctx context.Context, keyName string) (*types.Wallet, error) {
	wallet, err := c.GetNode().CreateWallet(ctx, keyName, c.Config.Bech32Prefix)
	if err != nil {
		return nil, err
	}

	// the faucet wallet is handled separately during chain initialization.
	// when we are creating the faucet wallet, the other nodes will not be started yet
	// and so cannot copy the key to them.
	if keyName != consts.FaucetAccountKeyName {
		if err := c.copyWalletKeyToAllNodes(wallet); err != nil {
			return nil, fmt.Errorf("failed to copy wallet key to all nodes: %w", err)
		}
	}

	return wallet, nil
}

// copyWalletKeyToAllNodes copies a wallet's key from the first node to all other nodes in the chain.
func (c *Chain) copyWalletKeyToAllNodes(wallet *types.Wallet) error {
	nodes := c.Nodes()
	if len(nodes) <= 1 {
		return nil
	}

	for i := 1; i < len(nodes); i++ {
		if err := c.ensureNodeKeyringInitialized(nodes[i]); err != nil {
			return fmt.Errorf("failed to initialize keyring for node %d: %w", i, err)
		}

		if err := c.copyWalletToValidator(wallet, nodes[i]); err != nil {
			return fmt.Errorf("failed to copy wallet key to node %d: %w", i, err)
		}
	}

	return nil
}

// ensureNodeKeyringInitialized ensures the keyring directory exists on the target node.
func (c *Chain) ensureNodeKeyringInitialized(node *ChainNode) error {
	keyringDir := path.Join(node.HomeDir(), "keyring-test")
	_, _, err := node.Exec(context.Background(), []string{"mkdir", "-p", keyringDir}, nil)
	return err
}

// copyWalletToValidator copies the faucet key from validator[0] to the specified validator.
func (c *Chain) copyWalletToValidator(wallet *types.Wallet, targetValidator *ChainNode) error {
	keyName := wallet.GetKeyName()

	sourceKeyring, err := c.Validators[0].GetKeyring()
	if err != nil {
		return fmt.Errorf("failed to get source keyring: %w", err)
	}

	if err := c.ensureNodeKeyringInitialized(targetValidator); err != nil {
		return fmt.Errorf("failed to initialize keyring directory: %w", err)
	}

	targetKeyring, err := targetValidator.GetKeyring()
	if err != nil {
		return fmt.Errorf("failed to get target keyring: %w", err)
	}

	armoredKey, err := sourceKeyring.ExportPrivKeyArmor(keyName, "")
	if err != nil {
		return fmt.Errorf("failed to export faucet key: %w", err)
	}

	if err := targetKeyring.ImportPrivKey(keyName, armoredKey, ""); err != nil {
		return fmt.Errorf("failed to import faucet key: %w", err)
	}

	return nil
}

// loadFaucetWallet loads the faucet wallet from the first node's keyring.
// This is used when skipInit is true and the wallet wasn't created during initialization.
func (c *Chain) loadFaucetWallet(ctx context.Context) error {
	if len(c.Validators) == 0 {
		return fmt.Errorf("no validators available to load faucet wallet from")
	}

	node := c.GetNode()
	kr, err := node.GetKeyring()
	if err != nil {
		return fmt.Errorf("failed to get keyring: %w", err)
	}

	keyInfo, err := kr.Key(consts.FaucetAccountKeyName)
	if err != nil {
		return fmt.Errorf("failed to get faucet key from keyring: %w", err)
	}

	addr, err := keyInfo.GetAddress()
	if err != nil {
		return fmt.Errorf("failed to get address from key: %w", err)
	}

	formattedAddress := sdk.MustBech32ifyAddressBytes(c.Config.Bech32Prefix, addr.Bytes())
	c.faucetWallet = types.NewWallet(addr.Bytes(), formattedAddress, c.Config.Bech32Prefix, consts.FaucetAccountKeyName)
	return nil
}

// getGenesisFileBz retrieves the genesis file bytes for the chain, generating a default genesis if none is specified.
func (c *Chain) getGenesisFileBz(ctx context.Context, defaultGenesisAmount sdk.Coins) ([]byte, error) {
	if c.Config.GenesisFileBz != nil {
		return c.Config.GenesisFileBz, nil
	}
	// only perform initial genesis and faucet account creation if no genesis keyring is provided.
	if len(c.Validators) > 0 && c.Validators[0].GenesisKeyring == nil {
		return c.initDefaultGenesis(ctx, defaultGenesisAmount)
	}

	return nil, fmt.Errorf("genesis file must be specified if no validator nodes are present")
}

// SubmitAndVoteOnGovV1Proposal submits a governance proposal and has all nodes vote based on the specified option.
func (c *Chain) SubmitAndVoteOnGovV1Proposal(ctx context.Context, proposal *govv1.MsgSubmitProposal, option govv1.VoteOption) (*govv1.Proposal, error) {
	resp, err := c.BroadcastMessages(ctx, c.GetFaucetWallet(), proposal)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast proposal: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("failed to submit proposal: %s", resp.RawLog)
	}

	proposalID, err := extractProposalIDFromResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to extract proposal ID from response: %w", err)
	}

	govQueryClient := govv1.NewQueryClient(c.GetNode().GrpcConn)

	// wait for the proposal to be indexed before voting
	err = wait.ForCondition(ctx, time.Second*30, time.Millisecond*500, func() (bool, error) {
		_, err := govQueryClient.Proposal(ctx, &govv1.QueryProposalRequest{ProposalId: proposalID})
		if err != nil {
			// proposal not yet indexed, keep waiting
			return false, nil
		}
		// proposal exists, we can proceed
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("proposal was not indexed after submission: %w", err)
	}

	for _, n := range c.Validators {
		if err := n.VoteOnProposal(ctx, proposalID, option); err != nil {
			return nil, fmt.Errorf("node %s failed to vote on proposal: %w", n.Name(), err)
		}
	}

	if err := wait.ForBlocks(ctx, 2, c.GetNode()); err != nil {
		return nil, fmt.Errorf("failed to wait for blocks after voting: %w", err)
	}

	var finalProp *govv1.QueryProposalResponse
	err = wait.ForCondition(ctx, time.Minute*2, time.Second*1, func() (bool, error) {
		prop, err := govQueryClient.Proposal(ctx, &govv1.QueryProposalRequest{ProposalId: proposalID})
		if err != nil {
			return false, fmt.Errorf("failed to query proposal: %w", err)
		}

		// keep waiting while in deposit or voting period
		if prop.Proposal.Status == govv1.ProposalStatus_PROPOSAL_STATUS_DEPOSIT_PERIOD ||
			prop.Proposal.Status == govv1.ProposalStatus_PROPOSAL_STATUS_VOTING_PERIOD {
			return false, nil
		}

		// proposal is in a final state (passed, rejected, or failed)
		finalProp = prop
		return true, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to wait for vote to be finished: %w", err)
	}

	return finalProp.Proposal, nil
}

// extractProposalIDFromResponse extracts the proposal ID from the transaction response events.
func extractProposalIDFromResponse(resp sdk.TxResponse) (uint64, error) {
	for _, event := range resp.Events {
		if event.Type == "submit_proposal" {
			for _, attr := range event.Attributes {
				if attr.Key == "proposal_id" {
					proposalID, err := strconv.ParseUint(attr.Value, 10, 64)
					if err != nil {
						return 0, fmt.Errorf("failed to parse proposal ID %q: %w", attr.Value, err)
					}
					return proposalID, nil
				}
			}
		}
	}
	return 0, fmt.Errorf("proposal_id not found in transaction events")
}
