package registry

// Registry models the contents of a hyperlane registry.
type Registry struct {
	Chains      map[string]*ChainEntry `yaml:"chains"`
	Deployments Deployments            `yaml:"deployments"`
	Strategies  map[string]*Strategy   `yaml:"strategies,omitempty"`
}

type Deployments struct {
	Core       map[string][]CoreDeployment      `yaml:"core,omitempty"`
	WarpRoutes map[string][]WarpRouteDeployment `yaml:"warpRoutes,omitempty"`
}

type CoreDeployment struct {
	Chain     string            `yaml:"chain"`
	Version   string            `yaml:"version,omitempty"`
	Addresses map[string]string `yaml:"addresses"`
}

type WarpRouteDeployment struct {
	RouteName string                      `yaml:"routeName"`
	Config    *WarpRouteConfig            `yaml:"config,omitempty"`
	Deploy    map[string]*WarpRouteDeploy `yaml:"deploy,omitempty"`
}

type WarpRouteConfig struct {
	Tokens []WarpToken `yaml:"tokens"`
}

type WarpToken struct {
	AddressOrDenom string `yaml:"addressOrDenom"`
	ChainName      string `yaml:"chainName"`
	Decimals       int    `yaml:"decimals"`
	Name           string `yaml:"name"`
	Standard       string `yaml:"standard"`
	Symbol         string `yaml:"symbol"`
}

type WarpRouteDeploy struct {
	IsNft bool   `yaml:"isNft"`
	Owner string `yaml:"owner"`
	Type  string `yaml:"type"`
}

type ChainEntry struct {
	Name      string         `yaml:"name"`
	Metadata  ChainMetadata  `yaml:"metadata"`
	Addresses ChainAddresses `yaml:"addresses"`
	LogoPath  string         `yaml:"logoPath,omitempty"`
}

type ChainMetadata struct {
	ChainID                any             `yaml:"chainId"`
	DomainID               uint32          `yaml:"domainId"`
	Name                   string          `yaml:"name"`
	DisplayName            string          `yaml:"displayName"`
	Protocol               string          `yaml:"protocol"`
	IsTestnet              bool            `yaml:"isTestnet"`
	NativeToken            NativeToken     `yaml:"nativeToken"`
	RpcURLs                []Endpoint      `yaml:"rpcUrls,omitempty"`
	RestURLs               []Endpoint      `yaml:"restUrls,omitempty"`
	GrpcURLs               []Endpoint      `yaml:"grpcUrls,omitempty"`
	BlockExplorers         []BlockExplorer `yaml:"blockExplorers,omitempty"`
	Blocks                 *BlockConfig    `yaml:"blocks,omitempty"`
	TechnicalStack         string          `yaml:"technicalStack,omitempty"`
	GasPrice               *GasPrice       `yaml:"gasPrice,omitempty"`
	GasCurrencyCoinGeckoId string          `yaml:"gasCurrencyCoinGeckoId,omitempty"`
	Bech32Prefix           string          `yaml:"bech32Prefix,omitempty"`
	CanonicalAsset         string          `yaml:"canonicalAsset,omitempty"`
	ContractAddressBytes   int             `yaml:"contractAddressBytes,omitempty"`
	Slip44                 int             `yaml:"slip44,omitempty"`
}

type NativeToken struct {
	Name     string `yaml:"name"`
	Symbol   string `yaml:"symbol"`
	Decimals uint8  `yaml:"decimals"`
	Denom    string `yaml:"denom,omitempty"`
}

type Endpoint struct {
	HTTP string `yaml:"http"`
}

type BlockExplorer struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url"`
	ApiURL string `yaml:"apiUrl,omitempty"`
	Family string `yaml:"family"`
}

type BlockConfig struct {
	Confirmations     int `yaml:"confirmations"`
	EstimateBlockTime int `yaml:"estimateBlockTime"`
	ReorgPeriod       int `yaml:"reorgPeriod"`
}

type GasPrice struct {
	Denom  string `yaml:"denom"`
	Amount string `yaml:"amount"`
}

type ChainAddresses map[string]string

type Strategy struct {
	Submitter Submitter `yaml:"submitter"`
}

type Submitter struct {
	Chain       string `yaml:"chain"`
	Type        string `yaml:"type"`
	PrivateKey  string `yaml:"privateKey,omitempty"`
	UserAddress string `yaml:"userAddress,omitempty"`
}
