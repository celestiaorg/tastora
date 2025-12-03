package hyperlane

import (
	"context"
	"fmt"
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
		return fmt.Errorf("no EVM chain found for core deployment")
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

	stdout, stderr, err := h.Exec(ctx, h.Logger, cmd, env)
	if err != nil {
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
