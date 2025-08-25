package dataavailability

import "github.com/celestiaorg/tastora/framework/types"

var _ types.NodeType = (*NodeType)(nil)

// NodeType represents a data availability node type
type NodeType struct {
	nodeTypeString string
}

// String returns the string representation of the NodeType
func (d NodeType) String() string {
	return d.nodeTypeString
}

// Predefined node types
var (
	BridgeNodeType = NodeType{"bridge"}
	LightNodeType  = NodeType{"light"}
	FullNodeType   = NodeType{"full"}
)