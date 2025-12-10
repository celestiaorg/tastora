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
	Address QuotedHexAddress `yaml:"address"`
	Owner   QuotedHexAddress `yaml:"owner"`
}

// HookCfg models a generic hook address + type.
type HookCfg struct {
	Address QuotedHexAddress `yaml:"address"`
	Type    string           `yaml:"type"`
}

// RequiredHookCfg models the required/protocol fee hook configuration.
type RequiredHookCfg struct {
	Address        QuotedHexAddress `yaml:"address"`
	Beneficiary    QuotedHexAddress `yaml:"beneficiary"`
	MaxProtocolFee string           `yaml:"maxProtocolFee"`
	Owner          QuotedHexAddress `yaml:"owner"`
	ProtocolFee    string           `yaml:"protocolFee"`
	Type           string           `yaml:"type"`
}

// InterchainAccountRouterCfg models the Interchain Account Router settings.
type InterchainAccountRouterCfg struct {
	Address          QuotedHexAddress  `yaml:"address"`
	Mailbox          QuotedHexAddress  `yaml:"mailbox"`
	Owner            QuotedHexAddress  `yaml:"owner"`
	ProxyAdmin       ProxyAdminCfg     `yaml:"proxyAdmin"`
	RemoteIcaRouters map[string]string `yaml:"remoteIcaRouters"`
}

// CoreConfig is the top-level structure for core-config.yaml
type CoreConfig struct {
	DefaultHook             HookCfg                    `yaml:"defaultHook"`
	DefaultIsm              HookCfg                    `yaml:"defaultIsm"`
	InterchainAccountRouter InterchainAccountRouterCfg `yaml:"interchainAccountRouter"`
	Owner                   QuotedHexAddress           `yaml:"owner"`
	ProxyAdmin              ProxyAdminCfg              `yaml:"proxyAdmin"`
	RequiredHook            RequiredHookCfg            `yaml:"requiredHook"`
}
