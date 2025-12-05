package hyperlane

import (
	"context"
	"encoding/json"
	"strings"
)

// BuildRelayerConfig generates a RelayerConfig from chain config providers.
func BuildRelayerConfig(ctx context.Context, chains []ChainConfigProvider) (*RelayerConfig, error) {
	relayChains := make([]string, len(chains))
	for i, c := range chains {
		entry, err := c.GetHyperlaneRegistryEntry(ctx)
		if err != nil {
			return nil, err
		}
		relayChains[i] = entry.Metadata.Name
	}

	config := &RelayerConfig{
		Chains:                  make(map[string]RelayerChainConfig),
		DefaultRpcConsensusType: "fallback",
		RelayChains:             strings.Join(relayChains, ","),
	}

	for _, chain := range chains {
		// Build per-chain relayer config directly from provider.
		perChain, err := chain.GetHyperlaneRelayerChainConfig(ctx)
		if err != nil {
			return nil, err
		}
		config.Chains[perChain.Name] = perChain
	}

	return config, nil
}

// serializeRelayerConfig converts RelayerConfig to JSON bytes.
func serializeRelayerConfig(config *RelayerConfig) ([]byte, error) {
	return json.MarshalIndent(config, "", "    ")
}
