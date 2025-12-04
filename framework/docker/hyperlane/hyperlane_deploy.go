package hyperlane

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"
	"path"

	"go.uber.org/zap"
)

const (
	registryPath = "/workspace/registry"
	configsPath  = "/workspace/configs"
)

func (h *Deployer) listRegistry(ctx context.Context) error {
	cmd := []string{"hyperlane", "registry", "list", "--registry", registryPath}

	stdout, stderr, err := h.Exec(ctx, h.Logger, cmd, nil)
	if err != nil {
		h.Logger.Error("registry list failed",
			zap.String("stdout", string(stdout)),
			zap.String("stderr", string(stderr)),
			zap.Error(err))
		return fmt.Errorf("registry list failed: %w", err)
	}

	h.Logger.Debug("registry list output", zap.String("stdout", string(stdout)))
	return nil
}

// preflightRegistry performs basic diagnostics to verify the container can
// read and write the mounted registry path and logs directory contents.
func (h *Deployer) preflightRegistry(ctx context.Context) {
	// Best-effort diagnostics; failures here should not be fatal but will be logged.
	// 1) Whoami
	_, _, _ = h.Exec(ctx, h.Logger, []string{"sh", "-lc", "echo UID=$(id -u) GID=$(id -g)"}, nil)
	// 2) List registry dirs
	_, _, _ = h.Exec(ctx, h.Logger, []string{"sh", "-lc", "ls -la /workspace || true; ls -la /workspace/registry || true; ls -la /workspace/registry/chains || true"}, nil)
	// 3) Write probe file
	_, _, _ = h.Exec(ctx, h.Logger, []string{"sh", "-lc", "echo rwtest > /workspace/registry/_preflight_rw.txt && ls -la /workspace/registry/_preflight_rw.txt"}, nil)
}

func (h *Deployer) deployCoreContracts(ctx context.Context) error {
	var evmChainName string
	var signerKey string
	for name, chainCfg := range h.schema.RelayerConfig.Chains {
		if chainCfg.Protocol == "ethereum" {
			evmChainName = name
			if chainCfg.Signer != nil {
				signerKey = chainCfg.Signer.Key
			}
			break
		}
	}

	if evmChainName == "" {
		// No EVM chains present; skip core deployment step.
		h.Logger.Info("no EVM chain found; skipping core deployment")
		return nil
	}

	// run preflight diagnostics to catch permission/layout issues early
	h.preflightRegistry(ctx)

	cmd := []string{
		"hyperlane", "core", "deploy",
		"--chain", evmChainName,
		"--registry", registryPath,
		"--yes",
	}

	env := []string{
		fmt.Sprintf("HYP_KEY=%s", signerKey),
	}

	stdout, stderr, err := h.Exec(ctx, h.Logger, cmd, env)
	if err != nil {
		// Post-error diagnostics: list registry contents to help pinpoint the fault.
		_, _, _ = h.Exec(ctx, h.Logger, []string{"sh", "-lc", "echo '--- registry dump ---'; ls -la /workspace/registry/chains || true; for d in /workspace/registry/chains/*; do echo \"--- $d ---\"; ls -la \"$d\" || true; for f in metadata.yaml metadata.json addresses.yaml addresses.json; do [ -f \"$d/$f\" ] && (echo \"--- cat $d/$f ---\"; sed -n '1,120p' \"$d/$f\") || true; done; done"}, nil)
		h.Logger.Error("core deploy failed",
			zap.String("stdout", string(stdout)),
			zap.String("stderr", string(stderr)),
			zap.Error(err))
		return fmt.Errorf("core deploy failed: %w", err)
	}

	h.Logger.Info("core contracts deployed", zap.String("chain", evmChainName))
	return nil
}

func (h *Deployer) deployWarpRoutes(ctx context.Context) error {
	cmd := []string{
		"hyperlane", "warp", "deploy",
		"--config", path.Join(configsPath, "warp-config.yaml"),
		"--registry", registryPath,
		"--yes",
	}

	stdout, stderr, err := h.Exec(ctx, h.Logger, cmd, nil)
	if err != nil {
		h.Logger.Error("warp deploy failed",
			zap.String("stdout", string(stdout)),
			zap.String("stderr", string(stderr)),
			zap.Error(err))
		return fmt.Errorf("warp deploy failed: %w", err)
	}

	h.Logger.Info("warp routes deployed")
	return nil
}

// writeCoreConfig generates configs/core-config.yaml from the registry and signer
func (h *Deployer) writeCoreConfig(ctx context.Context) error {
	// find first EVM chain and signer
	var evmChainName string
	var signerKey string
	for name, chainCfg := range h.schema.RelayerConfig.Chains {
		if chainCfg.Protocol == "ethereum" {
			evmChainName = name
			if chainCfg.Signer != nil {
				signerKey = chainCfg.Signer.Key
			}
			break
		}
	}
	if evmChainName == "" {
		return fmt.Errorf("no EVM chain found for core config")
	}

	// read addresses written by CLI from registry
	addrBytes, err := h.ReadFile(ctx, path.Join("registry", "chains", evmChainName, "addresses.yaml"))
	if err != nil {
		return fmt.Errorf("read addresses: %w", err)
	}
	var addrs ContractAddresses
	if err := yaml.Unmarshal(addrBytes, &addrs); err != nil {
		return fmt.Errorf("unmarshal addresses: %w", err)
	}

	// derive owner address from signerKey (hex privkey)
	ownerAddr, err := deriveEthAddress(signerKey)
	if err != nil {
		return fmt.Errorf("derive owner address: %w", err)
	}

    // build core-config structure
    core := CoreConfig{}
    core.DefaultHook = HookCfg{Address: addrs.MerkleTreeHook, Type: "merkleTreeHook"}
	// prefer TestIsm if present, otherwise InterchainSecurityModule
	if addrs.TestIsm != "" {
        core.DefaultIsm = HookCfg{Address: addrs.TestIsm, Type: "testIsm"}
    } else if addrs.InterchainSecurityModule != "" {
        core.DefaultIsm = HookCfg{Address: addrs.InterchainSecurityModule, Type: "testIsm"}
    }
    core.InterchainAccountRouter = InterchainAccountRouterCfg{
        Address:          addrs.InterchainAccountRouter,
        Mailbox:          addrs.Mailbox,
        Owner:            ownerAddr,
        ProxyAdmin:       ProxyAdminCfg{Address: addrs.ProxyAdmin, Owner: ownerAddr},
        RemoteIcaRouters: map[string]string{},
    }

    core.Owner = ownerAddr
    core.ProxyAdmin = ProxyAdminCfg{Address: addrs.ProxyAdmin, Owner: ownerAddr}
    // requiredHook maps to protocolFee settings; address is InterchainGasPaymaster
    core.RequiredHook = RequiredHookCfg{
        Address:        addrs.InterchainGasPaymaster,
        Beneficiary:    ownerAddr,
        MaxProtocolFee: "10000000000000000000000000000",
        Owner:          ownerAddr,
        ProtocolFee:    "0",
        Type:           "protocolFee",
    }

	// write YAML file
	b, err := yaml.Marshal(core)
	if err != nil {
		return fmt.Errorf("marshal core-config: %w", err)
	}
	if err := h.WriteFile(ctx, path.Join("configs", "core-config.yaml"), b); err != nil {
		return fmt.Errorf("write core-config: %w", err)
	}
	h.Logger.Info("wrote core-config.yaml")
	return nil
}
