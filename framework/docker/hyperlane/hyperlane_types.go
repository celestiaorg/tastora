package hyperlane

// ChainConfigProvider is the interface chains must implement to provide Hyperlane configuration.
type ChainConfigProvider interface {
	GetHyperlaneChainMetadata() ChainMetadata
}

// ChainMetadata contains all information needed to configure Hyperlane for a chain.
type ChainMetadata struct {
	// chain identity
	Name        string
	ChainID     interface{}
	DomainID    int
	DisplayName string
	Protocol    string
	IsTestnet   bool

	// network endpoints
	RPCURLs  []string
	RESTURLs []string
	GRPCURLs []string

	// token info
	NativeToken TokenMetadata

	// block configuration
	BlockConfig *BlockMetadata

	// signer configuration (private key hex string)
	SignerKey string

	// EVM-specific: Core contract addresses (empty if not deployed)
	CoreContracts *CoreContractAddresses

	// Cosmos-specific fields
	Bech32Prefix         string
	CanonicalAsset       string
	ContractAddressBytes int
	GasPrice             *GasPriceMetadata
	Slip44               int
	IndexConfig          *IndexMetadata
}

type TokenMetadata struct {
	Name     string
	Symbol   string
	Decimals int
	Denom    string
}

type BlockMetadata struct {
	Confirmations     int
	EstimateBlockTime int
	ReorgPeriod       int
}

type CoreContractAddresses struct {
	Mailbox                  string
	InterchainSecurityModule string
	InterchainGasPaymaster   string
	MerkleTreeHook           string
	ProxyAdmin               string
	ValidatorAnnounce        string
	AggregationHook          string
	DomainRoutingIsm         string
	FallbackRoutingHook      string
	ProtocolFee              string
	StorageGasOracle         string
	TestRecipient            string
}

type GasPriceMetadata struct {
	Denom  string
	Amount string
}

type IndexMetadata struct {
	From  int
	Chunk int
}
