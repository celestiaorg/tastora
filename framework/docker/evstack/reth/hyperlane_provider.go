package reth

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
)

var _ hyperlane.ChainConfigProvider = (*Node)(nil)

func (n *Node) GetHyperlaneRegistryEntry(ctx context.Context) (hyperlane.RegistryEntry, error) {
	networkInfo, err := n.GetNetworkInfo(ctx)
	if err != nil {
		return hyperlane.RegistryEntry{}, fmt.Errorf("get network info: %w", err)
	}

	rpcURL := fmt.Sprintf("http://%s", networkInfo.Internal.RPCAddress())

	meta := hyperlane.ChainMetadata{
		ChainID:     1234, // hard coded to the value in the default genesis.
		DomainID:    1234,
		Name:        "rethlocal",
		DisplayName: "Reth",
		Protocol:    "ethereum",
		IsTestnet:   true,
		NativeToken: hyperlane.NativeToken{
			Name:     "Ether",
			Symbol:   "ETH",
			Decimals: 18,
		},
		RpcURLs: []hyperlane.Endpoint{
			{HTTP: rpcURL},
		},
		Blocks: &hyperlane.BlockConfig{
			Confirmations:     1,
			EstimateBlockTime: 3,
			ReorgPeriod:       0,
		},
	}

	// ref: https://github.com/celestiaorg/celestia-zkevm/blob/b00d6efd0498a44c500e6b2097744167ff45f82b/hyperlane/registry/chains/rethlocal/addresses.yaml#L1
	addrs := hyperlane.ContractAddresses{
		Mailbox:                  "0xb1c938F5BA4B3593377F399e12175e8db0C787Ff",
		InterchainSecurityModule: "0xa05915fD6E32A1AA7E67d800164CaCB12487142d",
		InterchainGasPaymaster:   "0x1D957dA7A6988f5a9d2D2454637B4B7fea0Aeea5",
		MerkleTreeHook:           "0xFCb1d485ef46344029D9E8A7925925e146B3430E",
		ProxyAdmin:               "0x7e7aD18Adc99b94d4c728fDf13D4dE97B926A0D8",
		ValidatorAnnounce:        "0x79ec7bF05AF122D3782934d4Fb94eE32f0C01c97",
		AggregationHook:          "0xe53275A1FcA119e1c5eeB32E7a72e54835A63936",
		DomainRoutingIsm:         "0xE2c1756b8825C54638f98425c113b51730cc47f6",
		FallbackRoutingHook:      "0xE2c1756b8825C54638f98425c113b51730cc47f6",
		ProtocolFee:              "0x8A93d247134d91e0de6f96547cB0204e5BE8e5D8",
		StorageGasOracle:         "0x457cCf29090fe5A24c19c1bc95F492168C0EaFdb",
		TestRecipient:            "0xd7958B336f0019081Ad2279B2B7B7c3f744Bce0a",
		// factory and router addresses
		DomainRoutingIsmFactory:                    "0xE2c1756b8825C54638f98425c113b51730cc47f6",
		InterchainAccountIsm:                       "0x9F098AE0AC3B7F75F0B3126f471E5F592b47F300",
		InterchainAccountRouter:                    "0x4dc4E8bf5D0390C95Af9AFEb1e9c9927c4dB83e7",
		StaticAggregationHookFactory:               "0xe53275A1FcA119e1c5eeB32E7a72e54835A63936",
		StaticAggregationIsmFactory:                "0x25CdBD2bf399341F8FEe22eCdB06682AC81fDC37",
		StaticMerkleRootMultisigIsmFactory:         "0x2854CFaC53FCaB6C95E28de8C91B96a31f0af8DD",
		StaticMerkleRootWeightedMultisigIsmFactory: "0x94B9B5bD518109dB400ADC62ab2022D2F0008ff7",
		StaticMessageIdMultisigIsmFactory:          "0xCb1DC4aF63CFdaa4b9BFF307A8Dd4dC11B197E8f",
		StaticMessageIdWeightedMultisigIsmFactory:  "0x70Ac5980099d71F4cb561bbc0fcfEf08AA6279ec",
	}

	return hyperlane.RegistryEntry{Metadata: meta, Addresses: addrs}, nil

}

func (n *Node) GetHyperlaneRelayerChainConfig(ctx context.Context) (hyperlane.RelayerChainConfig, error) {
	entry, err := n.GetHyperlaneRegistryEntry(ctx)
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
	}

	// NOTE: this key is hard coded for testing purposes and corresponds to the 0xaF9053bB6c4346381C77C2FeD279B17ABAfCDf4d address in the genesis.
	cfg.Signer = &hyperlane.SignerConfig{Key: "0x82bfcfadbf1712f6550d8d2c00a39f05b33ec78939d0167be2a737d691f33a6a", Type: "hexKey"}

	// derive core contract addresses from registry entry
	cfg.Mailbox = entry.Addresses.Mailbox
	cfg.InterchainSecurityModule = entry.Addresses.InterchainSecurityModule
	cfg.InterchainGasPaymaster = entry.Addresses.InterchainGasPaymaster
	cfg.MerkleTreeHook = entry.Addresses.MerkleTreeHook
	cfg.ProxyAdmin = entry.Addresses.ProxyAdmin
	cfg.ValidatorAnnounce = entry.Addresses.ValidatorAnnounce
	cfg.AggregationHook = entry.Addresses.AggregationHook
	cfg.DomainRoutingIsm = entry.Addresses.DomainRoutingIsm
	cfg.FallbackRoutingHook = entry.Addresses.FallbackRoutingHook
	cfg.ProtocolFee = entry.Addresses.ProtocolFee
	cfg.StorageGasOracle = entry.Addresses.StorageGasOracle
	cfg.TestRecipient = entry.Addresses.TestRecipient

	return cfg, nil
}

func (n *Node) GetHyperlaneWarpConfigEntry(ctx context.Context) (*hyperlane.WarpConfigEntry, error) {
	cfg, err := n.GetHyperlaneRelayerChainConfig(ctx)
	if err != nil {
		return nil, err
	}

	return &hyperlane.WarpConfigEntry{
		Type:                     "synthetic",
		Owner:                    "0xaF9053bB6c4346381C77C2FeD279B17ABAfCDf4d",
		Mailbox:                  cfg.Mailbox,
		InterchainSecurityModule: cfg.InterchainSecurityModule,
		Name:                     "wTIA",
		Symbol:                   "TIA",
		Decimals:                 6,
	}, nil
}
