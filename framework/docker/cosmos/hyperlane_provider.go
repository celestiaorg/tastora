package cosmos

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
)

var _ hyperlane.ChainConfigProvider = (*Chain)(nil)

// GetHyperlaneRegistryMetadata returns the metadata required for this chain in a hyperlane registry.
func (c *Chain) GetHyperlaneRegistryMetadata(ctx context.Context) (hyperlane.ChainMetadata, error) {
	networkInfo, err := c.GetNetworkInfo(ctx)
	if err != nil {
		return hyperlane.ChainMetadata{}, err
	}

	signerKey, err := c.getFaucetPrivateKeyHex()
	if err != nil {
		return hyperlane.ChainMetadata{}, fmt.Errorf("failed to get faucet private key: %w", err)
	}

	return hyperlane.ChainMetadata{
		ChainID:     c.GetChainID(),
		DomainID:    69420,
		Name:        c.Config.Name,
		DisplayName: c.Config.Name,
		Protocol:    "cosmosnative",
		IsTestnet:   true,
		NativeToken: hyperlane.NativeToken{
			Name:     "TIA",
			Symbol:   "TIA",
			Decimals: 6,
			Denom:    c.Config.Denom,
		},
		RpcURLs: []hyperlane.Endpoint{
			{
				HTTP: fmt.Sprintf("http://%s", networkInfo.Internal.RPCAddress()),
			},
		},
		RestURLs: []hyperlane.Endpoint{
			{
				HTTP: fmt.Sprintf("http://%s", networkInfo.Internal.APIAddress()),
			},
		},
		Blocks: &hyperlane.BlockConfig{
			Confirmations:     1,
			EstimateBlockTime: 6,
			ReorgPeriod:       1,
		},
		TechnicalStack:       "other",
		Bech32Prefix:         c.Config.Bech32Prefix,
		CanonicalAsset:       c.Config.Denom,
		ContractAddressBytes: 0,
		GasPrice: &hyperlane.GasPrice{
			Denom:  c.Config.Denom,
			Amount: c.Config.GasPrices,
		},
		Slip44:    118,
		SignerKey: signerKey,
		CoreContracts: &hyperlane.CoreContractAddresses{
			Mailbox:                  "0x68797065726c616e650000000000000000000000000000000000000000000000",
			InterchainSecurityModule: "0x726f757465725f69736d00000000000000000000000000000000000000000000",
			InterchainGasPaymaster:   "0x726f757465725f706f73745f6469737061746368000000000000000000000000",
			MerkleTreeHook:           "0x726f757465725f706f73745f6469737061746368000000030000000000000001",
			ValidatorAnnounce:        "0x68797065726c616e650000000000000000000000000000000000000000000000",
		},
		IndexConfig: &hyperlane.IndexConfig{
			From:  1150,
			Chunk: 10,
		},
	}, nil
}

// GetHyperlaneRelayerChainConfig returns the contents required for this chain in a relayer configuration file.
func (c *Chain) GetHyperlaneRelayerChainConfig(ctx context.Context) (hyperlane.RelayerChainConfig, error) {
	meta, err := c.GetHyperlaneRegistryMetadata(ctx)
	if err != nil {
		return hyperlane.RelayerChainConfig{}, err
	}

	cfg := hyperlane.RelayerChainConfig{
		Name:        meta.Name,
		ChainID:     meta.ChainID,
		DomainID:    meta.DomainID,
		DisplayName: meta.DisplayName,
		Protocol:    meta.Protocol,
		IsTestnet:   meta.IsTestnet,
		NativeToken: &meta.NativeToken,
		Blocks:      meta.Blocks,
		RpcURLs:     meta.RpcURLs,
		RestURLs:    meta.RestURLs,
		GrpcURLs:    meta.GrpcURLs,
	}

	// signer derives from protocol and metadata
	cfg.Signer = &hyperlane.SignerConfig{
		Key:    meta.SignerKey,
		Type:   "hexKey",
		Prefix: meta.Bech32Prefix,
	}

	// cosmos-specific fields
	cfg.Bech32Prefix = meta.Bech32Prefix
	cfg.CanonicalAsset = meta.CanonicalAsset
	cfg.ContractAddressBytes = meta.ContractAddressBytes
	cfg.GasPrice = meta.GasPrice
	cfg.Index = meta.IndexConfig
	cfg.Slip44 = meta.Slip44

	return cfg, nil
}
