package relayer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/celestiaorg/tastora/framework/types"
)

// ConfigOption defines a function type for configuring HermesConfig.
type ConfigOption func(*HermesConfig)

// HermesConfig represents the full Hermes configuration
type HermesConfig struct {
	Global        GlobalConfig    `toml:"global"`
	Mode          ModeConfig      `toml:"mode"`
	Rest          RestConfig      `toml:"rest"`
	Telemetry     TelemetryConfig `toml:"telemetry"`
	TracingServer TracingConfig   `toml:"tracing_server"`
	Chains        []ChainConfig   `toml:"chains"`
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
	Enabled      bool `toml:"enabled"`
	Refresh      bool `toml:"refresh"`
	Misbehaviour bool `toml:"misbehaviour"`
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
	ID               string                 `toml:"id"`
	Type             string                 `toml:"type"`
	RPCAddr          string                 `toml:"rpc_addr"`
	GRPCAddr         string                 `toml:"grpc_addr"`
	EventSource      map[string]interface{} `toml:"event_source"`
	RPCTimeout       string                 `toml:"rpc_timeout"`
	TrustedNode      bool                   `toml:"trusted_node"`
	CCVConsumerChain bool                   `toml:"ccv_consumer_chain"`
	AccountPrefix    string                 `toml:"account_prefix"`
	KeyName          string                 `toml:"key_name"`
	KeyStoreType     string                 `toml:"key_store_type"`
	StorePrefix      string                 `toml:"store_prefix"`
	DefaultGas       int                    `toml:"default_gas"`
	MaxGas           int                    `toml:"max_gas"`
	GasPrice         GasPrice               `toml:"gas_price"`
	GasMultiplier    float64                `toml:"gas_multiplier"`
	MaxMsgNum        int                    `toml:"max_msg_num"`
	MaxTxSize        int                    `toml:"max_tx_size"`
	ClockDrift       string                 `toml:"clock_drift"`
	MaxBlockTime     string                 `toml:"max_block_time"`
	TrustingPeriod   string                 `toml:"trusting_period"`
	TrustThreshold   TrustThreshold         `toml:"trust_threshold"`
	AddressType      AddressType            `toml:"address_type"`
	MemoPrefix       string                 `toml:"memo_prefix"`
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

// NewHermesConfig creates a new Hermes configuration from chain configs
func NewHermesConfig(chains []types.ChainRelayerConfig) (*HermesConfig, error) {
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
			EventSource: map[string]interface{}{
				"mode":        "push",
				"url":         strings.Replace(chainCfg.RPCAddress, "http://", "ws://", 1) + "/websocket",
				"batch_delay": "500ms",
			},
			RPCTimeout:       "10s",
			TrustedNode:      true,
			CCVConsumerChain: false,
			AccountPrefix:    chainCfg.Bech32Prefix,
			KeyName:          fmt.Sprintf("relayer-%s", chainCfg.ChainID),
			KeyStoreType:     "Test",
			StorePrefix:      "ibc",
			DefaultGas:       100000,
			MaxGas:           400000,
			GasPrice: GasPrice{
				Price: gasPricesStr,
				Denom: chainCfg.Denom,
			},
			GasMultiplier:  1.1,
			MaxMsgNum:      30,
			MaxTxSize:      2097152,
			ClockDrift:     "5s",
			MaxBlockTime:   "30s",
			TrustingPeriod: "14days",
			TrustThreshold: TrustThreshold{
				Numerator:   1,
				Denominator: 3,
			},
			AddressType: AddressType{
				Derivation: "cosmos",
			},
			MemoPrefix: "",
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
