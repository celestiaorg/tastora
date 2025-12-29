package hyperlane

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"gopkg.in/yaml.v3"
)

const (
	hyperlaneHomeDir       = "/workspace"
	hyperlaneDefaultUIDGID = "1000:1000"
)

// Deployer is a deployment coordinator that executes Hyperlane contract deployments
// and configuration across multiple chains.
type Deployer struct {
	*container.Node

	cfg        Config
	relayerCfg *RelayerConfig
	registry   *Registry
	chains     []ChainConfigProvider
	deployed   bool
	hasWarp    bool
}

// NewDeployer creates a new Hyperlane deployment coordinator
func NewDeployer(ctx context.Context, cfg Config, testName string, chains []ChainConfigProvider) (*Deployer, error) {
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
func (d *Deployer) Name() string {
	return fmt.Sprintf("hyperlane-evstack-%d-%s", d.Index, internal.SanitizeDockerResourceName(d.TestName))
}

// Init generates configs and prepares the deployment
func (d *Deployer) init(ctx context.Context) error {
	d.Logger.Info("initializing hyperlane deployment coordinator")

	relayerCfg, err := BuildRelayerConfig(ctx, d.chains)
	if err != nil {
		return fmt.Errorf("failed to build relayer config: %w", err)
	}
	d.relayerCfg = relayerCfg

	registry, err := BuildRegistry(ctx, d.chains)
	if err != nil {
		return fmt.Errorf("failed to build registry: %w", err)
	}
	d.registry = registry

	if err := d.writeRelayerConfig(ctx); err != nil {
		return fmt.Errorf("failed to write relayer config: %w", err)
	}

	if err := d.writeRegistry(ctx); err != nil {
		return fmt.Errorf("failed to write registry: %w", err)
	}

	if err := d.writeWarpConfig(ctx); err != nil {
		return fmt.Errorf("failed to write warp config: %w", err)
	}

	d.Logger.Info("hyperlane initialization complete")
	return nil
}

// Deploy executes all deployment steps from the entrypoint script
func (d *Deployer) Deploy(ctx context.Context) error {
	if d.deployed {
		d.Logger.Info("deployment already completed, skipping")
		return nil
	}

	d.Logger.Info("starting hyperlane deployment")

	if err := d.deployCoreContracts(ctx); err != nil {
		return fmt.Errorf("failed to deploy core contracts: %w", err)
	}

	if err := d.deployWarpRoutes(ctx); err != nil {
		return fmt.Errorf("failed to deploy warp routes: %w", err)
	}

	d.deployed = true
	d.Logger.Info("hyperlane deployment completed successfully")
	return nil
}

func (d *Deployer) writeRelayerConfig(ctx context.Context) error {
	relayerConfigBytes, err := serializeRelayerConfig(d.relayerCfg)
	if err != nil {
		return fmt.Errorf("failed to serialize relayer config: %w", err)
	}

	if err := d.WriteFile(ctx, "relayer-config.json", relayerConfigBytes); err != nil {
		return fmt.Errorf("failed to write relayer config: %w", err)
	}
	return nil
}

func (d *Deployer) writeRegistry(ctx context.Context) error {
	for chainName, entry := range d.registry.Chains {
		metadataBytes, err := yaml.Marshal(entry.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata for %s: %w", chainName, err)
		}

		metadataPath := path.Join("registry", "chains", chainName, "metadata.yaml")
		if err := d.WriteFile(ctx, metadataPath, metadataBytes); err != nil {
			return fmt.Errorf("failed to write metadata for %s: %w", chainName, err)
		}
	}

	return nil
}

func (d *Deployer) writeWarpConfig(ctx context.Context) error {
	warpConfig := make(map[string]*WarpConfigEntry)

	for _, chain := range d.chains {
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
	if err := d.WriteFile(ctx, warpConfigPath, warpConfigBytes); err != nil {
		return fmt.Errorf("failed to write warp config: %w", err)
	}

	d.hasWarp = true
	return nil
}

// GetOnDiskSchema reconstructs a Schema by reading files written to disk
func (d *Deployer) GetOnDiskSchema(ctx context.Context) (*Schema, error) {
	relCfgBytes, err := d.ReadFile(ctx, "relayer-config.json")
	if err != nil {
		return nil, fmt.Errorf("read relayer-config.json: %w", err)
	}

	var relCfg RelayerConfig
	if err := json.Unmarshal(relCfgBytes, &relCfg); err != nil {
		return nil, fmt.Errorf("unmarshal relayer-config.json: %w", err)
	}

	reg := &Registry{Chains: make(map[string]*RegistryEntry)}
	for chainName := range relCfg.Chains {
		meta, err := d.readMetadataFromDisk(ctx, chainName)
		if err != nil {
			return nil, fmt.Errorf("read %s metadata: %w", chainName, err)
		}

		addrs, err := d.readAddressFromDisk(ctx, meta)
		if err != nil {
			return nil, fmt.Errorf("read %s addresses: %w", chainName, err)
		}

		reg.Chains[chainName] = &RegistryEntry{Metadata: meta, Addresses: addrs}
	}

	s := &Schema{RelayerConfig: &relCfg, Registry: reg}
	if b, err := d.ReadFile(ctx, filepath.Join("configs", "core-config.yaml")); err == nil {
		var core CoreConfig
		if err := yaml.Unmarshal(b, &core); err == nil {
			s.CoreConfig = &core
		}
	}

	if b, err := d.ReadFile(ctx, filepath.Join("configs", "warp-config.yaml")); err == nil {
		var warp map[string]*WarpConfigEntry
		if err := yaml.Unmarshal(b, &warp); err == nil {
			s.WarpConfig = warp
		}
	}
	return s, nil
}

// readMetadataFromDisk reads the chain metadata from the YAML file on disk.
func (d *Deployer) readMetadataFromDisk(ctx context.Context, chainName string) (ChainMetadata, error) {
	metadataPath := filepath.Join("registry", "chains", chainName, "metadata.yaml")
	bz, err := d.ReadFile(ctx, metadataPath)
	if err != nil {
		return ChainMetadata{}, fmt.Errorf("read %s addresses: %w", chainName, err)
	}

	var meta ChainMetadata
	if err := yaml.Unmarshal(bz, &meta); err != nil {
		return ChainMetadata{}, fmt.Errorf("unmarshal yaml %s: %w", metadataPath, err)
	}

	return meta, nil
}

// readAddressFromDisk reads the contract addresses from the YAML file on disk.
func (d *Deployer) readAddressFromDisk(ctx context.Context, meta ChainMetadata) (ContractAddresses, error) {
	// TODO: cosmos side not being handled yet, however ethereum addresses should be written to disk after
	// deployment.
	if meta.Protocol != "ethereum" {
		return ContractAddresses{}, nil
	}

	addressPath := filepath.Join("registry", "chains", meta.Name, "addresses.yaml")
	bz, err := d.ReadFile(ctx, addressPath)
	if err != nil {
		return ContractAddresses{}, fmt.Errorf("read %s addresses: %w", meta.Name, err)
	}

	var addresses ContractAddresses
	if err := yaml.Unmarshal(bz, &addresses); err != nil {
		return ContractAddresses{}, fmt.Errorf("unmarshal yaml %s: %w", addressPath, err)
	}

	return addresses, nil
}
