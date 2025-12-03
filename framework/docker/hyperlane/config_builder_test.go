package hyperlane

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildRelayerConfig_Empty(t *testing.T) {
	chains := []ChainConfigProvider{}
	config, err := BuildRelayerConfig(context.Background(), chains)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Empty(t, config.Chains)
	require.Equal(t, "fallback", config.DefaultRpcConsensusType)
	require.Equal(t, "", config.RelayChains)
}

func TestBuildRelayerConfig_SingleEVMChain(t *testing.T) {
	evmChain := &mockChainConfigProvider{
		metadata: ChainMetadata{
			Name:        "rethlocal",
			ChainID:     1234,
			DomainID:    1234,
			DisplayName: "Rethlocal",
			Protocol:    "ethereum",
			IsTestnet:   true,
			RpcURLs:     []Endpoint{{HTTP: "http://reth:8545"}},
			NativeToken: NativeToken{
				Name:     "Ether",
				Symbol:   "ETH",
				Decimals: 18,
			},
			Blocks: &BlockConfig{
				Confirmations:     1,
				EstimateBlockTime: 3,
				ReorgPeriod:       0,
			},
		},
		relayerConf: RelayerChainConfig{
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
		},
	}

	config, err := BuildRelayerConfig(context.Background(), []ChainConfigProvider{evmChain})
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Len(t, config.Chains, 1)
	require.Equal(t, "rethlocal", config.RelayChains)

	chain, ok := config.Chains["rethlocal"]
	require.True(t, ok)
	require.Equal(t, 1234, chain.ChainID)
	require.Equal(t, uint32(1234), chain.DomainID)
	require.Equal(t, "ethereum", chain.Protocol)
	require.True(t, chain.IsTestnet)

	require.NotNil(t, chain.Signer)
	require.Equal(t, "hexKey", chain.Signer.Type)
	require.Equal(t, "0x123", chain.Signer.Key)

	require.Equal(t, "0xb1c938F5BA4B3593377F399e12175e8db0C787Ff", chain.Mailbox)
	require.Equal(t, "0xa05915fD6E32A1AA7E67d800164CaCB12487142d", chain.InterchainSecurityModule)

	require.NotNil(t, chain.NativeToken)
	require.Equal(t, "Ether", chain.NativeToken.Name)
	require.Equal(t, "ETH", chain.NativeToken.Symbol)
	require.Equal(t, 18, chain.NativeToken.Decimals)

	require.NotNil(t, chain.Blocks)
	require.Equal(t, 1, chain.Blocks.Confirmations)
	require.Equal(t, 3, chain.Blocks.EstimateBlockTime)

	require.Len(t, chain.RpcURLs, 1)
	require.Equal(t, "http://reth:8545", chain.RpcURLs[0].HTTP)
}

