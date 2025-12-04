package hyperlane

import (
	"context"
	"fmt"
	"path"

	ismtypes "github.com/bcp-innovations/hyperlane-cosmos/x/core/01_interchain_security/types"
	"github.com/celestiaorg/tastora/framework/types"
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
	var evmChainName string
	var signerKey string
	for name, chainCfg := range d.relayerCfg.Chains {
		if chainCfg.Protocol == "ethereum" {
			evmChainName = name
			if chainCfg.Signer != nil {
				signerKey = chainCfg.Signer.Key
			}
			break
		}
	}

	if evmChainName == "" {
		d.Logger.Info("no EVM chain found, skipping core deployment")
		return nil
	}

	cmd := []string{
		"hyperlane", "core", "deploy",
		"--chain", evmChainName,
		"--registry", registryPath,
		"--yes",
	}

	env := []string{
		fmt.Sprintf("HYP_KEY=%s", signerKey),
	}
	_, _, err := d.Exec(ctx, d.Logger, cmd, env)
	if err != nil {
		return fmt.Errorf("core deploy failed: %w", err)
	}

	d.Logger.Info("core contracts deployed", zap.String("chain", evmChainName))

	// NOTE: the `hyperlane core deploy` step writes `addresses.yaml` to disk which is required to write the core
	// config to disk and so this step happens after execution.
	if err := d.writeCoreConfig(ctx); err != nil {
		return fmt.Errorf("failed to write core config: %w", err)
	}

	return nil
}

func (d *Deployer) deployWarpRoutes(ctx context.Context) error {
	cmd := []string{
		"hyperlane", "warp", "deploy",
		"--config", path.Join(configsPath, "warp-config.yaml"),
		"--registry", registryPath,
		"--yes",
	}

	_, _, err := d.Exec(ctx, d.Logger, cmd, nil)
	if err != nil {
		return fmt.Errorf("warp deploy failed: %w", err)
	}

	d.Logger.Info("warp routes deployed")

	return nil
}

// writeCoreConfig generates configs/core-config.yaml from the registry and signer
func (d *Deployer) writeCoreConfig(ctx context.Context) error {
	// find first EVM chain and signer
	var evmChainName string
	for name, chainCfg := range d.relayerCfg.Chains {
		if chainCfg.Protocol == "ethereum" {
			evmChainName = name
			break
		}
	}
	if evmChainName == "" {
		return fmt.Errorf("no EVM chain found for core config")
	}

	// read addresses written by CLI from registry
	addrBytes, err := d.ReadFile(ctx, path.Join("registry", "chains", evmChainName, "addresses.yaml"))
	if err != nil {
		return fmt.Errorf("read addresses: %w", err)
	}

	var addrs ContractAddresses
	if err := yaml.Unmarshal(addrBytes, &addrs); err != nil {
		return fmt.Errorf("unmarshal addresses: %w", err)
	}

	// build core-config structure
	// modeled after https://github.com/celestiaorg/celestia-zkevm/blob/927364fec76bc78bc390953590f07d48d430dc20/hyperlane/configs/core-config.yaml#L1
	core := CoreConfig{
		DefaultHook: HookCfg{
			Address: addrs.MerkleTreeHook,
			Type:    "merkleTreeHook",
		},
		InterchainAccountRouter: InterchainAccountRouterCfg{
			Address:          addrs.InterchainAccountRouter,
			Mailbox:          addrs.Mailbox,
			Owner:            ownerAddr,
			ProxyAdmin:       ProxyAdminCfg{Address: addrs.ProxyAdmin, Owner: ownerAddr},
			RemoteIcaRouters: map[string]string{},
		},
		Owner: ownerAddr,
		ProxyAdmin: ProxyAdminCfg{
			Address: addrs.ProxyAdmin,
			Owner:   ownerAddr,
		},
		RequiredHook: RequiredHookCfg{
			Address:        addrs.InterchainGasPaymaster,
			Beneficiary:    ownerAddr,
			MaxProtocolFee: "10000000000000000000000000000",
			Owner:          ownerAddr,
			ProtocolFee:    "0",
			Type:           "protocolFee",
		},
		DefaultIsm: HookCfg{
			Address: addrs.InterchainSecurityModule,
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

// DeployNoopISMCosmos broadcasts MsgCreateNoopIsm using the chain's faucet wallet via the framework broadcaster.
func (d *Deployer) DeployNoopISMCosmos(ctx context.Context, chain types.Broadcaster, sender *types.Wallet) error {
	msg := &ismtypes.MsgCreateNoopIsm{Creator: sender.GetFormattedAddress()}
	if _, err := chain.BroadcastMessages(ctx, sender, msg); err != nil {
		return fmt.Errorf("broadcast MsgCreateNoopIsm failed: %w", err)
	}
	d.Logger.Info("noop ISM created via BroadcastMessages", zap.String("from", sender.GetFormattedAddress()))
	return nil
}
