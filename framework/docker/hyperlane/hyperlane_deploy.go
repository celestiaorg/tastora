package hyperlane

import (
	"context"
	sdkmath "cosmossdk.io/math"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane/internal"
	"path"

	hyputil "github.com/bcp-innovations/hyperlane-cosmos/util"
	ismtypes "github.com/bcp-innovations/hyperlane-cosmos/x/core/01_interchain_security/types"
	hooktypes "github.com/bcp-innovations/hyperlane-cosmos/x/core/02_post_dispatch/types"
	coretypes "github.com/bcp-innovations/hyperlane-cosmos/x/core/types"
	warptypes "github.com/bcp-innovations/hyperlane-cosmos/x/warp/types"
	evmutil "github.com/celestiaorg/tastora/framework/testutil/evm"
	"github.com/celestiaorg/tastora/framework/types"
	abci "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const (
	registryPath = "/workspace/registry"
	configsPath  = "/workspace/configs"

	// ownerAddr is the address corresponding to the HYP_KEY environment variable private key.
	// it is present in the default evolve genesis as a pre-funded account.
	ownerAddr = "0xaF9053bB6c4346381C77C2FeD279B17ABAfCDf4d"
)

func (d *Deployer) deployCoreContracts(ctx context.Context) error {
	cmd := []string{
		"hyperlane", "core", "deploy",
		"--config", path.Join(configsPath, "core-config.yaml"),
		"--chain", d.evmChainName,
		"--registry", registryPath,
		"--yes",
	}

	env := []string{
		fmt.Sprintf("HYP_KEY=%s", d.hypPrivateKey),
	}

	if err := d.writeCoreConfig(ctx); err != nil {
		return fmt.Errorf("failed to write core config: %w", err)
	}

	_, _, err := d.Exec(ctx, d.Logger, cmd, env)
	if err != nil {
		return fmt.Errorf("core deploy failed: %w", err)
	}

	d.Logger.Info("core contracts deployed", zap.String("chain", d.evmChainName))

	return nil
}

func (d *Deployer) deployWarpRoutes(ctx context.Context) error {
	cmd := []string{
		"hyperlane", "warp", "deploy",
		"--config", path.Join(configsPath, "warp-config.yaml"),
		"--registry", registryPath,
		"--yes",
	}

	env := []string{
		fmt.Sprintf("HYP_KEY=%s", d.hypPrivateKey),
	}

	_, _, err := d.Exec(ctx, d.Logger, cmd, env)
	if err != nil {
		return fmt.Errorf("warp deploy failed: %w", err)
	}

	d.Logger.Info("warp routes deployed")

	return nil
}

// writeCoreConfig generates configs/core-config.yaml from the registry and signer
func (d *Deployer) writeCoreConfig(ctx context.Context) error {
	// find first EVM chain and signer
	var chainCfg RelayerChainConfig
	for _, c := range d.relayerCfg.Chains {
		if c.Protocol == "ethereum" {
			chainCfg = c
			break
		}
	}

	if chainCfg.Name == "" {
		return fmt.Errorf("no EVM chain found for core config")
	}

	// build core-config structure
	// modeled after https://github.com/celestiaorg/celestia-zkevm/blob/927364fec76bc78bc390953590f07d48d430dc20/hyperlane/configs/core-config.yaml#L1
	core := CoreConfig{
		DefaultHook: HookCfg{
			Address: QuotedHexAddress(chainCfg.MerkleTreeHook),
			Type:    "merkleTreeHook",
		},
		InterchainAccountRouter: InterchainAccountRouterCfg{
			Address:          QuotedHexAddress("0x4dc4E8bf5D0390C95Af9AFEb1e9c9927c4dB83e7"), // TODO: don't hard code this
			Mailbox:          QuotedHexAddress(chainCfg.Mailbox),
			Owner:            QuotedHexAddress(ownerAddr),
			ProxyAdmin:       ProxyAdminCfg{Address: QuotedHexAddress(chainCfg.ProxyAdmin), Owner: QuotedHexAddress(ownerAddr)},
			RemoteIcaRouters: map[string]string{},
		},
		Owner: QuotedHexAddress(ownerAddr),
		ProxyAdmin: ProxyAdminCfg{
			Address: QuotedHexAddress(chainCfg.ProxyAdmin),
			Owner:   QuotedHexAddress(ownerAddr),
		},
		RequiredHook: RequiredHookCfg{
			Address:        QuotedHexAddress(chainCfg.InterchainGasPaymaster),
			Beneficiary:    QuotedHexAddress(ownerAddr),
			MaxProtocolFee: "10000000000000000000000000000",
			Owner:          QuotedHexAddress(ownerAddr),
			ProtocolFee:    "0",
			Type:           "protocolFee",
		},
		DefaultIsm: HookCfg{
			Address: QuotedHexAddress(chainCfg.InterchainSecurityModule),
			Type:    "testIsm",
		},
	}

	bz, err := yaml.Marshal(core)
	if err != nil {
		return fmt.Errorf("marshal core-config: %w", err)
	}

	if err := d.WriteFile(ctx, path.Join("configs", "core-config.yaml"), bz); err != nil {
		return fmt.Errorf("write core-config: %w", err)
	}

	d.Logger.Info("wrote core-config.yaml")
	return nil
}

