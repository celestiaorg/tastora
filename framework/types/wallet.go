package types

type Wallet interface {
	GetKeyName() string
	GetFormattedAddress() string
	GetBech32Prefix() string
}
