package hyperlane

import "context"

// Schema contains all Hyperlane configuration structures.
type Schema struct {
	RelayerConfig *RelayerConfig
	Registry      *Registry
	WarpConfig    map[string]*WarpConfigEntry
	CoreConfig    *CoreConfig
}

// buildSchema builds a hyperlane scheme from the provided set of chains.
func buildSchema(ctx context.Context, chains []ChainConfigProvider) (*Schema, error) {
	config, err := BuildRelayerConfig(ctx, chains)
	if err != nil {
		return nil, err
	}

	registry, err := BuildRegistry(ctx, chains)
	if err != nil {
		return nil, err
	}

	return &Schema{
		RelayerConfig: config,
		Registry:      registry,
	}, nil
}

// Registry models the contents of a hyperlane registry.
type Registry struct {
	Chains     map[string]*RegistryEntry `yaml:"chains" json:"chains"`
	Strategies map[string]*Strategy      `yaml:"strategies,omitempty" json:"strategies,omitempty"`
}

type WarpRouteConfig struct {
	Tokens []WarpToken `yaml:"tokens" json:"tokens"`
}

type WarpToken struct {
	AddressOrDenom string `yaml:"addressOrDenom" json:"addressOrDenom"`
	ChainName      string `yaml:"chainName" json:"chainName"`
	Decimals       int    `yaml:"decimals" json:"decimals"`
	Name           string `yaml:"name" json:"name"`
	Standard       string `yaml:"standard" json:"standard"`
	Symbol         string `yaml:"symbol" json:"symbol"`
}

type WarpRouteDeploy struct {
	IsNft bool   `yaml:"isNft" json:"isNft"`
	Owner string `yaml:"owner" json:"owner"`
	Type  string `yaml:"type" json:"type"`
}

