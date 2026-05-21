package reth

import "github.com/celestiaorg/tastora/framework/types"

var NodeType types.NodeType = rethNodeType("reth")

const (
	defaultMetricsPort = "9001"
	defaultP2PPort     = "30303"
	defaultRPCPort     = "8545"
	defaultEnginePort  = "8551"
	defaultWSPort      = "8546"
)

// defaultInternalPorts returns the default internal container ports for a Reth node.
func defaultInternalPorts() types.Ports {
	return types.Ports{
		Metrics: defaultMetricsPort,
		P2P:     defaultP2PPort,
		RPC:     defaultRPCPort,
		Engine:  defaultEnginePort,
		API:     defaultWSPort,
	}
}

// rethNodeType satisfies types.NodeType for container.Node
type rethNodeType string

func (t rethNodeType) String() string { return string(t) }