// EnrollRemoteRouterOnCosmos enrolls a remote router for a given token on the Cosmos chain
// by broadcasting a MsgEnrollRemoteRouter via the provided broadcaster and wallet.
// tokenID is the Cosmos bytes32 identifier for the router/token (hyputil.HexAddress),
// remoteDomain is the EVM domain ID, and receiverContract is the EVM router contract (0x-prefixed hex).
func (d *Deployer) EnrollRemoteRouterOnCosmos(ctx context.Context, b types.Broadcaster, wallet *types.Wallet, tokenID hyputil.HexAddress, remoteDomain uint32, receiverContract string) error {
	msg := &warptypes.MsgEnrollRemoteRouter{
		Owner:   wallet.GetFormattedAddress(),
		TokenId: tokenID,
		RemoteRouter: &warptypes.RemoteRouter{
			ReceiverDomain:   remoteDomain,
			ReceiverContract: receiverContract,
			Gas:              sdkmath.NewInt(0),
		},
	}
	if _, err := b.BroadcastMessages(ctx, wallet, msg); err != nil {
		return fmt.Errorf("broadcast MsgEnrollRemoteRouter failed: %w", err)
	}
	d.Logger.Info("enrolled remote router on cosmos", zap.Uint32("remote_domain", remoteDomain), zap.String("receiver_contract", receiverContract))
	return nil
}

// DeployCosmosNoopISM deploys the complete cosmos-native hyperlane stack including ISM, hooks, mailbox, and token.
func (d *Deployer) DeployCosmosNoopISM(ctx context.Context, chain types.Broadcaster, sender *types.Wallet) (*CosmosConfig, error) {
	ismID, err := d.deployNoopISM(ctx, chain, sender)
	if err != nil {
		return nil, fmt.Errorf("deploy noop ISM: %w", err)
	}
	d.Logger.Info("created noop ISM", zap.String("ism_id", ismID.String()))

	hooksID, err := d.deployNoopHook(ctx, chain, sender)
	if err != nil {
		return nil, fmt.Errorf("deploy noop hook: %w", err)
	}
	d.Logger.Info("created noop hook", zap.String("hooks_id", hooksID.String()))

	mailboxID, err := d.createMailbox(ctx, chain, sender, ismID, hooksID)
	if err != nil {
		return nil, fmt.Errorf("create mailbox: %w", err)
	}
	d.Logger.Info("created mailbox", zap.String("mailbox_id", mailboxID.String()))

	merkleTreeHookID, err := d.deployMerkleTreeHook(ctx, chain, sender, mailboxID)
	if err != nil {
		return nil, fmt.Errorf("deploy merkle tree hook: %w", err)
	}
	d.Logger.Info("created merkle tree hook", zap.String("merkle_tree_hook_id", merkleTreeHookID.String()))

	tokenID, err := d.createCollateralToken(ctx, chain, sender, mailboxID)
	if err != nil {
		return nil, fmt.Errorf("create collateral token: %w", err)
	}
	d.Logger.Info("created collateral token", zap.String("token_id", tokenID.String()))

	if err := d.setTokenISM(ctx, chain, sender, tokenID, ismID); err != nil {
		return nil, fmt.Errorf("set token ISM: %w", err)
	}

	d.Logger.Info("set ISM on token")

	config := &CosmosConfig{
		IsmID:            ismID,
		HooksID:          hooksID,
		MailboxID:        mailboxID,
		TokenID:          tokenID,
		MerkleTreeHookID: merkleTreeHookID,
	}

	d.Logger.Info("cosmos-native noop-ism deployment completed")
	return config, nil
}

