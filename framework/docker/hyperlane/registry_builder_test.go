package hyperlane

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestBuildRegistry_SingleEVMChain(t *testing.T) {
    evmChain := newEVMProvider()

	reg, err := BuildRegistry(context.Background(), []ChainConfigProvider{evmChain})
	require.NoError(t, err)
	require.NotNil(t, reg)
	require.Len(t, reg.Chains, 1)

	chain, ok := reg.Chains["rethlocal"]
	require.True(t, ok)
	require.Equal(t, "ethereum", chain.Metadata.Protocol)
	require.Equal(t, 1234, chain.Metadata.ChainID)
	require.Equal(t, uint32(1234), chain.Metadata.DomainID)
	require.Equal(t, "Ether", chain.Metadata.NativeToken.Name)
	require.Len(t, chain.Metadata.RpcURLs, 1)
	require.Equal(t, "http://reth:8545", chain.Metadata.RpcURLs[0].HTTP)
}

func TestBuildRegistry_SingleCosmosChain(t *testing.T) {
    cosmosChain := newCosmosProvider()

	reg, err := BuildRegistry(context.Background(), []ChainConfigProvider{cosmosChain})
	require.NoError(t, err)
	require.NotNil(t, reg)
	require.Len(t, reg.Chains, 1)

	chain, ok := reg.Chains["celestia"]
	require.True(t, ok)
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
    evmChain := newEVMProviderWithCore()

	reg, err := BuildRegistry(context.Background(), []ChainConfigProvider{evmChain})
	require.NoError(t, err)
	require.NotNil(t, reg)

	chain, ok := reg.Chains["rethlocal"]
	require.True(t, ok)
    require.NotEmpty(t, chain.Addresses.Mailbox)
    require.NotEmpty(t, chain.Addresses.InterchainSecurityModule)
    require.NotEmpty(t, chain.Addresses.InterchainGasPaymaster)
    require.NotEmpty(t, chain.Addresses.MerkleTreeHook)
    require.NotEmpty(t, chain.Addresses.ProxyAdmin)
    require.NotEmpty(t, chain.Addresses.ValidatorAnnounce)
}

func TestSerializeRegistry(t *testing.T) {
    evmChain := newEVMProvider()

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
