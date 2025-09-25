package evmsingle

import "github.com/celestiaorg/tastora/framework/types"

var NodeType types.NodeType = evmSingleNodeType("evm-single")

// defaultPorts returns the default internal container ports for an ev-node-evm-single node.
func defaultPorts() types.Ports {
	return types.Ports{
		RPC: "7331",
		P2P: "7676",
	}
}

// evmSingleNodeType satisfies types.NodeType for container.Node
type evmSingleNodeType string

func (t evmSingleNodeType) String() string { return "evm-single" }
