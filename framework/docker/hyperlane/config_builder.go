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
		metadata, err := c.GetHyperlaneChainMetadata(ctx)
		if err != nil {
			return nil, err
		}
		relayChains[i] = metadata.Name
	}

	config := &RelayerConfig{
		Chains:                  make(map[string]ChainConfig),
		DefaultRpcConsensusType: "fallback",
		RelayChains:             strings.Join(relayChains, ","),
	}

	for _, chain := range chains {
		metadata, err := chain.GetHyperlaneChainMetadata(ctx)
		if err != nil {
			return nil, err
		}

		chainConfig := ChainConfig{
			ChainID:     metadata.ChainID,
			DomainID:    metadata.DomainID,
			Name:        metadata.Name,
			DisplayName: metadata.DisplayName,
			Protocol:    metadata.Protocol,
			IsTestnet:   metadata.IsTestnet,
			NativeToken: &metadata.NativeToken,
			Blocks:      metadata.Blocks,
			Signer:      buildSignerConfig(metadata),
			RpcURLs:     metadata.RpcURLs,
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
	if meta.CoreContracts == nil {
		return
	}

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

func applyCosmosFields(config *ChainConfig, meta ChainMetadata) {
	config.Bech32Prefix = meta.Bech32Prefix
	config.CanonicalAsset = meta.CanonicalAsset
	config.ContractAddressBytes = meta.ContractAddressBytes
	config.Slip44 = meta.Slip44

	config.RestURLs = meta.RestURLs
	config.GrpcURLs = meta.GrpcURLs

	config.GasPrice = meta.GasPrice
	config.Index = meta.IndexConfig
}