// EnrollRemoteRouter invokes enrollRemoteRouter(uint32,bytes32) on the given contract
// using EVM settings (RPC URL + signer) from the relayer config for the first EVM chain.
// routerHex must be a 0x-prefixed 32-byte hex string.
func (d *Deployer) EnrollRemoteRouter(ctx context.Context, contractAddress string, domain uint32, routerHex string, chainName string, rpcURL string) (gethcommon.Hash, error) {
	var signerKey string
	for _, chainCfg := range d.relayerCfg.Chains {
		if chainCfg.Name == chainName {
			if chainCfg.Protocol != "ethereum" {
				return gethcommon.Hash{}, fmt.Errorf("chain %s is not an evm chain", chainName)
			}
			if len(chainCfg.RpcURLs) == 0 || chainCfg.Signer == nil {
				return gethcommon.Hash{}, fmt.Errorf("evm chain missing rpcUrls or signer in relayer config")
			}
			signerKey = chainCfg.Signer.Key
			break
		}
	}

	if rpcURL == "" || signerKey == "" {
		return gethcommon.Hash{}, fmt.Errorf("no evm chain configured in relayer config")
	}

	router := gethcommon.HexToHash(routerHex)

	sender, err := evmutil.NewSender(ctx, rpcURL)
	if err != nil {
		return gethcommon.Hash{}, fmt.Errorf("connect evm rpc: %w", err)
	}
	defer sender.Close()

	txHash, err := sender.SendFunctionTx(ctx, signerKey, contractAddress, internal.HyperlaneRouterABI, "enrollRemoteRouter", domain, router)
	if err != nil {
		return gethcommon.Hash{}, fmt.Errorf("enrollRemoteRouter tx failed: %w", err)
	}
	d.Logger.Info("enrolled remote router", zap.Uint32("domain", domain), zap.String("contract", contractAddress), zap.String("tx", txHash.Hex()))
	return txHash, nil
}

func (d *Deployer) deployNoopISM(ctx context.Context, chain types.Broadcaster, sender *types.Wallet) (hyputil.HexAddress, error) {
	msg := &ismtypes.MsgCreateNoopIsm{Creator: sender.GetFormattedAddress()}
	resp, err := chain.BroadcastMessages(ctx, sender, msg)
	if err != nil {
		return hyputil.HexAddress{}, fmt.Errorf("broadcast MsgCreateNoopIsm: %w", err)
	}
	return parseEventValue(resp.Events, "/hyperlane.core.interchain_security.v1.EventCreateNoopIsm", func(e *ismtypes.EventCreateNoopIsm) (hyputil.HexAddress, bool) {
		return e.IsmId, true
	})
}

func (d *Deployer) deployNoopHook(ctx context.Context, chain types.Broadcaster, sender *types.Wallet) (hyputil.HexAddress, error) {
	msg := &hooktypes.MsgCreateNoopHook{Owner: sender.GetFormattedAddress()}
	resp, err := chain.BroadcastMessages(ctx, sender, msg)
	if err != nil {
		return hyputil.HexAddress{}, fmt.Errorf("broadcast MsgCreateNoopHook: %w", err)
	}
	return parseEventValue(resp.Events, "/hyperlane.core.post_dispatch.v1.EventCreateNoopHook", func(e *hooktypes.EventCreateNoopHook) (hyputil.HexAddress, bool) {
		return e.NoopHookId, true
	})
}