func TestBuildRelayerConfig_SingleCosmosChain(t *testing.T) {
	cosmosChain := &mockChainConfigProvider{
		metadata: ChainMetadata{
			Name:         "celestia",
			ChainID:      "celestia-testnet",
			DomainID:     69420,
			DisplayName:  "Celestia",
			Protocol:     "cosmosnative",
			IsTestnet:    true,
			RpcURLs:      []Endpoint{{HTTP: "http://celestia-validator:26657"}},
			RestURLs:     []Endpoint{{HTTP: "http://celestia-validator:1317"}},
			GrpcURLs:     []Endpoint{{HTTP: "http://celestia-validator:9090"}},
			Bech32Prefix: "celestia",
			NativeToken: NativeToken{
				Name:     "TIA",
				Symbol:   "TIA",
				Decimals: 6,
				Denom:    "utia",
			},
			Blocks: &BlockConfig{
				Confirmations:     1,
				EstimateBlockTime: 6,
				ReorgPeriod:       1,
			},
			GasPrice: &GasPrice{
				Denom:  "utia",
				Amount: "0.002",
			},
			CanonicalAsset:       "utia",
			ContractAddressBytes: 32,
			Slip44:               118,
		},
		relayerConf: RelayerChainConfig{
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
		},
	}

	config, err := BuildRelayerConfig(context.Background(), []ChainConfigProvider{cosmosChain})
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Len(t, config.Chains, 1)

	chain, ok := config.Chains["celestia"]
	require.True(t, ok)
	require.Equal(t, "celestia-testnet", chain.ChainID)
	require.Equal(t, "cosmosnative", chain.Protocol)

	require.NotNil(t, chain.Signer)
	require.Equal(t, "cosmosKey", chain.Signer.Type)
	require.Equal(t, "0x456", chain.Signer.Key)
	require.Equal(t, "celestia", chain.Signer.Prefix)

	require.Equal(t, "celestia", chain.Bech32Prefix)
	require.Equal(t, "utia", chain.CanonicalAsset)
	require.Equal(t, 32, chain.ContractAddressBytes)
	require.Equal(t, 118, chain.Slip44)

	require.NotNil(t, chain.GasPrice)
	require.Equal(t, "utia", chain.GasPrice.Denom)
	require.Equal(t, "0.002", chain.GasPrice.Amount)

	require.NotNil(t, chain.Index)
	require.Equal(t, 1150, chain.Index.From)
	require.Equal(t, 10, chain.Index.Chunk)

	require.Len(t, chain.RpcURLs, 1)
	require.Len(t, chain.RestURLs, 1)
	require.Len(t, chain.GrpcURLs, 1)
}

func TestBuildRelayerConfig_MultipleChains(t *testing.T) {
	evmChain := &mockChainConfigProvider{
		metadata: ChainMetadata{
			Name:        "rethlocal",
			ChainID:     1234,
			DomainID:    1234,
			DisplayName: "Rethlocal",
			Protocol:    "ethereum",
			IsTestnet:   true,
			RpcURLs:     []Endpoint{{HTTP: "http://reth:8545"}},
			NativeToken: NativeToken{
				Name:     "Ether",
				Symbol:   "ETH",
				Decimals: 18,
			},
		},
		relayerConf: RelayerChainConfig{Name: "rethlocal", ChainID: 1234, DomainID: 1234, DisplayName: "Rethlocal", Protocol: "ethereum", IsTestnet: true, NativeToken: &NativeToken{Name: "Ether", Symbol: "ETH", Decimals: 18}, Signer: &SignerConfig{Type: "hexKey", Key: "0x123"}, RpcURLs: []Endpoint{{HTTP: "http://reth:8545"}}},
	}

	cosmosChain := &mockChainConfigProvider{
		metadata: ChainMetadata{
			Name:         "celestia",
			ChainID:      "celestia-testnet",
			DomainID:     69420,
			DisplayName:  "Celestia",
			Protocol:     "cosmosnative",
			IsTestnet:    true,
			RpcURLs:      []Endpoint{{HTTP: "http://celestia-validator:26657"}},
			Bech32Prefix: "celestia",
			NativeToken: NativeToken{
				Name:     "TIA",
				Symbol:   "TIA",
				Decimals: 6,
				Denom:    "utia",
			},
		},
		relayerConf: RelayerChainConfig{Name: "celestia", ChainID: "celestia-testnet", DomainID: 69420, DisplayName: "Celestia", Protocol: "cosmosnative", IsTestnet: true, NativeToken: &NativeToken{Name: "TIA", Symbol: "TIA", Decimals: 6, Denom: "utia"}, Signer: &SignerConfig{Type: "cosmosKey", Key: "0x456", Prefix: "celestia"}, RpcURLs: []Endpoint{{HTTP: "http://celestia-validator:26657"}}},
	}

	config, err := BuildRelayerConfig(
		context.Background(),
		[]ChainConfigProvider{evmChain, cosmosChain},
	)
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Len(t, config.Chains, 2)
	require.Equal(t, "rethlocal,celestia", config.RelayChains)

	_, hasEVM := config.Chains["rethlocal"]
	_, hasCosmos := config.Chains["celestia"]
	require.True(t, hasEVM)
	require.True(t, hasCosmos)

	require.Equal(t, "hexKey", config.Chains["rethlocal"].Signer.Type)
	require.Equal(t, "cosmosKey", config.Chains["celestia"].Signer.Type)
}

