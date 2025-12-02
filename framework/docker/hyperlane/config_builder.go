package hyperlane

import (
	"encoding/json"
	"strings"
)

// BuildRelayerConfig generates a RelayerConfig from chain config providers.
func BuildRelayerConfig(chains []HyperlaneChainConfigProvider, relayChains []string) (*RelayerConfig, error) {
	config := &RelayerConfig{
		Chains:                  make(map[string]ChainConfig),
		DefaultRpcConsensusType: "fallback",
		RelayChains:             strings.Join(relayChains, ","),
	}

	for _, chain := range chains {
		metadata := chain.GetHyperlaneChainMetadata()

		chainConfig := ChainConfig{
			ChainID:     metadata.ChainID,
			DomainID:    metadata.DomainID,
			Name:        metadata.Name,
			DisplayName: metadata.DisplayName,
			Protocol:    metadata.Protocol,
			IsTestnet:   metadata.IsTestnet,
			NativeToken: buildNativeTokenConfig(metadata.NativeToken),
			Blocks:      buildBlockConfigForRelayer(metadata.BlockConfig),
			Signer:      buildSignerConfig(metadata),
			RPCUrls:     buildURLItemsConfig(metadata.RPCURLs),
		}

		switch metadata.Protocol {
		case "ethereum":
			applyEVMFields(&chainConfig, metadata)
		case "cosmosnative":
			applyCosmosFields(&chainConfig, metadata)
		}

		config.Chains[metadata.Name] = chainConfig
	}

	return config, nil
}

// SerializeRelayerConfig converts RelayerConfig to JSON bytes.
func SerializeRelayerConfig(config *RelayerConfig) ([]byte, error) {
	return json.MarshalIndent(config, "", "    ")
}

func buildSignerConfig(meta ChainMetadata) *SignerConfig {
	signer := &SignerConfig{
		Key: meta.SignerKey,
	}

	switch meta.Protocol {
	case "ethereum":
		signer.Type = "hexKey"
	case "cosmosnative":
		signer.Type = "cosmosKey"
		signer.Prefix = meta.Bech32Prefix
	}

	return signer
}

func applyEVMFields(config *ChainConfig, meta ChainMetadata) {
	if meta.CoreContracts != nil {
		config.Mailbox = meta.CoreContracts.Mailbox
		config.InterchainSecurityModule = meta.CoreContracts.InterchainSecurityModule
		config.InterchainGasPaymaster = meta.CoreContracts.InterchainGasPaymaster
		config.MerkleTreeHook = meta.CoreContracts.MerkleTreeHook
		config.ProxyAdmin = meta.CoreContracts.ProxyAdmin
		config.ValidatorAnnounce = meta.CoreContracts.ValidatorAnnounce
		config.AggregationHook = meta.CoreContracts.AggregationHook
		config.DomainRoutingIsm = meta.CoreContracts.DomainRoutingIsm
		config.FallbackRoutingHook = meta.CoreContracts.FallbackRoutingHook
		config.ProtocolFee = meta.CoreContracts.ProtocolFee
		config.StorageGasOracle = meta.CoreContracts.StorageGasOracle
		config.TestRecipient = meta.CoreContracts.TestRecipient
	}
}

func applyCosmosFields(config *ChainConfig, meta ChainMetadata) {
	config.Bech32Prefix = meta.Bech32Prefix
	config.CanonicalAsset = meta.CanonicalAsset
	config.ContractAddressBytes = meta.ContractAddressBytes
	config.Slip44 = meta.Slip44

	config.RESTUrls = buildURLItemsConfig(meta.RESTURLs)
	config.GRPCUrls = buildURLItemsConfig(meta.GRPCURLs)

	if meta.GasPrice != nil {
		config.GasPrice = &GasPrice{
			Denom:  meta.GasPrice.Denom,
			Amount: meta.GasPrice.Amount,
		}
	}

	if meta.IndexConfig != nil {
		config.Index = &IndexConfig{
			From:  meta.IndexConfig.From,
			Chunk: meta.IndexConfig.Chunk,
		}
	}
}

func buildNativeTokenConfig(token TokenMetadata) *NativeToken {
	nativeToken := &NativeToken{
		Name:     token.Name,
		Symbol:   token.Symbol,
		Decimals: token.Decimals,
	}

	if token.Denom != "" {
		nativeToken.Denom = token.Denom
	}

	return nativeToken
}

func buildBlockConfigForRelayer(block *BlockMetadata) *BlockConfig {
	if block == nil {
		return nil
	}

	return &BlockConfig{
		Confirmations:     block.Confirmations,
		EstimateBlockTime: block.EstimateBlockTime,
		ReorgPeriod:       block.ReorgPeriod,
	}
}

func buildURLItemsConfig(urls []string) []URLItem {
	items := make([]URLItem, len(urls))
	for i, url := range urls {
		items[i] = URLItem{HTTP: url}
	}
	return items
}
