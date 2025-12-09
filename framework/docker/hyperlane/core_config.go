package hyperlane

import (
	"gopkg.in/yaml.v3"
)

type QuotedString string

func (qs QuotedString) MarshalYAML() (interface{}, error) {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: string(qs),
		Style: yaml.DoubleQuotedStyle,
	}, nil
}

// ProxyAdminCfg models the proxy admin address and owner fields.
type ProxyAdminCfg struct {
	Address QuotedString `yaml:"address"`
	Owner   QuotedString `yaml:"owner"`
}

// HookCfg models a generic hook address + type.
type HookCfg struct {
	Address QuotedString `yaml:"address"`
	Type    string       `yaml:"type"`
}

// RequiredHookCfg models the required/protocol fee hook configuration.
type RequiredHookCfg struct {
	Address        QuotedString `yaml:"address"`
	Beneficiary    QuotedString `yaml:"beneficiary"`
	MaxProtocolFee string       `yaml:"maxProtocolFee"`
	Owner          QuotedString `yaml:"owner"`
	ProtocolFee    string       `yaml:"protocolFee"`
	Type           string       `yaml:"type"`
}

// InterchainAccountRouterCfg models the Interchain Account Router settings.
type InterchainAccountRouterCfg struct {
	Address          QuotedString      `yaml:"address"`
	Mailbox          QuotedString      `yaml:"mailbox"`
	Owner            QuotedString      `yaml:"owner"`
	ProxyAdmin       ProxyAdminCfg     `yaml:"proxyAdmin"`
	RemoteIcaRouters map[string]string `yaml:"remoteIcaRouters"`
}

// CoreConfig is the top-level structure for core-config.yaml
type CoreConfig struct {
	DefaultHook             HookCfg                    `yaml:"defaultHook"`
	DefaultIsm              HookCfg                    `yaml:"defaultIsm"`
	InterchainAccountRouter InterchainAccountRouterCfg `yaml:"interchainAccountRouter"`
	Owner                   QuotedString               `yaml:"owner"`
	ProxyAdmin              ProxyAdminCfg              `yaml:"proxyAdmin"`
	RequiredHook            RequiredHookCfg            `yaml:"requiredHook"`
}
