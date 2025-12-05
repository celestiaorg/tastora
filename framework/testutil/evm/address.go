package evm

import (
	hyputil "github.com/bcp-innovations/hyperlane-cosmos/util"
	gethcommon "github.com/ethereum/go-ethereum/common"
)

// PadEVMAddress returns a 32-byte Hyperlane HexAddress from a 20-byte EVM
// address value by left-padding with zeros. This mirrors the common pattern:
//
//	var padded [32]byte; copy(padded[12:], evm.Bytes())
func PadEVMAddress(addr gethcommon.Address) hyputil.HexAddress {
	var out [32]byte
	copy(out[12:], addr.Bytes())
	return hyputil.HexAddress(out[:])
}
