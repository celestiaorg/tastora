package hyperlane

// ProxyAdminCfg models the proxy admin address and owner fields.
type ProxyAdminCfg struct {
	Address string `yaml:"address"`
	Owner   string `yaml:"owner"`
}

// HookCfg models a generic hook address + type.
type HookCfg struct {
	Address string `yaml:"address"`
	Type    string `yaml:"type"`
}

// RequiredHookCfg models the required/protocol fee hook configuration.
type RequiredHookCfg struct {
	Address        string `yaml:"address"`
	Beneficiary    string `yaml:"beneficiary"`
	MaxProtocolFee string `yaml:"maxProtocolFee"`
	Owner          string `yaml:"owner"`
	ProtocolFee    string `yaml:"protocolFee"`
	Type           string `yaml:"type"`
}

// InterchainAccountRouterCfg models the Interchain Account Router settings.
type InterchainAccountRouterCfg struct {
	Address          string            `yaml:"address"`
	Mailbox          string            `yaml:"mailbox"`
	Owner            string            `yaml:"owner"`
	ProxyAdmin       ProxyAdminCfg     `yaml:"proxyAdmin"`
	RemoteIcaRouters map[string]string `yaml:"remoteIcaRouters"`
}

// CoreConfig is the top-level structure for core-config.yaml
type CoreConfig struct {
	DefaultHook             HookCfg                    `yaml:"defaultHook"`
	DefaultIsm              HookCfg                    `yaml:"defaultIsm"`
	InterchainAccountRouter InterchainAccountRouterCfg `yaml:"interchainAccountRouter"`
	Owner                   string                     `yaml:"owner"`
	ProxyAdmin              ProxyAdminCfg              `yaml:"proxyAdmin"`
	RequiredHook            RequiredHookCfg            `yaml:"requiredHook"`
}
