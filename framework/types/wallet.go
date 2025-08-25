package types

type Wallet struct {
	Address          []byte
	FormattedAddress string
	KeyName          string
	Bech32Prefix     string
}

func (w *Wallet) GetBech32Prefix() string {
	return w.Bech32Prefix
}

func (w *Wallet) GetKeyName() string {
	return w.KeyName
}

func (w *Wallet) GetFormattedAddress() string {
	return w.FormattedAddress
}

func NewWallet(address []byte, formattedAddress string, bechPrefix string, keyName string) *Wallet {
	return &Wallet{
		Address:          address,
		FormattedAddress: formattedAddress,
		KeyName:          keyName,
		Bech32Prefix:     bechPrefix,
	}
}
