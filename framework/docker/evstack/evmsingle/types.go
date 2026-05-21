package evmsingle

import "github.com/celestiaorg/tastora/framework/types"

var NodeType types.NodeType = evmSingleNodeType("evm-single")

const (
	defaultRPCPort = "7331"
	defaultP2PPort = "7676"
)

// defaultPorts returns the default internal container ports for an ev-node-evm-single node.
func defaultPorts() types.Ports {
	return types.Ports{
		RPC: defaultRPCPort,
		P2P: defaultP2PPort,
	}
}

// evmSingleNodeType satisfies types.NodeType for container.Node
type evmSingleNodeType string

func (t evmSingleNodeType) String() string { return string(t) }
