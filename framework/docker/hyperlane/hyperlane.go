package hyperlane

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/ethereum/go-ethereum/common"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"path"
	"path/filepath"
	"regexp"
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
	return fmt.Sprintf("hyperlane-deploy-%d-%s", d.Index, internal.SanitizeDockerResourceName(d.TestName))
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

		// TOOD: write addresses?
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
	addressPath := filepath.Join("registry", "chains", meta.Name, "addresses.yaml")
	bz, err := d.ReadFile(ctx, addressPath)
	if err != nil && meta.Name != "ethereum" {
		// NOTE: the cosmosnative side gets populated later manually, not by the deploy step.
		d.Logger.Warn("failed to read file", zap.String("path", addressPath))
		return ContractAddresses{}, nil
	}

	var addresses ContractAddresses
	if err := yaml.Unmarshal(bz, &addresses); err != nil {
		return ContractAddresses{}, fmt.Errorf("unmarshal yaml %s: %w", addressPath, err)
	}

	return addresses, nil
}

// GetEVMWarpTokenAddress reads the deployed EVM warp token address.
func (d *Deployer) GetEVMWarpTokenAddress(ctx context.Context) (common.Address, error) {
	// TODO: parse from deployment output instead of hardcoding
	addr := common.HexToAddress("0x345a583028762De4d733852c9D4f419077093A48")
	d.Logger.Info("using hardcoded EVM warp token address", zap.String("address", addr.Hex()))
	return addr, nil
}

// readWarpAddressFromChain reads the EVM warp token address by querying it with hyperlane warp read.
// After enrollment, the warp config should have the token address and remoteRouters populated.
func (d *Deployer) readWarpAddressFromChain(ctx context.Context, evmChainName string) (common.Address, error) {
	// First, list all warp routes to see what's available
	listCmd := []string{"hyperlane", "warp", "read", "--registry", path.Join(hyperlaneHomeDir, "registry")}
	if listOut, _, err := d.Exec(ctx, d.Logger, listCmd, nil); err == nil {
		d.Logger.Info("available warp routes", zap.String("output", string(listOut)))
	}

	// Try using hyperlane warp read with the token symbol from our warp config
	// The symbol should be "TIA" based on our warp-config.yaml
	cmd := []string{
		"hyperlane", "warp", "read",
		"--registry", path.Join(hyperlaneHomeDir, "registry"),
		"--symbol", "TIA",
		"--chain", evmChainName,
	}

	stdout, stderr, err := d.Exec(ctx, d.Logger, cmd, nil)
	if err != nil {
		d.Logger.Warn("hyperlane warp read with symbol failed, trying fallback",
			zap.String("chain", evmChainName),
			zap.String("stdout", string(stdout)),
			zap.String("stderr", string(stderr)),
			zap.Error(err))

		// Fallback: try reading from deployment artifacts
		return d.readWarpAddressFromDeployment(ctx, evmChainName)
	}

	output := string(stdout)
	d.Logger.Info("warp read output", zap.String("chain", evmChainName), zap.String("output", output))

	// Parse the YAML output
	var warpData map[string]interface{}
	if err := yaml.Unmarshal(stdout, &warpData); err != nil {
		d.Logger.Warn("failed to unmarshal warp read output", zap.Error(err))
		return d.readWarpAddressFromDeployment(ctx, evmChainName)
	}

	d.Logger.Info("parsed warp read data", zap.Any("data", warpData))

	// The output should have the chain name as a key
	chainData, ok := warpData[evmChainName].(map[string]interface{})
	if !ok {
		return common.Address{}, fmt.Errorf("no data for chain %s in warp read output", evmChainName)
	}

	// Look for the token address in various possible fields
	possibleFields := []string{"address", "token", "tokenAddress", "contractAddress"}
	for _, field := range possibleFields {
		if tokenAddr, ok := chainData[field].(string); ok && len(tokenAddr) == 42 && tokenAddr[:2] == "0x" {
			addr := common.HexToAddress(tokenAddr)
			d.Logger.Info("found EVM warp token address from warp read",
				zap.String("field", field),
				zap.String("address", addr.Hex()))
			return addr, nil
		}
	}

	// Fallback: try to extract any 20-byte address from the output using regex
	addrRe := regexp.MustCompile(`(?:address|token|deployed)["\s:]+["']?(0x[0-9a-fA-F]{40})["']?`)
	if matches := addrRe.FindStringSubmatch(output); len(matches) >= 2 {
		addr := common.HexToAddress(matches[1])
		d.Logger.Info("found EVM warp token address via regex",
			zap.String("address", addr.Hex()))
		return addr, nil
	}

	d.Logger.Warn("no token address found in warp read output", zap.Any("chainData", chainData))
	return common.Address{}, fmt.Errorf("no token address found in warp read output for chain %s", evmChainName)
}

