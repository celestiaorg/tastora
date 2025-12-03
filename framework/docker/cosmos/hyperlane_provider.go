package cosmos

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
)

var _ hyperlane.ChainConfigProvider = (*Chain)(nil)

// GetHyperlaneRegistryEntry returns the registry entry (metadata + addresses) for this chain.
func (c *Chain) GetHyperlaneRegistryEntry(ctx context.Context) (hyperlane.RegistryEntry, error) {
	networkInfo, err := c.GetNetworkInfo(ctx)
	if err != nil {
		return hyperlane.RegistryEntry{}, err
	}

	meta := hyperlane.ChainMetadata{
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
		Slip44: 118,
	}
	return hyperlane.RegistryEntry{Metadata: meta, Addresses: hyperlane.ContractAddresses{}}, nil
}

// GetHyperlaneRelayerChainConfig returns the contents required for this chain in a relayer configuration file.
func (c *Chain) GetHyperlaneRelayerChainConfig(ctx context.Context) (hyperlane.RelayerChainConfig, error) {
	entry, err := c.GetHyperlaneRegistryEntry(ctx)
	if err != nil {
		return hyperlane.RelayerChainConfig{}, err
	}

	cfg := hyperlane.RelayerChainConfig{
		Name:        entry.Metadata.Name,
		ChainID:     entry.Metadata.ChainID,
		DomainID:    entry.Metadata.DomainID,
		DisplayName: entry.Metadata.DisplayName,
		Protocol:    entry.Metadata.Protocol,
		IsTestnet:   entry.Metadata.IsTestnet,
		NativeToken: &entry.Metadata.NativeToken,
		Blocks:      entry.Metadata.Blocks,
		RpcURLs:     entry.Metadata.RpcURLs,
		RestURLs:    entry.Metadata.RestURLs,
		GrpcURLs:    entry.Metadata.GrpcURLs,
	}

	// signer derives from chain faucet/private key
	signerKey, err := c.getFaucetPrivateKeyHex()
	if err != nil {
		return hyperlane.RelayerChainConfig{}, fmt.Errorf("failed to get faucet private key: %w", err)
	}
	cfg.Signer = &hyperlane.SignerConfig{Key: signerKey, Type: "cosmosKey", Prefix: entry.Metadata.Bech32Prefix}

	// cosmos-specific fields
	cfg.Bech32Prefix = entry.Metadata.Bech32Prefix
	cfg.CanonicalAsset = entry.Metadata.CanonicalAsset
	cfg.ContractAddressBytes = entry.Metadata.ContractAddressBytes
	cfg.GasPrice = entry.Metadata.GasPrice
	cfg.Slip44 = entry.Metadata.Slip44

	// set index configuration in relayer config
	cfg.Index = &hyperlane.IndexConfig{From: 1150, Chunk: 10}

	return cfg, nil
}

func (c *Chain) GetHyperlaneWarpConfigEntry(ctx context.Context) (*hyperlane.WarpConfigEntry, error) {
	return nil, nil
}
