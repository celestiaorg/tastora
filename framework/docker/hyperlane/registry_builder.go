package hyperlane

import (
	"context"
	"gopkg.in/yaml.v3"
)

// BuildRegistry generates a Registry from chain config providers.
func BuildRegistry(ctx context.Context, chains []ChainConfigProvider) (*Registry, error) {
	reg := &Registry{
		Chains: make(map[string]*ChainEntry),
		Deployments: Deployments{
			Core: make(map[string][]CoreDeployment),
		},
	}

	for _, chain := range chains {
        metadata, err := chain.GetHyperlaneRegistryMetadata(ctx)
        if err != nil {
            return nil, err
        }

        chainEntry := &ChainEntry{
            Metadata:  metadata,
            Addresses: buildChainAddresses(metadata),
        }

		reg.Chains[metadata.Name] = chainEntry

        if metadata.CoreContracts != nil && metadata.CoreContracts.Mailbox != "" {
            reg.Deployments.Core[metadata.Name] = []CoreDeployment{
                {
                    Addresses: buildCoreAddressesMap(metadata.CoreContracts),
                },
            }
        }
	}

	return reg, nil
}

// SerializeRegistry converts Registry to YAML bytes.
func SerializeRegistry(reg *Registry) ([]byte, error) {
	return yaml.Marshal(reg)
}

func buildChainAddresses(meta ChainMetadata) ChainAddresses {
	if meta.CoreContracts == nil {
		return ChainAddresses{}
	}

	addresses := ChainAddresses{}

	if meta.CoreContracts.Mailbox != "" {
		addresses["mailbox"] = meta.CoreContracts.Mailbox
	}
	if meta.CoreContracts.InterchainSecurityModule != "" {
		addresses["interchainSecurityModule"] = meta.CoreContracts.InterchainSecurityModule
	}
	if meta.CoreContracts.InterchainGasPaymaster != "" {
		addresses["interchainGasPaymaster"] = meta.CoreContracts.InterchainGasPaymaster
	}
	if meta.CoreContracts.MerkleTreeHook != "" {
		addresses["merkleTreeHook"] = meta.CoreContracts.MerkleTreeHook
	}
	if meta.CoreContracts.ProxyAdmin != "" {
		addresses["proxyAdmin"] = meta.CoreContracts.ProxyAdmin
	}
	if meta.CoreContracts.ValidatorAnnounce != "" {
		addresses["validatorAnnounce"] = meta.CoreContracts.ValidatorAnnounce
	}
	if meta.CoreContracts.AggregationHook != "" {
		addresses["aggregationHook"] = meta.CoreContracts.AggregationHook
	}
	if meta.CoreContracts.DomainRoutingIsm != "" {
		addresses["domainRoutingIsm"] = meta.CoreContracts.DomainRoutingIsm
	}
	if meta.CoreContracts.FallbackRoutingHook != "" {
		addresses["fallbackRoutingHook"] = meta.CoreContracts.FallbackRoutingHook
	}
	if meta.CoreContracts.ProtocolFee != "" {
		addresses["protocolFee"] = meta.CoreContracts.ProtocolFee
	}
	if meta.CoreContracts.StorageGasOracle != "" {
		addresses["storageGasOracle"] = meta.CoreContracts.StorageGasOracle
	}
	if meta.CoreContracts.TestRecipient != "" {
		addresses["testRecipient"] = meta.CoreContracts.TestRecipient
	}

	return addresses
}

func buildCoreAddressesMap(contracts *CoreContractAddresses) map[string]string {
	addresses := make(map[string]string)

	if contracts.Mailbox != "" {
		addresses["mailbox"] = contracts.Mailbox
	}
	if contracts.InterchainSecurityModule != "" {
		addresses["interchainSecurityModule"] = contracts.InterchainSecurityModule
	}
	if contracts.InterchainGasPaymaster != "" {
		addresses["interchainGasPaymaster"] = contracts.InterchainGasPaymaster
	}
	if contracts.MerkleTreeHook != "" {
		addresses["merkleTreeHook"] = contracts.MerkleTreeHook
	}
	if contracts.ProxyAdmin != "" {
		addresses["proxyAdmin"] = contracts.ProxyAdmin
	}
	if contracts.ValidatorAnnounce != "" {
		addresses["validatorAnnounce"] = contracts.ValidatorAnnounce
	}

	return addresses
}
