package hyperlane

import (
	"gopkg.in/yaml.v3"
)

// QuotedHexAddress is required to marshal hex addresses to be written as strings, and not interpreted as numbers.
type QuotedHexAddress string

func (qs QuotedHexAddress) MarshalYAML() (interface{}, error) {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: string(qs),
		Style: yaml.DoubleQuotedStyle,
	}, nil
}

// ProxyAdminCfg models the proxy admin address and owner fields.
type ProxyAdminCfg struct {
	Address QuotedHexAddress `yaml:"address,omitempty"`
	Owner   QuotedHexAddress `yaml:"owner,omitempty"`
}

// HookCfg models a generic hook address + type.
type HookCfg struct {
	Address QuotedHexAddress `yaml:"address,omitempty"`
	Type    string           `yaml:"type"`
}

type TestIsmCfg struct {
	Address QuotedHexAddress `yaml:"address,omitempty"`
	Type    string           `yaml:"type"`
}

type MerkleTreeHookCfg struct {
	Address QuotedHexAddress `yaml:"address,omitempty"`
	Type    string           `yaml:"type"`
}

// ProtocolFeeHookCfg models the protocol fee hook configuration.
type ProtocolFeeHookCfg struct {
	Address        QuotedHexAddress `yaml:"address,omitempty"`
	Beneficiary    QuotedHexAddress `yaml:"beneficiary"`
	MaxProtocolFee string           `yaml:"maxProtocolFee"`
	Owner          QuotedHexAddress `yaml:"owner"`
	ProtocolFee    string           `yaml:"protocolFee"`
	Type           string           `yaml:"type"`
}

// InterchainAccountRouterCfg models the Interchain Account Router settings.
type InterchainAccountRouterCfg struct {
	Address          QuotedHexAddress  `yaml:"address,omitempty"`
	Mailbox          QuotedHexAddress  `yaml:"mailbox,omitempty"`
	Owner            QuotedHexAddress  `yaml:"owner,omitempty"`
	ProxyAdmin       ProxyAdminCfg     `yaml:"proxyAdmin,omitempty"`
	RemoteIcaRouters map[string]string `yaml:"remoteIcaRouters,omitempty"`
}

// CoreConfig is the top-level structure for core-config.yaml
type CoreConfig struct {
	DefaultHook             ProtocolFeeHookCfg         `yaml:"defaultHook"`
	RequiredHook            MerkleTreeHookCfg          `yaml:"requiredHook"`
	DefaultIsm              TestIsmCfg                 `yaml:"defaultIsm"`
	InterchainAccountRouter InterchainAccountRouterCfg `yaml:"interchainAccountRouter,omitempty"`
	Owner                   QuotedHexAddress           `yaml:"owner"`
	ProxyAdmin              ProxyAdminCfg              `yaml:"proxyAdmin,omitempty"`
}
