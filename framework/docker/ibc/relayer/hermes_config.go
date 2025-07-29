package relayer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/celestiaorg/tastora/framework/types"
)

// HermesConfig represents the full Hermes configuration
type HermesConfig struct {
	Global       GlobalConfig    `toml:"global"`
	Mode         ModeConfig      `toml:"mode"`
	Rest         RestConfig      `toml:"rest"`
	Telemetry    TelemetryConfig `toml:"telemetry"`
	TracingServer TracingConfig  `toml:"tracing_server"`
	Chains       []ChainConfig   `toml:"chains"`
}

// GlobalConfig contains global Hermes settings
type GlobalConfig struct {
	LogLevel string `toml:"log_level"`
}

// ModeConfig defines the relayer operation modes
type ModeConfig struct {
	Clients     ClientsConfig     `toml:"clients"`
	Connections ConnectionsConfig `toml:"connections"`
	Channels    ChannelsConfig    `toml:"channels"`
	Packets     PacketsConfig     `toml:"packets"`
}

type ClientsConfig struct {
	Enabled        bool `toml:"enabled"`
	Refresh        bool `toml:"refresh"`
	Misbehaviour   bool `toml:"misbehaviour"`
}

type ConnectionsConfig struct {
	Enabled bool `toml:"enabled"`
}

type ChannelsConfig struct {
	Enabled bool `toml:"enabled"`
}

type PacketsConfig struct {
	Enabled      bool `toml:"enabled"`
	ClearOnStart bool `toml:"clear_on_start"`
}

// RestConfig for REST API
type RestConfig struct {
	Enabled bool   `toml:"enabled"`
	Host    string `toml:"host"`
	Port    int    `toml:"port"`
}

// TelemetryConfig for telemetry
type TelemetryConfig struct {
	Enabled bool   `toml:"enabled"`
	Host    string `toml:"host"`
	Port    int    `toml:"port"`
}

// TracingConfig for tracing server
type TracingConfig struct {
	Enabled bool   `toml:"enabled"`
	Host    string `toml:"host"`
	Port    int    `toml:"port"`
}

// ChainConfig represents configuration for a single chain
type ChainConfig struct {
	ID                    string            `toml:"id"`
	Type                  string            `toml:"type"`
	RPCAddr               string            `toml:"rpc_addr"`
	GRPCAddr              string            `toml:"grpc_addr"`
	EventSource           EventSourceConfig `toml:"event_source"`
	RPCTimeout            string            `toml:"rpc_timeout"`
	TrustedNode           bool              `toml:"trusted_node"`
	AccountPrefix         string            `toml:"account_prefix"`
	KeyName               string            `toml:"key_name"`
	KeyStore              string            `toml:"key_store"`
	StorePrefix           string            `toml:"store_prefix"`
	DefaultGas            int               `toml:"default_gas"`
	MaxGas                int               `toml:"max_gas"`
	GasPrice              GasPrice          `toml:"gas_price"`
	GasMultiplier         float64           `toml:"gas_multiplier"`
	MaxMsgNum             int               `toml:"max_msg_num"`
	MaxTxSize             int               `toml:"max_tx_size"`
	ClockDrift            string            `toml:"clock_drift"`
	MaxBlockTime          string            `toml:"max_block_time"`
	TrustingPeriod        string            `toml:"trusting_period"`
	TrustThreshold        TrustThreshold    `toml:"trust_threshold"`
	AddressType           AddressType       `toml:"address_type"`
	Memo                  string            `toml:"memo"`
	ProofSpecs            []ProofSpec       `toml:"proof_specs"`
	Extension             ExtensionOptions  `toml:"extension_options"`
}

type EventSourceConfig struct {
	Mode          string `toml:"mode"`
	URL           string `toml:"url"`
	BatchDelay    string `toml:"batch_delay"`
}

type GasPrice struct {
	Price float64 `toml:"price"`
	Denom string  `toml:"denom"`
}

type TrustThreshold struct {
	Numerator   int `toml:"numerator"`
	Denominator int `toml:"denominator"`
}

type AddressType struct {
	Derivation string `toml:"derivation"`
}

type ProofSpec struct {
	LeafSpec     LeafSpec `toml:"leaf_spec"`
	InnerSpec    InnerSpec `toml:"inner_spec"`
	MaxDepth     int      `toml:"max_depth"`
	MinDepth     int      `toml:"min_depth"`
	PrehashKeyBeforeComparison bool `toml:"prehash_key_before_comparison"`
}

type LeafSpec struct {
	Hash         string `toml:"hash"`
	PrehashKey   string `toml:"prehash_key"`
	PrehashValue string `toml:"prehash_value"`
	Length       string `toml:"length"`
	Prefix       string `toml:"prefix"`
}

type InnerSpec struct {
	ChildOrder      []int  `toml:"child_order"`
	ChildSize       int    `toml:"child_size"`
	MinPrefixLength int    `toml:"min_prefix_length"`
	MaxPrefixLength int    `toml:"max_prefix_length"`
	EmptyChild      string `toml:"empty_child"`
	Hash            string `toml:"hash"`
}

