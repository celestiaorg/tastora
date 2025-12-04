package hyperlane

import (
    "context"
    "crypto/ecdsa"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "path"
    "path/filepath"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	gethcrypto "github.com/ethereum/go-ethereum/crypto"
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
	hasWarp  bool
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
		{"write core config", h.writeCoreConfig},
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

		// Also write JSON variants to maximize CLI compatibility
		metadataJSON, err := json.MarshalIndent(entry.Metadata, "", "  ")
		if err == nil {
			_ = h.WriteFile(ctx, path.Join("registry", "chains", chainName, "metadata.json"), metadataJSON)
		}
		addressesJSON, err := json.MarshalIndent(entry.Addresses, "", "  ")
		if err == nil {
			_ = h.WriteFile(ctx, path.Join("registry", "chains", chainName, "addresses.json"), addressesJSON)
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

	h.hasWarp = true
	return nil
}

// deriveEthAddress derives the Ethereum address from a hex private key
func deriveEthAddress(hexKey string) (string, error) {
	k := hexKey
	if len(k) >= 2 && (k[:2] == "0x" || k[:2] == "0X") {
		k = k[2:]
	}
	b, err := hex.DecodeString(k)
	if err != nil {
		return "", fmt.Errorf("decode privkey: %w", err)
	}
	priv, err := gethcrypto.ToECDSA(b)
	if err != nil {
		return "", fmt.Errorf("to ecdsa: %w", err)
	}
	pub := priv.Public().(*ecdsa.PublicKey)
	addr := gethcrypto.PubkeyToAddress(*pub)
	return addr.Hex(), nil
}

// GetOnDiskSchema reconstructs a Schema by reading files written to disk
func (h *Deployer) GetOnDiskSchema(ctx context.Context) (*Schema, error) {
    // Read relayer-config.json
    relCfgBytes, err := h.ReadFile(ctx, "relayer-config.json")
    if err != nil {
        return nil, fmt.Errorf("read relayer-config.json: %w", err)
    }
    var relCfg RelayerConfig
    if err := json.Unmarshal(relCfgBytes, &relCfg); err != nil {
        return nil, fmt.Errorf("unmarshal relayer-config.json: %w", err)
    }

    reg := &Registry{Chains: make(map[string]*RegistryEntry)}
    for chainName := range relCfg.Chains {
        // metadata yaml or json
        var meta ChainMetadata
        if err := h.readYAMLOrJSON(ctx,
            filepath.Join("registry", "chains", chainName, "metadata.yaml"),
            filepath.Join("registry", "chains", chainName, "metadata.json"), &meta); err != nil {
            return nil, fmt.Errorf("read %s metadata: %w", chainName, err)
        }
        var addrs ContractAddresses
        if err := h.readYAMLOrJSON(ctx,
            filepath.Join("registry", "chains", chainName, "addresses.yaml"),
            filepath.Join("registry", "chains", chainName, "addresses.json"), &addrs); err != nil {
            return nil, fmt.Errorf("read %s addresses: %w", chainName, err)
        }
        reg.Chains[chainName] = &RegistryEntry{Metadata: meta, Addresses: addrs}
    }

    return &Schema{RelayerConfig: &relCfg, Registry: reg}, nil
}

func (h *Deployer) readYAMLOrJSON(ctx context.Context, yamlPath, jsonPath string, out any) error {
    if b, err := h.ReadFile(ctx, yamlPath); err == nil {
        if err := yaml.Unmarshal(b, out); err != nil {
            return fmt.Errorf("unmarshal yaml %s: %w", yamlPath, err)
        }
        return nil
    }
    if b, err := h.ReadFile(ctx, jsonPath); err == nil {
        if err := json.Unmarshal(b, out); err != nil {
            return fmt.Errorf("unmarshal json %s: %w", jsonPath, err)
        }
        return nil
    }
    return fmt.Errorf("neither %s nor %s could be read", yamlPath, jsonPath)
}
