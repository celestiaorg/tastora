package config

type CoreConfig struct {
	DefaultHook             Hook                     `yaml:"defaultHook"`
	DefaultIsm              Ism                      `yaml:"defaultIsm"`
	InterchainAccountRouter InterchainAccountRouter  `yaml:"interchainAccountRouter"`
	Owner                   string                   `yaml:"owner"`
	ProxyAdmin              ProxyAdmin               `yaml:"proxyAdmin"`
	RequiredHook            RequiredHook             `yaml:"requiredHook"`
}

type Hook struct {
	Address string `yaml:"address"`
	Type    string `yaml:"type"`
}

type Ism struct {
	Address string `yaml:"address"`
	Type    string `yaml:"type"`
}

type InterchainAccountRouter struct {
	Address          string                `yaml:"address"`
	Mailbox          string                `yaml:"mailbox"`
	Owner            string                `yaml:"owner"`
	ProxyAdmin       ProxyAdmin            `yaml:"proxyAdmin"`
	RemoteIcaRouters map[string]string     `yaml:"remoteIcaRouters"`
}

type ProxyAdmin struct {
	Address string `yaml:"address"`
	Owner   string `yaml:"owner"`
}

type RequiredHook struct {
	Address        string `yaml:"address"`
	Beneficiary    string `yaml:"beneficiary"`
	MaxProtocolFee string `yaml:"maxProtocolFee"`
	Owner          string `yaml:"owner"`
	ProtocolFee    string `yaml:"protocolFee"`
	Type           string `yaml:"type"`
}

type WarpConfig map[string]*WarpRouteConfig

type WarpRouteConfig struct {
	Type                       string `yaml:"type"`
	Token                      string `yaml:"token,omitempty"`
	Owner                      string `yaml:"owner,omitempty"`
	Mailbox                    string `yaml:"mailbox"`
	InterchainSecurityModule   string `yaml:"interchainSecurityModule"`
	InterchainGasPaymaster     string `yaml:"interchainGasPaymaster,omitempty"`
	IsNft                      bool   `yaml:"isNft,omitempty"`
	Name                       string `yaml:"name,omitempty"`
	Symbol                     string `yaml:"symbol,omitempty"`
	Decimals                   int    `yaml:"decimals,omitempty"`
	TotalSupply                string `yaml:"totalSupply,omitempty"`
}