// readWarpAddressFromDeployment is a fallback that reads from deployment files.
func (d *Deployer) readWarpAddressFromDeployment(ctx context.Context, chainName string) (common.Address, error) {
	// Try 1: Check if addresses were written to registry/chains/<chainName>/addresses.yaml
	// (warp deploy might update this file)
	addressPath := filepath.Join("registry", "chains", chainName, "addresses.yaml")
	if addrBytes, err := d.ReadFile(ctx, addressPath); err == nil {
		var addrs map[string]interface{}
		if err := yaml.Unmarshal(addrBytes, &addrs); err == nil {
			d.Logger.Info("registry addresses", zap.String("chain", chainName), zap.Any("addresses", addrs))

			// look for warp-related keys
			for key, val := range addrs {
				if addr, ok := val.(string); ok && len(addr) == 42 && addr[:2] == "0x" {
					// found a 20-byte address, check if it's likely the warp token
					if key == "warpToken" || key == "hypERC20" || key == "syntheticToken" {
						ethAddr := common.HexToAddress(addr)
						d.Logger.Info("found warp token in registry addresses",
							zap.String("key", key),
							zap.String("address", ethAddr.Hex()))
						return ethAddr, nil
					}
				}
			}
		}
	}

	// Try 2: Read warp-config.yaml (might have been updated by warp deploy)
	warpConfigPath := filepath.Join("configs", "warp-config.yaml")
	if warpConfigBytes, err := d.ReadFile(ctx, warpConfigPath); err == nil {
		var warpConfig map[string]map[string]interface{}
		if err := yaml.Unmarshal(warpConfigBytes, &warpConfig); err == nil {
			d.Logger.Info("warp config contents", zap.Any("config", warpConfig))

			if chainConfig, ok := warpConfig[chainName]; ok {
				// try to find the token address
				if tokenAddr, ok := chainConfig["token"].(string); ok && len(tokenAddr) == 42 && tokenAddr[:2] == "0x" {
					addr := common.HexToAddress(tokenAddr)
					d.Logger.Info("found warp token address from config", zap.String("address", addr.Hex()))
					return addr, nil
				}
				if addressOrDenom, ok := chainConfig["addressOrDenom"].(string); ok && len(addressOrDenom) == 42 && addressOrDenom[:2] == "0x" {
					addr := common.HexToAddress(addressOrDenom)
					d.Logger.Info("found warp token address via addressOrDenom", zap.String("address", addr.Hex()))
					return addr, nil
				}
			}
		}
	}

	// Try 3: Read from expected deployment file location
	// The pattern is: registry/deployments/warp_routes/ETH/<chainname>-deploy.yaml
	deploymentPath := path.Join("registry", "deployments", "warp_routes", "ETH", chainName+"-deploy.yaml")

	// use cat to read the file directly
	catCmd := []string{"cat", deploymentPath}
	if catStdout, _, catErr := d.Exec(ctx, d.Logger, catCmd, nil); catErr == nil {
		fileBytes := catStdout
		d.Logger.Info("deployment file contents",
			zap.String("file", deploymentPath),
			zap.String("contents", string(fileBytes)))

		// try to parse as YAML and look for addresses
		var data map[string]interface{}
		if err := yaml.Unmarshal(fileBytes, &data); err == nil {
			d.Logger.Info("parsed deployment file", zap.String("file", deploymentPath), zap.Any("data", data))
			if addr := extractAddressFromYAML(data, chainName); addr != (common.Address{}) {
				d.Logger.Info("found warp token in deployment file",
					zap.String("file", deploymentPath),
					zap.String("address", addr.Hex()))
				return addr, nil
			}
		} else {
			d.Logger.Warn("failed to unmarshal deployment file", zap.String("file", deploymentPath), zap.Error(err))
		}
	} else {
		d.Logger.Info("deployment file not found or could not be read", zap.String("path", deploymentPath))
	}

	return common.Address{}, fmt.Errorf("could not find EVM warp token address for chain %s in registry, config, or deployment files", chainName)
}

