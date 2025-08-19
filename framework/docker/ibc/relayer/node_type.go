package relayer

import "github.com/celestiaorg/tastora/framework/types"

// HermesNodeType represents a Hermes IBC relayer node
type HermesNodeType struct{}

// String returns the string representation of the HermesNodeType
func (h HermesNodeType) String() string {
	return "hermes"
}

// HermesRelayer is the singleton instance representing a Hermes IBC relayer node
var HermesRelayer = HermesNodeType{}

// Interface Compliance Check - ensure HermesNodeType implements the NodeType interface
var _ types.NodeType = (*HermesNodeType)(nil)
