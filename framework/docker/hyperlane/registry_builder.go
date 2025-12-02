package hyperlane

import (
	"github.com/celestiaorg/tastora/framework/docker/hyperlane/registry"
	"gopkg.in/yaml.v3"
)

// BuildRegistry generates a Registry from chain config providers.
func BuildRegistry(chains []ChainConfigProvider) (*registry.Registry, error) {
	reg := &registry.Registry{
		Chains: make(map[string]*registry.ChainEntry),
		Deployments: registry.Deployments{
			Core: make(map[string][]registry.CoreDeployment),
		},
	}

	for _, chain := range chains {
		metadata := chain.GetHyperlaneChainMetadata()

		chainEntry := &registry.ChainEntry{
			Name:      metadata.Name,
			Metadata:  buildChainMetadata(metadata),
			Addresses: buildChainAddresses(metadata),
		}

		reg.Chains[metadata.Name] = chainEntry

		if metadata.CoreContracts != nil && metadata.CoreContracts.Mailbox != "" {
			reg.Deployments.Core[metadata.Name] = []registry.CoreDeployment{
				{
					Chain:     metadata.Name,
					Addresses: buildCoreAddressesMap(metadata.CoreContracts),
				},
			}
		}
	}

	return reg, nil
}

// SerializeRegistry converts Registry to YAML bytes.
func SerializeRegistry(reg *registry.Registry) ([]byte, error) {
	return yaml.Marshal(reg)
}

func buildChainMetadata(meta ChainMetadata) registry.ChainMetadata {
	chainMeta := registry.ChainMetadata{
		ChainID:              meta.ChainID,
		DomainID:             uint32(meta.DomainID),
		Name:                 meta.Name,
		DisplayName:          meta.DisplayName,
		Protocol:             meta.Protocol,
		IsTestnet:            meta.IsTestnet,
		NativeToken:          buildNativeToken(meta.NativeToken),
		RpcURLs:              buildEndpoints(meta.RPCURLs),
		RestURLs:             buildEndpoints(meta.RESTURLs),
		GrpcURLs:             buildEndpoints(meta.GRPCURLs),
		Bech32Prefix:         meta.Bech32Prefix,
		CanonicalAsset:       meta.CanonicalAsset,
		ContractAddressBytes: meta.ContractAddressBytes,
		Slip44:               meta.Slip44,
	}

	if meta.BlockConfig != nil {
		chainMeta.Blocks = &registry.BlockConfig{
			Confirmations:     meta.BlockConfig.Confirmations,
			EstimateBlockTime: meta.BlockConfig.EstimateBlockTime,
			ReorgPeriod:       meta.BlockConfig.ReorgPeriod,
		}
	}

	if meta.GasPrice != nil {
		chainMeta.GasPrice = &registry.GasPrice{
			Denom:  meta.GasPrice.Denom,
			Amount: meta.GasPrice.Amount,
		}
	}

	return chainMeta
}

func buildNativeToken(token TokenMetadata) registry.NativeToken {
	return registry.NativeToken{
		Name:     token.Name,
		Symbol:   token.Symbol,
		Decimals: uint8(token.Decimals),
		Denom:    token.Denom,
	}
}

func buildEndpoints(urls []string) []registry.Endpoint {
	endpoints := make([]registry.Endpoint, len(urls))
	for i, url := range urls {
		endpoints[i] = registry.Endpoint{HTTP: url}
	}
	return endpoints
}

func buildChainAddresses(meta ChainMetadata) registry.ChainAddresses {
	if meta.CoreContracts == nil {
		return registry.ChainAddresses{}
	}

	addresses := registry.ChainAddresses{}

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
