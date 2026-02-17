package evm

import (
	hyputil "github.com/bcp-innovations/hyperlane-cosmos/util"
	gethcommon "github.com/ethereum/go-ethereum/common"
)

// PadAddress returns a 32-byte Hyperlane HexAddress from a 20-byte EVM
// address value by left-padding with zeros.
func PadAddress(addr gethcommon.Address) hyputil.HexAddress {
	var out [32]byte
	copy(out[12:], addr.Bytes())
	return hyputil.HexAddress(out[:])
}
