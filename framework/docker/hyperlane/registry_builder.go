package hyperlane

import (
	"context"
)

// BuildRegistry generates a Registry from chain config providers.
func BuildRegistry(ctx context.Context, chains []ChainConfigProvider) (*Registry, error) {
	reg := &Registry{Chains: make(map[string]*RegistryEntry)}

	for _, chain := range chains {
		entry, err := chain.GetHyperlaneRegistryEntry(ctx)
		if err != nil {
			return nil, err
		}
		e := entry
		reg.Chains[entry.Metadata.Name] = &e
	}

	return reg, nil
}
