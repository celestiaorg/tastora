package hyperlane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type mockChainConfigProvider struct {
	metadata ChainMetadata
}

func (m *mockChainConfigProvider) GetHyperlaneChainMetadata(context.Context) (ChainMetadata, error) {
	return m.metadata, nil
}

func TestBuildRegistry_SingleEVMChain(t *testing.T) {
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
			SignerKey: "0x123",
		},
	}

	reg, err := BuildRegistry(context.Background(), []ChainConfigProvider{evmChain})
	require.NoError(t, err)
	require.NotNil(t, reg)
	require.Len(t, reg.Chains, 1)

	chain, ok := reg.Chains["rethlocal"]
	require.True(t, ok)
	require.Equal(t, "rethlocal", chain.Name)
	require.Equal(t, "ethereum", chain.Metadata.Protocol)
	require.Equal(t, 1234, chain.Metadata.ChainID)
	require.Equal(t, uint32(1234), chain.Metadata.DomainID)
	require.Equal(t, "Ether", chain.Metadata.NativeToken.Name)
	require.Len(t, chain.Metadata.RpcURLs, 1)
	require.Equal(t, "http://reth:8545", chain.Metadata.RpcURLs[0].HTTP)
}

func TestBuildRegistry_SingleCosmosChain(t *testing.T) {
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
			Slip44:    118,
			SignerKey: "0x456",
		},
	}

	reg, err := BuildRegistry(context.Background(), []ChainConfigProvider{cosmosChain})
	require.NoError(t, err)
	require.NotNil(t, reg)
	require.Len(t, reg.Chains, 1)

	chain, ok := reg.Chains["celestia"]
	require.True(t, ok)
	require.Equal(t, "celestia", chain.Name)
	require.Equal(t, "cosmosnative", chain.Metadata.Protocol)
	require.Equal(t, "celestia-testnet", chain.Metadata.ChainID)
	require.Equal(t, "celestia", chain.Metadata.Bech32Prefix)
	require.Equal(t, "TIA", chain.Metadata.NativeToken.Name)
	require.Equal(t, "utia", chain.Metadata.NativeToken.Denom)
	require.Len(t, chain.Metadata.RpcURLs, 1)
	require.Len(t, chain.Metadata.RestURLs, 1)
	require.Len(t, chain.Metadata.GrpcURLs, 1)
	require.NotNil(t, chain.Metadata.GasPrice)
	require.Equal(t, "utia", chain.Metadata.GasPrice.Denom)
}

func TestBuildRegistry_WithCoreContracts(t *testing.T) {
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
			SignerKey: "0x123",
			CoreContracts: &CoreContractAddresses{
				Mailbox:                  "0xb1c938F5BA4B3593377F399e12175e8db0C787Ff",
				InterchainSecurityModule: "0xa05915fD6E32A1AA7E67d800164CaCB12487142d",
				InterchainGasPaymaster:   "0x1D957dA7A6988f5a9d2D2454637B4B7fea0Aeea5",
				MerkleTreeHook:           "0xFCb1d485ef46344029D9E8A7925925e146B3430E",
				ProxyAdmin:               "0x7e7aD18Adc99b94d4c728fDf13D4dE97B926A0D8",
				ValidatorAnnounce:        "0x79ec7bF05AF122D3782934d4Fb94eE32f0C01c97",
			},
		},
	}

	reg, err := BuildRegistry(context.Background(), []ChainConfigProvider{evmChain})
	require.NoError(t, err)
	require.NotNil(t, reg)

	chain, ok := reg.Chains["rethlocal"]
	require.True(t, ok)
	require.Len(t, chain.Addresses, 6)
	require.Equal(t, "0xb1c938F5BA4B3593377F399e12175e8db0C787Ff", chain.Addresses["mailbox"])

	require.Len(t, reg.Deployments.Core, 1)
	require.Len(t, reg.Deployments.Core["rethlocal"], 1)
	require.Equal(t, "rethlocal", reg.Deployments.Core["rethlocal"][0].Chain)
	require.Len(t, reg.Deployments.Core["rethlocal"][0].Addresses, 6)
}

func TestBuildRegistry_MultipleChains(t *testing.T) {
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
			RpcURLs:      []Endpoint{{HTTP: "http://celestia-validator:26657"}},
			Bech32Prefix: "celestia",
			NativeToken: NativeToken{
				Name:     "TIA",
				Symbol:   "TIA",
				Decimals: 6,
				Denom:    "utia",
			},
			SignerKey: "0x456",
		},
	}

	reg, err := BuildRegistry(context.Background(), []ChainConfigProvider{evmChain, cosmosChain})
	require.NoError(t, err)
	require.NotNil(t, reg)
	require.Len(t, reg.Chains, 2)

	_, hasEVM := reg.Chains["rethlocal"]
	_, hasCosmos := reg.Chains["celestia"]
	require.True(t, hasEVM)
	require.True(t, hasCosmos)
}

func TestSerializeRegistry(t *testing.T) {
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
			SignerKey: "0x123",
		},
	}

	reg, err := BuildRegistry(context.Background(), []ChainConfigProvider{evmChain})
	require.NoError(t, err)

	yamlBytes, err := SerializeRegistry(reg)
	require.NoError(t, err)
	require.NotEmpty(t, yamlBytes)

	var deserialized map[string]interface{}
	err = yaml.Unmarshal(yamlBytes, &deserialized)
	require.NoError(t, err)

	chains, ok := deserialized["chains"].(map[string]interface{})
	require.True(t, ok)
	require.Len(t, chains, 1)
}
