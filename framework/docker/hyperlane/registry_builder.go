package hyperlane

import (
	"context"
	"gopkg.in/yaml.v3"
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

// SerializeRegistry converts Registry to YAML bytes.
func SerializeRegistry(reg *Registry) ([]byte, error) {
	return yaml.Marshal(reg)
}

// no helper required; addresses are provided by the chain provider as part of the entry