// extractAddressFromYAML recursively searches for a 20-byte EVM address in YAML data
func extractAddressFromYAML(data interface{}, chainName string) common.Address {
	switch v := data.(type) {
	case map[string]interface{}:
		// check if this map has the chain name as a key
		if chainData, ok := v[chainName].(map[string]interface{}); ok {
			if tokenAddr, ok := chainData["token"].(string); ok && len(tokenAddr) == 42 && tokenAddr[:2] == "0x" {
				return common.HexToAddress(tokenAddr)
			}
			if addressOrDenom, ok := chainData["addressOrDenom"].(string); ok && len(addressOrDenom) == 42 && addressOrDenom[:2] == "0x" {
				return common.HexToAddress(addressOrDenom)
			}
		}

		// recursively search all values
		for _, val := range v {
			if addr := extractAddressFromYAML(val, chainName); addr != (common.Address{}) {
				return addr
			}
		}
	case []interface{}:
		for _, item := range v {
			if addr := extractAddressFromYAML(item, chainName); addr != (common.Address{}) {
				return addr
			}
		}
	case string:
		// check if this is a 20-byte address
		if len(v) == 42 && v[:2] == "0x" {
			return common.HexToAddress(v)
		}
	}
	return common.Address{}
}

// Minimal struct to parse the warp route YAML
type warpRouteChain struct {
	RemoteRouters map[string]struct {
		Address string `yaml:"address"`
	} `yaml:"remoteRouters"`
}

// extractEVMRemoteRouterAddress takes in the raw YAML (from the registry file)
// and returns the *20-byte EVM address* extracted from the padded 32-byte address.
func extractEVMRemoteRouterAddress(yamlBytes []byte) (common.Address, error) {
	var routes map[string]warpRouteChain
	if err := yaml.Unmarshal(yamlBytes, &routes); err != nil {
		return common.Address{}, fmt.Errorf("failed to unmarshal warp route: %w", err)
	}

	if len(routes) == 0 {
		return common.Address{}, fmt.Errorf("no remoteRouters found in warp route file")
	}

	var chain warpRouteChain
	for _, v := range routes {
		chain = v
		break
	}

	// Assume a single remote router
	var padded string
	for _, rr := range chain.RemoteRouters {
		padded = rr.Address
		break
	}

	// Strip 0x
	padded = common.HexToHash(padded).Hex()
	hexStr := padded[2:] // remove "0x"

	// Must be 32 bytes (64 hex chars)
	if len(hexStr) != 64 {
		return common.Address{}, fmt.Errorf("invalid padded address length: %d", len(hexStr))
	}

	// Extract last 20 bytes (40 hex chars)
	evmHex := hexStr[24*2:] // skip first 12 bytes → 12*2 = 24 chars
	if len(evmHex) != 40 {
		return common.Address{}, fmt.Errorf("unexpected truncated EVM hex: %s", evmHex)
	}

	addrBytes, err := hex.DecodeString(evmHex)
	if err != nil {
		return common.Address{}, fmt.Errorf("hex decode failed: %w", err)
	}

	return common.BytesToAddress(addrBytes), nil
}
