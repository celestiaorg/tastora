package hyperlane

// Schema contains all Hyperlane configuration structures.
type Schema struct {
	RelayerConfig RelayerConfig
	Registry      Registry
}

// Registry models the contents of a hyperlane registry.
type Registry struct {
	Chains      map[string]*ChainEntry `yaml:"chains" json:"chains"`
	Deployments Deployments            `yaml:"deployments" json:"deployments"`
	Strategies  map[string]*Strategy   `yaml:"strategies,omitempty" json:"strategies,omitempty"`
}

type Deployments struct {
	Core       map[string][]CoreDeployment      `yaml:"core,omitempty" json:"core,omitempty"`
	WarpRoutes map[string][]WarpRouteDeployment `yaml:"warpRoutes,omitempty" json:"warpRoutes,omitempty"`
}

type CoreDeployment struct {
	Chain     string            `yaml:"chain" json:"chain"`
	Version   string            `yaml:"version,omitempty" json:"version,omitempty"`
	Addresses map[string]string `yaml:"addresses" json:"addresses"`
}

type WarpRouteDeployment struct {
	RouteName string                      `yaml:"routeName" json:"routeName"`
	Config    *WarpRouteConfig            `yaml:"config,omitempty" json:"config,omitempty"`
	Deploy    map[string]*WarpRouteDeploy `yaml:"deploy,omitempty" json:"deploy,omitempty"`
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

type ChainEntry struct {
	Name      string         `yaml:"name" json:"name"`
	Metadata  ChainMetadata  `yaml:"metadata" json:"metadata"`
	Addresses ChainAddresses `yaml:"addresses" json:"addresses"`
	LogoPath  string         `yaml:"logoPath,omitempty" json:"logoPath,omitempty"`
}

type ChainAddresses map[string]string

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
	Chains                  map[string]ChainConfig `json:"chains" yaml:"chains"`
	DefaultRpcConsensusType string                 `json:"defaultRpcConsensusType" yaml:"defaultRpcConsensusType"`
	RelayChains             string                 `json:"relayChains" yaml:"relayChains"`
}

type ChainConfig struct {
	Blocks      *BlockConfig  `json:"blocks,omitempty" yaml:"blocks,omitempty"`
	ChainID     interface{}   `json:"chainId" yaml:"chainId"`
	DisplayName string        `json:"displayName" yaml:"displayName"`
	DomainID    uint32        `json:"domainId" yaml:"domainId"`
	IsTestnet   bool          `json:"isTestnet" yaml:"isTestnet"`
	Name        string        `json:"name" yaml:"name"`
	NativeToken *NativeToken  `json:"nativeToken" yaml:"nativeToken"`
	Protocol    string        `json:"protocol" yaml:"protocol"`
	Signer      *SignerConfig `json:"signer" yaml:"signer"`

	RpcURLs  []Endpoint `json:"rpcUrls,omitempty" yaml:"rpcUrls,omitempty"`
	RestURLs []Endpoint `json:"restUrls,omitempty" yaml:"restUrls,omitempty"`
	GrpcURLs []Endpoint `json:"grpcUrls,omitempty" yaml:"grpcUrls,omitempty"`

	// Ethereum-only fields
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

// shared types with both json and yaml tags

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
