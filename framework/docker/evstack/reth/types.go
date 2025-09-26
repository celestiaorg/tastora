package reth

import "github.com/celestiaorg/tastora/framework/types"

var NodeType types.NodeType = rethNodeType("reth")

// defaultInternalPorts returns the default internal container ports for a Reth node.
func defaultInternalPorts() types.Ports {
	return types.Ports{
		Metrics: "9001",
		P2P:     "30303",
		RPC:     "8545",
		Engine:  "8551",
		API:     "8546", // WS
	}
}

// rethNodeType satisfies types.NodeType for container.Node
type rethNodeType string

func (t rethNodeType) String() string { return string(t) }
