package sdkacc

import (
	"fmt"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	"strings"
)

// AddressToBech32 converts an sdk address to a string, the prefix must be supplied.
func AddressToBech32(addr sdk.AccAddress, prefix string) (string, error) {
	return bech32.ConvertAndEncode(prefix, addr)
}

// AddressFromWallet returns an sdk address from a wallet.
func AddressFromWallet(wallet *types.Wallet) (sdk.AccAddress, error) {
	if len(strings.TrimSpace(wallet.GetFormattedAddress())) == 0 {
		return sdk.AccAddress{}, fmt.Errorf("empty address string is not allowed")
	}

	bz, err := sdk.GetFromBech32(wallet.GetFormattedAddress(), wallet.GetBech32Prefix())
	if err != nil {
		return nil, err
	}

	err = sdk.VerifyAddressFormat(bz)
	if err != nil {
		return nil, err
	}

	return bz, nil
}