func TestSerializeRelayerConfig(t *testing.T) {
	evmChain := &mockChainConfigProvider{
		metadata: ChainMetadata{
			Name:        "rethlocal",
			ChainID:     1234,
			DomainID:    1234,
			DisplayName: "Rethlocal",
			Protocol:    "ethereum",
			IsTestnet:   true,
			RpcURLs:     []Endpoint{{HTTP: "http://reth:8545"}},
			NativeToken: NativeToken{
				Name:     "Ether",
				Symbol:   "ETH",
				Decimals: 18,
			},
		},
		relayerConf: RelayerChainConfig{Name: "rethlocal", ChainID: 1234, DomainID: 1234, DisplayName: "Rethlocal", Protocol: "ethereum", IsTestnet: true, NativeToken: &NativeToken{Name: "Ether", Symbol: "ETH", Decimals: 18}, Signer: &SignerConfig{Type: "hexKey", Key: "0x123"}, RpcURLs: []Endpoint{{HTTP: "http://reth:8545"}}},
	}

	config, err := BuildRelayerConfig(context.Background(), []ChainConfigProvider{evmChain})
	require.NoError(t, err)

	jsonBytes, err := SerializeRelayerConfig(config)
	require.NoError(t, err)
	require.NotEmpty(t, jsonBytes)

	var deserialized map[string]interface{}
	err = json.Unmarshal(jsonBytes, &deserialized)
	require.NoError(t, err)

	chains, ok := deserialized["chains"].(map[string]interface{})
	require.True(t, ok)
	require.Len(t, chains, 1)

	defaultRpcConsensusType, ok := deserialized["defaultRpcConsensusType"].(string)
	require.True(t, ok)
	require.Equal(t, "fallback", defaultRpcConsensusType)

	relayChains, ok := deserialized["relayChains"].(string)
	require.True(t, ok)
	require.Equal(t, "rethlocal", relayChains)
}

func TestBuildRelayerConfig_SignerTypes(t *testing.T) {
	tests := []struct {
		name           string
		protocol       string
		bech32Prefix   string
		expectedType   string
		expectedPrefix string
	}{
		{
			name:           "ethereum uses hexKey",
			protocol:       "ethereum",
			bech32Prefix:   "",
			expectedType:   "hexKey",
			expectedPrefix: "",
		},
		{
			name:           "cosmosnative uses cosmosKey",
			protocol:       "cosmosnative",
			bech32Prefix:   "celestia",
			expectedType:   "cosmosKey",
			expectedPrefix: "celestia",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := &mockChainConfigProvider{
				metadata: ChainMetadata{
					Name:         "test",
					ChainID:      "test",
					DomainID:     1,
					Protocol:     tt.protocol,
					Bech32Prefix: tt.bech32Prefix,
					NativeToken:  NativeToken{Name: "Test", Symbol: "TST", Decimals: 6},
				},
				relayerConf: RelayerChainConfig{
					Name:        "test",
					ChainID:     "test",
					DomainID:    1,
					Protocol:    tt.protocol,
					NativeToken: &NativeToken{Name: "Test", Symbol: "TST", Decimals: 6},
					Signer:      &SignerConfig{Type: tt.expectedType, Prefix: tt.expectedPrefix, Key: "0x123"},
				},
			}

			config, err := BuildRelayerConfig(context.Background(), []ChainConfigProvider{chain})
			require.NoError(t, err)

			chainConfig := config.Chains["test"]
			require.Equal(t, tt.expectedType, chainConfig.Signer.Type)
			require.Equal(t, tt.expectedPrefix, chainConfig.Signer.Prefix)
		})
	}
}
