package hyperlane

type RelayerConfig struct {
	Chains                  map[string]ChainConfig `json:"chains"`
	DefaultRpcConsensusType string                 `json:"defaultRpcConsensusType"`
	RelayChains             string                 `json:"relayChains"`
}

type ChainConfig struct {
	// Common fields
	Blocks      *BlockConfig  `json:"blocks,omitempty"`
	ChainID     interface{}   `json:"chainId"` // some chains use number, some use string
	DisplayName string        `json:"displayName"`
	DomainID    int           `json:"domainId"`
	IsTestnet   bool          `json:"isTestnet"`
	Name        string        `json:"name"`
	NativeToken *NativeToken  `json:"nativeToken"`
	Protocol    string        `json:"protocol"` // "ethereum", "cosmosnative", etc.
	Signer      *SignerConfig `json:"signer"`

	RPCUrls  []URLItem `json:"rpcUrls,omitempty"`
	RESTUrls []URLItem `json:"restUrls,omitempty"`
	GRPCUrls []URLItem `json:"grpcUrls,omitempty"`

	// Ethereum-only fields
	AggregationHook          string `json:"aggregationHook,omitempty"`
	DomainRoutingIsm         string `json:"domainRoutingIsm,omitempty"`
	FallbackRoutingHook      string `json:"fallbackRoutingHook,omitempty"`
	InterchainGasPaymaster   string `json:"interchainGasPaymaster,omitempty"`
	InterchainSecurityModule string `json:"interchainSecurityModule,omitempty"`
	Mailbox                  string `json:"mailbox,omitempty"`
	MerkleTreeHook           string `json:"merkleTreeHook,omitempty"`
	ProtocolFee              string `json:"protocolFee,omitempty"`
	ProxyAdmin               string `json:"proxyAdmin,omitempty"`
	StorageGasOracle         string `json:"storageGasOracle,omitempty"`
	TestRecipient            string `json:"testRecipient,omitempty"`
	ValidatorAnnounce        string `json:"validatorAnnounce,omitempty"`

	// Cosmos-only fields
	Bech32Prefix         string       `json:"bech32Prefix,omitempty"`
	CanonicalAsset       string       `json:"canonicalAsset,omitempty"`
	ContractAddressBytes int          `json:"contractAddressBytes,omitempty"`
	GasPrice             *GasPrice    `json:"gasPrice,omitempty"`
	Index                *IndexConfig `json:"index,omitempty"`
	Slip44               int          `json:"slip44,omitempty"`
}

type BlockConfig struct {
	Confirmations     int `json:"confirmations"`
	EstimateBlockTime int `json:"estimateBlockTime"`
	ReorgPeriod       int `json:"reorgPeriod"`
}

type NativeToken struct {
	Decimals int    `json:"decimals"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Denom    string `json:"denom,omitempty"`
}

type URLItem struct {
	HTTP string `json:"http"`
}

type SignerConfig struct {
	Type   string `json:"type"`
	Key    string `json:"key"`
	Prefix string `json:"prefix,omitempty"` // cosmos-only
}

type GasPrice struct {
	Denom  string `json:"denom"`
	Amount string `json:"amount"`
}

type IndexConfig struct {
	From  int `json:"from"`
	Chunk int `json:"chunk"`
}