func (d *Deployer) deployMerkleTreeHook(ctx context.Context, chain types.Broadcaster, sender *types.Wallet, mailboxID hyputil.HexAddress) (hyputil.HexAddress, error) {
	msg := &hooktypes.MsgCreateMerkleTreeHook{
		Owner:     sender.GetFormattedAddress(),
		MailboxId: mailboxID,
	}
	resp, err := chain.BroadcastMessages(ctx, sender, msg)
	if err != nil {
		return hyputil.HexAddress{}, fmt.Errorf("broadcast MsgCreateMerkleTreeHook: %w", err)
	}
	return parseEventValue(resp.Events, "/hyperlane.core.post_dispatch.v1.EventCreateMerkleTreeHook", func(e *hooktypes.EventCreateMerkleTreeHook) (hyputil.HexAddress, bool) {
		return e.MerkleTreeHookId, true
	})
}

func (d *Deployer) createMailbox(ctx context.Context, chain types.Broadcaster, sender *types.Wallet, ismID, hooksID hyputil.HexAddress) (hyputil.HexAddress, error) {
	var domainID uint32
	for _, chainCfg := range d.relayerCfg.Chains {
		if chainCfg.Protocol == "cosmosnative" {
			domainID = chainCfg.DomainID
			break
		}
	}

	msg := &coretypes.MsgCreateMailbox{
		Owner:        sender.GetFormattedAddress(),
		LocalDomain:  domainID,
		DefaultIsm:   ismID,
		DefaultHook:  &hooksID,
		RequiredHook: &hooksID,
	}

	resp, err := chain.BroadcastMessages(ctx, sender, msg)
	if err != nil {
		return hyputil.HexAddress{}, fmt.Errorf("broadcast MsgCreateMailbox: %w", err)
	}
	return parseEventValue(resp.Events, "/hyperlane.core.v1.EventCreateMailbox", func(e *coretypes.EventCreateMailbox) (hyputil.HexAddress, bool) {
		return e.MailboxId, true
	})
}

func (d *Deployer) createCollateralToken(ctx context.Context, chain types.Broadcaster, sender *types.Wallet, mailboxID hyputil.HexAddress) (hyputil.HexAddress, error) {
	msg := &warptypes.MsgCreateCollateralToken{
		Owner:         sender.GetFormattedAddress(),
		OriginMailbox: mailboxID,
		OriginDenom:   "utia",
	}
	resp, err := chain.BroadcastMessages(ctx, sender, msg)
	if err != nil {
		return hyputil.HexAddress{}, fmt.Errorf("broadcast MsgCreateCollateralToken: %w", err)
	}

	return parseEventValue(resp.Events, "/hyperlane.warp.v1.EventCreateCollateralToken", func(e *warptypes.EventCreateCollateralToken) (hyputil.HexAddress, bool) {
		return e.TokenId, true
	})
}

func (d *Deployer) setTokenISM(ctx context.Context, chain types.Broadcaster, sender *types.Wallet, tokenID, ismID hyputil.HexAddress) error {
	msg := &warptypes.MsgSetToken{
		Owner:   sender.GetFormattedAddress(),
		TokenId: tokenID,
		IsmId:   &ismID,
	}
	_, err := chain.BroadcastMessages(ctx, sender, msg)
	if err != nil {
		return fmt.Errorf("broadcast MsgSetToken: %w", err)
	}
	return nil
}

// parseEventValue extracts a HexAddress from a specified event type given the type url and type T.
func parseEventValue[T any](
	events []abci.Event,
	typeURL string,
	extract func(T) (hyputil.HexAddress, bool),
) (hyputil.HexAddress, error) {

	for _, evt := range events {
		typedEvt, err := sdk.ParseTypedEvent(evt)
		if err != nil {
			continue
		}

		if sdk.MsgTypeURL(typedEvt) != typeURL {
			continue
		}

		e, ok := typedEvt.(T)
		if !ok {
			continue
		}

		if val, ok := extract(e); ok {
			return val, nil
		}
	}

	return hyputil.HexAddress{}, fmt.Errorf("event %s not found", typeURL)
}
