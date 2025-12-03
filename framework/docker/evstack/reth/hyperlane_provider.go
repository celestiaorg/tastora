package reth

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
)

var _ hyperlane.ChainConfigProvider = (*Node)(nil)

func (n *Node) GetHyperlaneRegistryMetadata(ctx context.Context) (hyperlane.ChainMetadata, error) {
	networkInfo, err := n.GetNetworkInfo(ctx)
	if err != nil {
		return hyperlane.ChainMetadata{}, fmt.Errorf("get network info: %w", err)
	}

	rpcURL := fmt.Sprintf("http://%s", networkInfo.Internal.RPCAddress())

	return hyperlane.ChainMetadata{
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
		// NOTE: this key is hard coded for testing purposes and corresponds to the 0xaF9053bB6c4346381C77C2FeD279B17ABAfCDf4d
		// in the default genesis used.
		SignerKey: "0x82bfcfadbf1712f6550d8d2c00a39f05b33ec78939d0167be2a737d691f33a6a",
		CoreContracts: &hyperlane.CoreContractAddresses{
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
		},
	}, nil

}

func (n *Node) GetHyperlaneRelayerChainConfig(ctx context.Context) (hyperlane.RelayerChainConfig, error) {
	meta, err := n.GetHyperlaneRegistryMetadata(ctx)
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
	}

	cfg.Signer = &hyperlane.SignerConfig{
		Key:  meta.SignerKey,
		Type: "hexKey",
		// no bech32 prefix required for evm.
	}

	// protocol-specific fields
	cfg.Mailbox = meta.CoreContracts.Mailbox
	cfg.InterchainSecurityModule = meta.CoreContracts.InterchainSecurityModule
	cfg.InterchainGasPaymaster = meta.CoreContracts.InterchainGasPaymaster
	cfg.MerkleTreeHook = meta.CoreContracts.MerkleTreeHook
	cfg.ProxyAdmin = meta.CoreContracts.ProxyAdmin
	cfg.ValidatorAnnounce = meta.CoreContracts.ValidatorAnnounce
	cfg.AggregationHook = meta.CoreContracts.AggregationHook
	cfg.DomainRoutingIsm = meta.CoreContracts.DomainRoutingIsm
	cfg.FallbackRoutingHook = meta.CoreContracts.FallbackRoutingHook
	cfg.ProtocolFee = meta.CoreContracts.ProtocolFee
	cfg.StorageGasOracle = meta.CoreContracts.StorageGasOracle
	cfg.TestRecipient = meta.CoreContracts.TestRecipient

	return cfg, nil
}