type WarpConfigEntry struct {
	Type                     string `yaml:"type" json:"type"`
	Owner                    string `yaml:"owner,omitempty" json:"owner,omitempty"`
	Mailbox                  string `yaml:"mailbox,omitempty" json:"mailbox,omitempty"`
	InterchainSecurityModule string `yaml:"interchainSecurityModule,omitempty" json:"interchainSecurityModule,omitempty"`
	Name                     string `yaml:"name,omitempty" json:"name,omitempty"`
	Symbol                   string `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Decimals                 int    `yaml:"decimals,omitempty" json:"decimals,omitempty"`
}

type RegistryEntry struct {
	Metadata  ChainMetadata     `yaml:"metadata" json:"metadata"`
	Addresses ContractAddresses `yaml:"addresses" json:"addresses"`
}

// ContractAddresses models core contract addresses as a structured object
// rather than a generic map to align with file layouts.
type ContractAddresses struct {
	Mailbox                  string `yaml:"mailbox,omitempty" json:"mailbox,omitempty"`
	InterchainSecurityModule string `yaml:"interchainSecurityModule,omitempty" json:"interchainSecurityModule,omitempty"`
	InterchainGasPaymaster   string `yaml:"interchainGasPaymaster,omitempty" json:"interchainGasPaymaster,omitempty"`
	MerkleTreeHook           string `yaml:"merkleTreeHook,omitempty" json:"merkleTreeHook,omitempty"`
	ProxyAdmin               string `yaml:"proxyAdmin,omitempty" json:"proxyAdmin,omitempty"`
	ValidatorAnnounce        string `yaml:"validatorAnnounce,omitempty" json:"validatorAnnounce,omitempty"`
	AggregationHook          string `yaml:"aggregationHook,omitempty" json:"aggregationHook,omitempty"`
	DomainRoutingIsm         string `yaml:"domainRoutingIsm,omitempty" json:"domainRoutingIsm,omitempty"`
	FallbackRoutingHook      string `yaml:"fallbackRoutingHook,omitempty" json:"fallbackRoutingHook,omitempty"`
	ProtocolFee              string `yaml:"protocolFee,omitempty" json:"protocolFee,omitempty"`
	StorageGasOracle         string `yaml:"storageGasOracle,omitempty" json:"storageGasOracle,omitempty"`
	TestRecipient            string `yaml:"testRecipient,omitempty" json:"testRecipient,omitempty"`
	TestIsm                  string `yaml:"testIsm,omitempty" json:"testIsm,omitempty"`
	// Additional factory and router addresses used by Hyperlane
	DomainRoutingIsmFactory                    string `yaml:"domainRoutingIsmFactory,omitempty" json:"domainRoutingIsmFactory,omitempty"`
	InterchainAccountIsm                       string `yaml:"interchainAccountIsm,omitempty" json:"interchainAccountIsm,omitempty"`
	InterchainAccountRouter                    string `yaml:"interchainAccountRouter,omitempty" json:"interchainAccountRouter,omitempty"`
	StaticAggregationHookFactory               string `yaml:"staticAggregationHookFactory,omitempty" json:"staticAggregationHookFactory,omitempty"`
	StaticAggregationIsmFactory                string `yaml:"staticAggregationIsmFactory,omitempty" json:"staticAggregationIsmFactory,omitempty"`
	StaticMerkleRootMultisigIsmFactory         string `yaml:"staticMerkleRootMultisigIsmFactory,omitempty" json:"staticMerkleRootMultisigIsmFactory,omitempty"`
	StaticMerkleRootWeightedMultisigIsmFactory string `yaml:"staticMerkleRootWeightedMultisigIsmFactory,omitempty" json:"staticMerkleRootWeightedMultisigIsmFactory,omitempty"`
	StaticMessageIdMultisigIsmFactory          string `yaml:"staticMessageIdMultisigIsmFactory,omitempty" json:"staticMessageIdMultisigIsmFactory,omitempty"`
	StaticMessageIdWeightedMultisigIsmFactory  string `yaml:"staticMessageIdWeightedMultisigIsmFactory,omitempty" json:"staticMessageIdWeightedMultisigIsmFactory,omitempty"`
}

type Strategy struct {
	Submitter Submitter `yaml:"submitter" json:"submitter"`
}

type Submitter struct {
	Chain       string `yaml:"chain" json:"chain"`
	Type        string `yaml:"type" json:"type"`
	PrivateKey  string `yaml:"privateKey,omitempty" json:"privateKey,omitempty"`
	UserAddress string `yaml:"userAddress,omitempty" json:"userAddress,omitempty"`
}

type RelayerConfig struct {
	Chains                  map[string]RelayerChainConfig `json:"chains" yaml:"chains"`
	DefaultRpcConsensusType string                        `json:"defaultRpcConsensusType" yaml:"defaultRpcConsensusType"`
	RelayChains             string                        `json:"relayChains" yaml:"relayChains"`
}

// RelayerChainConfig represents a single chain's relayer config (relayer/chains/<name>.json).
type RelayerChainConfig struct {
	// Name is not serialized; used only as the map key.
	Name        string        `json:"-" yaml:"-"`
	Blocks      *BlockConfig  `json:"blocks,omitempty" yaml:"blocks,omitempty"`
	ChainID     interface{}   `json:"chainId" yaml:"chainId"`
	DisplayName string        `json:"displayName" yaml:"displayName"`
	DomainID    uint32        `json:"domainId" yaml:"domainId"`
	IsTestnet   bool          `json:"isTestnet" yaml:"isTestnet"`
	NativeToken *NativeToken  `json:"nativeToken" yaml:"nativeToken"`
	Protocol    string        `json:"protocol" yaml:"protocol"`
	Signer      *SignerConfig `json:"signer" yaml:"signer"`

	RpcURLs  []Endpoint `json:"rpcUrls,omitempty" yaml:"rpcUrls,omitempty"`
	RestURLs []Endpoint `json:"restUrls,omitempty" yaml:"restUrls,omitempty"`
	GrpcURLs []Endpoint `json:"grpcUrls,omitempty" yaml:"grpcUrls,omitempty"`

	AggregationHook          string `json:"aggregationHook,omitempty" yaml:"aggregationHook,omitempty"`
	DomainRoutingIsm         string `json:"domainRoutingIsm,omitempty" yaml:"domainRoutingIsm,omitempty"`
	FallbackRoutingHook      string `json:"fallbackRoutingHook,omitempty" yaml:"fallbackRoutingHook,omitempty"`
	InterchainGasPaymaster   string `json:"interchainGasPaymaster,omitempty" yaml:"interchainGasPaymaster,omitempty"`
	InterchainSecurityModule string `json:"interchainSecurityModule,omitempty" yaml:"interchainSecurityModule,omitempty"`
	Mailbox                  string `json:"mailbox,omitempty" yaml:"mailbox,omitempty"`
	MerkleTreeHook           string `json:"merkleTreeHook,omitempty" yaml:"merkleTreeHook,omitempty"`
	ProtocolFee              string `json:"protocolFee,omitempty" yaml:"protocolFee,omitempty"`
	ProxyAdmin               string `json:"proxyAdmin,omitempty" yaml:"proxyAdmin,omitempty"`
	StorageGasOracle         string `json:"storageGasOracle,omitempty" yaml:"storageGasOracle,omitempty"`
	TestRecipient            string `json:"testRecipient,omitempty" yaml:"testRecipient,omitempty"`
	ValidatorAnnounce        string `json:"validatorAnnounce,omitempty" yaml:"validatorAnnounce,omitempty"`

	// Cosmos-only fields
	Bech32Prefix         string       `json:"bech32Prefix,omitempty" yaml:"bech32Prefix,omitempty"`
	CanonicalAsset       string       `json:"canonicalAsset,omitempty" yaml:"canonicalAsset,omitempty"`
	ContractAddressBytes int          `json:"contractAddressBytes,omitempty" yaml:"contractAddressBytes,omitempty"`
	GasPrice             *GasPrice    `json:"gasPrice,omitempty" yaml:"gasPrice,omitempty"`
	Index                *IndexConfig `json:"index,omitempty" yaml:"index,omitempty"`
	Slip44               int          `json:"slip44,omitempty" yaml:"slip44,omitempty"`
}

type SignerConfig struct {
	Type   string `json:"type" yaml:"type"`
	Key    string `json:"key" yaml:"key"`
	Prefix string `json:"prefix,omitempty" yaml:"prefix,omitempty"`
}

type IndexConfig struct {
	From  int `json:"from" yaml:"from"`
	Chunk int `json:"chunk" yaml:"chunk"`
}

type NativeToken struct {
	Name     string `json:"name" yaml:"name"`
	Symbol   string `json:"symbol" yaml:"symbol"`
	Decimals int    `json:"decimals" yaml:"decimals"`
	Denom    string `json:"denom,omitempty" yaml:"denom,omitempty"`
}

type BlockConfig struct {
	Confirmations     int `json:"confirmations" yaml:"confirmations"`
	EstimateBlockTime int `json:"estimateBlockTime" yaml:"estimateBlockTime"`
	ReorgPeriod       int `json:"reorgPeriod" yaml:"reorgPeriod"`
}

type GasPrice struct {
	Denom  string `json:"denom" yaml:"denom"`
	Amount string `json:"amount" yaml:"amount"`
}

type Endpoint struct {
	HTTP string `json:"http" yaml:"http"`
}

type BlockExplorer struct {
	Name   string `json:"name" yaml:"name"`
	URL    string `json:"url" yaml:"url"`
	ApiURL string `json:"apiUrl,omitempty" yaml:"apiUrl,omitempty"`
	Family string `json:"family" yaml:"family"`
}
