package hyperlane

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildRelayerConfig_Empty(t *testing.T) {
	chains := []ChainConfigProvider{}
	config, err := BuildRelayerConfig(chains)
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
			RPCURLs:     []string{"http://reth:8545"},
			NativeToken: TokenMetadata{
				Name:     "Ether",
				Symbol:   "ETH",
				Decimals: 18,
			},
			BlockConfig: &BlockMetadata{
				Confirmations:     1,
				EstimateBlockTime: 3,
				ReorgPeriod:       0,
			},
			SignerKey: "0x123",
			CoreContracts: &CoreContractAddresses{
				Mailbox:                  "0xb1c938F5BA4B3593377F399e12175e8db0C787Ff",
				InterchainSecurityModule: "0xa05915fD6E32A1AA7E67d800164CaCB12487142d",
			},
		},
	}

	config, err := BuildRelayerConfig([]ChainConfigProvider{evmChain})
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Len(t, config.Chains, 1)
	require.Equal(t, "rethlocal", config.RelayChains)

	chain, ok := config.Chains["rethlocal"]
	require.True(t, ok)
	require.Equal(t, "rethlocal", chain.Name)
	require.Equal(t, 1234, chain.ChainID)
	require.Equal(t, 1234, chain.DomainID)
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

	require.Len(t, chain.RPCUrls, 1)
	require.Equal(t, "http://reth:8545", chain.RPCUrls[0].HTTP)
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
			RPCURLs:      []string{"http://celestia-validator:26657"},
			RESTURLs:     []string{"http://celestia-validator:1317"},
			GRPCURLs:     []string{"http://celestia-validator:9090"},
			Bech32Prefix: "celestia",
			NativeToken: TokenMetadata{
				Name:     "TIA",
				Symbol:   "TIA",
				Decimals: 6,
				Denom:    "utia",
			},
			BlockConfig: &BlockMetadata{
				Confirmations:     1,
				EstimateBlockTime: 6,
				ReorgPeriod:       1,
			},
			GasPrice: &GasPriceMetadata{
				Denom:  "utia",
				Amount: "0.002",
			},
			IndexConfig: &IndexMetadata{
				From:  1150,
				Chunk: 10,
			},
			CanonicalAsset:       "utia",
			ContractAddressBytes: 32,
			Slip44:               118,
			SignerKey:            "0x456",
		},
	}

	config, err := BuildRelayerConfig([]ChainConfigProvider{cosmosChain})
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Len(t, config.Chains, 1)

	chain, ok := config.Chains["celestia"]
	require.True(t, ok)
	require.Equal(t, "celestia", chain.Name)
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

	require.Len(t, chain.RPCUrls, 1)
	require.Len(t, chain.RESTUrls, 1)
	require.Len(t, chain.GRPCUrls, 1)
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
			RPCURLs:     []string{"http://reth:8545"},
			NativeToken: TokenMetadata{
				Name:     "Ether",
				Symbol:   "ETH",
				Decimals: 18,
			},
			SignerKey: "0x123",
		},
	}

	cosmosChain := &mockChainConfigProvider{
		metadata: ChainMetadata{
			Name:         "celestia",
			ChainID:      "celestia-testnet",
			DomainID:     69420,
			DisplayName:  "Celestia",
			Protocol:     "cosmosnative",
			IsTestnet:    true,
			RPCURLs:      []string{"http://celestia-validator:26657"},
			Bech32Prefix: "celestia",
			NativeToken: TokenMetadata{
				Name:     "TIA",
				Symbol:   "TIA",
				Decimals: 6,
				Denom:    "utia",
			},
			SignerKey: "0x456",
		},
	}

	config, err := BuildRelayerConfig(
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
			RPCURLs:     []string{"http://reth:8545"},
			NativeToken: TokenMetadata{
				Name:     "Ether",
				Symbol:   "ETH",
				Decimals: 18,
			},
			SignerKey: "0x123",
		},
	}

	config, err := BuildRelayerConfig([]ChainConfigProvider{evmChain})
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
					NativeToken: TokenMetadata{
						Name:     "Test",
						Symbol:   "TST",
						Decimals: 6,
					},
					SignerKey: "0x123",
				},
			}

			config, err := BuildRelayerConfig([]ChainConfigProvider{chain})
			require.NoError(t, err)

			chainConfig := config.Chains["test"]
			require.Equal(t, tt.expectedType, chainConfig.Signer.Type)
			require.Equal(t, tt.expectedPrefix, chainConfig.Signer.Prefix)
		})
	}
}
