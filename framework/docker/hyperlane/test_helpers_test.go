package hyperlane

import "context"

// Shared mock implementing ChainConfigProvider with convenient constructors.
type mockChainConfigProvider struct {
	entry       RegistryEntry
	relayerConf RelayerChainConfig
}

func (m *mockChainConfigProvider) GetHyperlaneRegistryEntry(context.Context) (RegistryEntry, error) {
	return m.entry, nil
}

func (m *mockChainConfigProvider) GetHyperlaneRelayerChainConfig(context.Context) (RelayerChainConfig, error) {
	return m.relayerConf, nil
}

// newEVMProvider returns a default EVM provider for "rethlocal" suitable for both
// registry and relayer tests.
func newEVMProvider() *mockChainConfigProvider {
	meta := ChainMetadata{
		Name:        "rethlocal",
		ChainID:     1234,
		DomainID:    1234,
		DisplayName: "Rethlocal",
		Protocol:    "ethereum",
		IsTestnet:   true,
		RpcURLs:     []Endpoint{{HTTP: "http://reth:8545"}},
		NativeToken: NativeToken{Name: "Ether", Symbol: "ETH", Decimals: 18},
		Blocks:      &BlockConfig{Confirmations: 1, EstimateBlockTime: 3, ReorgPeriod: 0},
	}
	relayer := RelayerChainConfig{
		Name:                     "rethlocal",
		ChainID:                  1234,
		DomainID:                 1234,
		DisplayName:              "Rethlocal",
		Protocol:                 "ethereum",
		IsTestnet:                true,
		NativeToken:              &NativeToken{Name: "Ether", Symbol: "ETH", Decimals: 18},
		Blocks:                   &BlockConfig{Confirmations: 1, EstimateBlockTime: 3, ReorgPeriod: 0},
		Signer:                   &SignerConfig{Type: "hexKey", Key: "0x123"},
		RpcURLs:                  []Endpoint{{HTTP: "http://reth:8545"}},
		Mailbox:                  "0xb1c938F5BA4B3593377F399e12175e8db0C787Ff",
		InterchainSecurityModule: "0xa05915fD6E32A1AA7E67d800164CaCB12487142d",
	}
	return &mockChainConfigProvider{entry: RegistryEntry{Metadata: meta}, relayerConf: relayer}
}

// newEVMProviderWithCore is like newEVMProvider but includes core contracts in metadata
// for registry address/deployments tests.
func newEVMProviderWithCore() *mockChainConfigProvider {
	m := newEVMProvider()
	m.relayerConf.Mailbox = "0xb1c938F5BA4B3593377F399e12175e8db0C787Ff"
	m.relayerConf.InterchainSecurityModule = "0xa05915fD6E32A1AA7E67d800164CaCB12487142d"
	m.relayerConf.InterchainGasPaymaster = "0x1D957dA7A6988f5a9d2D2454637B4B7fea0Aeea5"
	m.relayerConf.MerkleTreeHook = "0xFCb1d485ef46344029D9E8A7925925e146B3430E"
	m.relayerConf.ProxyAdmin = "0x7e7aD18Adc99b94d4c728fDf13D4dE97B926A0D8"
	m.relayerConf.ValidatorAnnounce = "0x79ec7bF05AF122D3782934d4Fb94eE32f0C01c97"
	// also set registry addresses in entry
	m.entry.Addresses = ContractAddresses{
		Mailbox:                  m.relayerConf.Mailbox,
		InterchainSecurityModule: m.relayerConf.InterchainSecurityModule,
		InterchainGasPaymaster:   m.relayerConf.InterchainGasPaymaster,
		MerkleTreeHook:           m.relayerConf.MerkleTreeHook,
		ProxyAdmin:               m.relayerConf.ProxyAdmin,
		ValidatorAnnounce:        m.relayerConf.ValidatorAnnounce,
	}
	return m
}

// newCosmosProvider returns a default Cosmos provider for "celestia" suitable for both
// registry and relayer tests.
func newCosmosProvider() *mockChainConfigProvider {
	meta := ChainMetadata{
		Name:                 "celestia",
		ChainID:              "celestia-testnet",
		DomainID:             69420,
		DisplayName:          "Celestia",
		Protocol:             "cosmosnative",
		IsTestnet:            true,
		RpcURLs:              []Endpoint{{HTTP: "http://celestia-validator:26657"}},
		RestURLs:             []Endpoint{{HTTP: "http://celestia-validator:1317"}},
		GrpcURLs:             []Endpoint{{HTTP: "http://celestia-validator:9090"}},
		Bech32Prefix:         "celestia",
		NativeToken:          NativeToken{Name: "TIA", Symbol: "TIA", Decimals: 6, Denom: "utia"},
		Blocks:               &BlockConfig{Confirmations: 1, EstimateBlockTime: 6, ReorgPeriod: 1},
		GasPrice:             &GasPrice{Denom: "utia", Amount: "0.002"},
		CanonicalAsset:       "utia",
		ContractAddressBytes: 32,
		Slip44:               118,
	}
	relayer := RelayerChainConfig{
		Name:                 "celestia",
		ChainID:              "celestia-testnet",
		DomainID:             69420,
		DisplayName:          "Celestia",
		Protocol:             "cosmosnative",
		IsTestnet:            true,
		NativeToken:          &NativeToken{Name: "TIA", Symbol: "TIA", Decimals: 6, Denom: "utia"},
		Blocks:               &BlockConfig{Confirmations: 1, EstimateBlockTime: 6, ReorgPeriod: 1},
		Signer:               &SignerConfig{Type: "cosmosKey", Key: "0x456", Prefix: "celestia"},
		RpcURLs:              []Endpoint{{HTTP: "http://celestia-validator:26657"}},
		RestURLs:             []Endpoint{{HTTP: "http://celestia-validator:1317"}},
		GrpcURLs:             []Endpoint{{HTTP: "http://celestia-validator:9090"}},
		Bech32Prefix:         "celestia",
		CanonicalAsset:       "utia",
		ContractAddressBytes: 32,
		GasPrice:             &GasPrice{Denom: "utia", Amount: "0.002"},
		Index:                &IndexConfig{From: 1150, Chunk: 10},
		Slip44:               118,
	}
	return &mockChainConfigProvider{entry: RegistryEntry{Metadata: meta}, relayerConf: relayer}
}
