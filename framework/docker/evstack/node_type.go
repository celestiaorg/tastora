package evstack

import "github.com/celestiaorg/tastora/framework/types"

// NodeType represents an evstack Node
type NodeType struct{}

// String returns the string representation of the NodeType
func (e NodeType) String() string {
	return "evstack"
}

// EvstackType is the singleton instance representing an evstack Node
var EvstackType = NodeType{}

// Interface Compliance Check - ensure NodeType implements the NodeType interface
var _ types.NodeType = (*NodeType)(nil)