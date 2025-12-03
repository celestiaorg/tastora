package hyperlane

import (
	"context"
	"fmt"
	"path"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const (
	hyperlaneHomeDir       = "/workspace"
	hyperlaneDefaultUIDGID = "0:0"
)

// Deployer is a deployment coordinator that executes Hyperlane contract deployments
// and configuration across multiple chains.
type Deployer struct {
	*container.Node

	cfg      Config
	schema   *Schema
	chains   []ChainConfigProvider
	deployed bool
}

// NewHyperlane creates a new Hyperlane deployment coordinator
func NewHyperlane(ctx context.Context, cfg Config, testName string, chains []ChainConfigProvider) (*Deployer, error) {
	if len(chains) == 0 {
		return nil, fmt.Errorf("at least one chain required")
	}

	image := cfg.HyperlaneImage
	if image.UIDGID == "" {
		image.UIDGID = hyperlaneDefaultUIDGID
	}

	node := container.NewNode(
		cfg.DockerNetworkID,
		cfg.DockerClient,
		testName,
		image,
		hyperlaneHomeDir,
		0, // should not be a need to ever have more than 1
		DeployerNodeType,
		cfg.Logger,
	)

	d := &Deployer{
		Node:   node,
		cfg:    cfg,
		chains: chains,
	}

	lifecycle := container.NewLifecycle(cfg.Logger, cfg.DockerClient, d.Name())
	d.SetContainerLifecycle(lifecycle)

	if err := d.CreateAndSetupVolume(ctx, d.Name()); err != nil {
		return nil, err
	}

	if err := d.init(ctx); err != nil {
		return nil, err
	}

	return d, nil
}

// Name returns the hostname of the docker container
func (h *Deployer) Name() string {
	return fmt.Sprintf("hyperlane-deploy-%d-%s", h.Index, internal.SanitizeDockerResourceName(h.TestName))
}

// Init generates configs and prepares the deployment
func (h *Deployer) init(ctx context.Context) error {
	h.Logger.Info("initializing hyperlane deployment coordinator")

	schema, err := BuildSchema(ctx, h.chains)
	if err != nil {
		return fmt.Errorf("failed to build schema: %w", err)
	}
	h.schema = schema

	if err := h.writeConfigs(ctx); err != nil {
		return fmt.Errorf("failed to write configs: %w", err)
	}

	h.Logger.Info("hyperlane initialization complete")
	return nil
}

// Deploy executes all deployment steps from the entrypoint script
func (h *Deployer) Deploy(ctx context.Context) error {
	if h.deployed {
		h.Logger.Info("deployment already completed, skipping")
		return nil
	}

	h.Logger.Info("starting hyperlane deployment")

	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"list registry", h.listRegistry},
		{"deploy core contracts", h.deployCoreContracts},
		{"deploy warp routes", h.deployWarpRoutes},
	}

	for _, step := range steps {
		h.Logger.Info("executing step", zap.String("step", step.name))
		if err := step.fn(ctx); err != nil {
			return fmt.Errorf("failed at step %s: %w", step.name, err)
		}
	}

	h.deployed = true
	h.Logger.Info("hyperlane deployment completed successfully")
	return nil
}

func (h *Deployer) writeConfigs(ctx context.Context) error {
	relayerConfigBytes, err := SerializeRelayerConfig(h.schema.RelayerConfig)
	if err != nil {
		return fmt.Errorf("failed to serialize relayer config: %w", err)
	}
	if err := h.WriteFile(ctx, "relayer-config.json", relayerConfigBytes); err != nil {
		return fmt.Errorf("failed to write relayer config: %w", err)
	}

	if err := h.writeRegistry(ctx); err != nil {
		return fmt.Errorf("failed to write registry: %w", err)
	}

	if err := h.writeWarpConfig(ctx); err != nil {
		return fmt.Errorf("failed to write warp config: %w", err)
	}

	h.Logger.Debug("wrote all config files to volume")
	return nil
}

func (h *Deployer) writeRegistry(ctx context.Context) error {
	for chainName, entry := range h.schema.Registry.Chains {
		metadataBytes, err := yaml.Marshal(entry.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata for %s: %w", chainName, err)
		}
		metadataPath := path.Join("registry", "chains", chainName, "metadata.yaml")
		if err := h.WriteFile(ctx, metadataPath, metadataBytes); err != nil {
			return fmt.Errorf("failed to write metadata for %s: %w", chainName, err)
		}

		addressesBytes, err := yaml.Marshal(entry.Addresses)
		if err != nil {
			return fmt.Errorf("failed to marshal addresses for %s: %w", chainName, err)
		}
		addressesPath := path.Join("registry", "chains", chainName, "addresses.yaml")
		if err := h.WriteFile(ctx, addressesPath, addressesBytes); err != nil {
			return fmt.Errorf("failed to write addresses for %s: %w", chainName, err)
		}
	}

	return nil
}

func (h *Deployer) writeWarpConfig(ctx context.Context) error {
	warpConfig := make(map[string]*WarpConfigEntry)

	for _, chain := range h.chains {
		entry, err := chain.GetHyperlaneWarpConfigEntry(ctx)
		if err != nil {
			return fmt.Errorf("failed to get warp config entry: %w", err)
		}
		if entry != nil {
			registryEntry, err := chain.GetHyperlaneRegistryEntry(ctx)
			if err != nil {
				return fmt.Errorf("failed to get registry entry: %w", err)
			}
			warpConfig[registryEntry.Metadata.Name] = entry
		}
	}

	if len(warpConfig) == 0 {
		return fmt.Errorf("no chains with warp config found")
	}

	warpConfigBytes, err := yaml.Marshal(warpConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal warp config: %w", err)
	}

	warpConfigPath := path.Join("configs", "warp-config.yaml")
	if err := h.WriteFile(ctx, warpConfigPath, warpConfigBytes); err != nil {
		return fmt.Errorf("failed to write warp config: %w", err)
	}

	return nil
}
