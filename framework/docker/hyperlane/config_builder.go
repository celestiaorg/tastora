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
        metadata, err := c.GetHyperlaneRegistryMetadata(ctx)
        if err != nil {
            return nil, err
        }
        relayChains[i] = metadata.Name
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

// SerializeRelayerConfig converts RelayerConfig to JSON bytes.
func SerializeRelayerConfig(config *RelayerConfig) ([]byte, error) {
	return json.MarshalIndent(config, "", "    ")
}

// Note: per-chain relayer config is provided by the ChainConfigProvider now,
// so we no longer derive signer or protocol-specific fields here.
