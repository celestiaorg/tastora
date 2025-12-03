package hyperlane

import "context"

// ChainConfigProvider provides exact models for registry and relayer configs.
type ChainConfigProvider interface {
	// GetHyperlaneRegistryEntry returns the on-disk registry entry (metadata + addresses).
	GetHyperlaneRegistryEntry(ctx context.Context) (RegistryEntry, error)
	// GetHyperlaneRelayerChainConfig returns the on-disk relayer chain config model.
	GetHyperlaneRelayerChainConfig(ctx context.Context) (RelayerChainConfig, error)
}

// ChainMetadata contains all information needed to configure Hyperlane for a chain.
// includes fields for both registry and relayer config generation.
type ChainMetadata struct {
	ChainID     interface{} `json:"chainId" yaml:"chainId"`
	DomainID    uint32      `json:"domainId" yaml:"domainId"`
	Name        string      `json:"name" yaml:"name"`
	DisplayName string      `json:"displayName" yaml:"displayName"`
	Protocol    string      `json:"protocol" yaml:"protocol"`
	IsTestnet   bool        `json:"isTestnet" yaml:"isTestnet"`
	NativeToken NativeToken `json:"nativeToken" yaml:"nativeToken"`

	// network endpoints
	RpcURLs  []Endpoint `json:"rpcUrls,omitempty" yaml:"rpcUrls,omitempty"`
	RestURLs []Endpoint `json:"restUrls,omitempty" yaml:"restUrls,omitempty"`
	GrpcURLs []Endpoint `json:"grpcUrls,omitempty" yaml:"grpcUrls,omitempty"`

	Blocks                 *BlockConfig    `json:"blocks,omitempty" yaml:"blocks,omitempty"`
	BlockExplorers         []BlockExplorer `json:"blockExplorers,omitempty" yaml:"blockExplorers,omitempty"`
	TechnicalStack         string          `json:"technicalStack,omitempty" yaml:"technicalStack,omitempty"`
	GasCurrencyCoinGeckoId string          `json:"gasCurrencyCoinGeckoId,omitempty" yaml:"gasCurrencyCoinGeckoId,omitempty"`

	// Cosmos-specific fields
	Bech32Prefix         string    `json:"bech32Prefix,omitempty" yaml:"bech32Prefix,omitempty"`
	CanonicalAsset       string    `json:"canonicalAsset,omitempty" yaml:"canonicalAsset,omitempty"`
	ContractAddressBytes int       `json:"contractAddressBytes,omitempty" yaml:"contractAddressBytes,omitempty"`
	GasPrice             *GasPrice `json:"gasPrice,omitempty" yaml:"gasPrice,omitempty"`
	Slip44               int       `json:"slip44,omitempty" yaml:"slip44,omitempty"`
}