type ExtensionOptions struct {
	ExtensionOptions []ExtensionOption `toml:"extension_options"`
	NonCriticalExtensionOptions []ExtensionOption `toml:"non_critical_extension_options"`
}

type ExtensionOption struct {
	TypeURL string `toml:"type_url"`
	Value   string `toml:"value"`
}

// NewHermesConfig creates a new Hermes configuration from chain configs
func NewHermesConfig(chains []types.ChainConfig) (*HermesConfig, error) {
	hermesChains := make([]ChainConfig, len(chains))
	
	for i, chainCfg := range chains {
		// Parse gas price
		gasPricesStr, err := strconv.ParseFloat(strings.ReplaceAll(chainCfg.GasPrices, chainCfg.Denom, ""), 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse gas prices for chain %s: %w", chainCfg.ChainID, err)
		}
		
		hermesChains[i] = ChainConfig{
			ID:       chainCfg.ChainID,
			Type:     "CosmosSdk", // Default to Cosmos SDK
			RPCAddr:  chainCfg.RPCAddress,
			GRPCAddr: chainCfg.GRPCAddress,
			EventSource: EventSourceConfig{
				Mode:       "push",
				URL:        chainCfg.RPCAddress,
				BatchDelay: "500ms",
			},
			RPCTimeout:    "10s",
			TrustedNode:   true,
			AccountPrefix: chainCfg.Bech32Prefix,
			KeyName:       fmt.Sprintf("relayer-%s", chainCfg.ChainID),
			KeyStore:      "Test",
			StorePrefix:   "ibc",
			DefaultGas:    100000,
			MaxGas:        400000,
			GasPrice: GasPrice{
				Price: gasPricesStr,
				Denom: chainCfg.Denom,
			},
			GasMultiplier: 1.1,
			MaxMsgNum:     30,
			MaxTxSize:     2097152,
			ClockDrift:    "5s",
			MaxBlockTime:  "30s",
			TrustingPeriod: "14days",
			TrustThreshold: TrustThreshold{
				Numerator:   1,
				Denominator: 3,
			},
			AddressType: AddressType{
				Derivation: "cosmos",
			},
			Memo: "",
			ProofSpecs: []ProofSpec{
				{
					LeafSpec: LeafSpec{
						Hash:         "SHA256",
						PrehashKey:   "NO_HASH",
						PrehashValue: "SHA256",
						Length:       "VAR_PROTO",
						Prefix:       "AA==",
					},
					InnerSpec: InnerSpec{
						ChildOrder:      []int{0, 1},
						ChildSize:       33,
						MinPrefixLength: 4,
						MaxPrefixLength: 12,
						EmptyChild:      "",
						Hash:            "SHA256",
					},
					MaxDepth:                   0,
					MinDepth:                   0,
					PrehashKeyBeforeComparison: false,
				},
				{
					LeafSpec: LeafSpec{
						Hash:         "SHA256",
						PrehashKey:   "NO_HASH",
						PrehashValue: "SHA256",
						Length:       "VAR_PROTO",
						Prefix:       "AA==",
					},
					InnerSpec: InnerSpec{
						ChildOrder:      []int{0, 1},
						ChildSize:       32,
						MinPrefixLength: 1,
						MaxPrefixLength: 1,
						EmptyChild:      "",
						Hash:            "SHA256",
					},
					MaxDepth:                   0,
					MinDepth:                   0,
					PrehashKeyBeforeComparison: false,
				},
			},
			Extension: ExtensionOptions{
				ExtensionOptions: []ExtensionOption{
					{
						TypeURL: "/ibc.lightclients.tendermint.v1.ClientState",
						Value:   "",
					},
				},
				NonCriticalExtensionOptions: []ExtensionOption{},
			},
		}
	}
	
	return &HermesConfig{
		Global: GlobalConfig{
			LogLevel: "info",
		},
		Mode: ModeConfig{
			Clients: ClientsConfig{
				Enabled:      true,
				Refresh:      true,
				Misbehaviour: true,
			},
			Connections: ConnectionsConfig{
				Enabled: true,
			},
			Channels: ChannelsConfig{
				Enabled: true,
			},
			Packets: PacketsConfig{
				Enabled:      true,
				ClearOnStart: true,
			},
		},
		Rest: RestConfig{
			Enabled: false,
			Host:    "127.0.0.1",
			Port:    3000,
		},
		Telemetry: TelemetryConfig{
			Enabled: false,
			Host:    "127.0.0.1",
			Port:    3001,
		},
		TracingServer: TracingConfig{
			Enabled: false,
			Host:    "127.0.0.1",
			Port:    5555,
		},
		Chains: hermesChains,
	}, nil
}

// ToTOML converts the Hermes config to TOML
func (c *HermesConfig) ToTOML() ([]byte, error) {
	var buf strings.Builder
	encoder := toml.NewEncoder(&buf)
	err := encoder.Encode(c)
	if err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}