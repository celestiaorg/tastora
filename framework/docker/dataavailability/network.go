package dataavailability

import (
	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
)

// Network is a docker implementation of a data availability network.
type Network struct {
	cfg   Config
	log   *zap.Logger
	nodes []*Node
}

// GetNodes returns all nodes in the data availability network.
func (n *Network) GetNodes() []*Node {
	return n.nodes
}

// GetNodesByType returns nodes filtered by the specified node type.
func (n *Network) GetNodesByType(nodeType types.DANodeType) []*Node {
	return filterNodes(n.nodes, func(node *Node) bool {
		return node.nodeType == nodeType
	})
}

// filterNodes returns a new slice containing only nodes that satisfy the predicate function.
func filterNodes(nodes []*Node, predicate func(*Node) bool) []*Node {
	var filtered []*Node
	for _, node := range nodes {
		if predicate(node) {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// GetBridgeNodes returns only the bridge nodes in the network.
func (n *Network) GetBridgeNodes() []*Node {
	return n.GetNodesByType(types.BridgeNode)
}

// GetFullNodes returns only the full nodes in the network.
func (n *Network) GetFullNodes() []*Node {
	return n.GetNodesByType(types.FullNode)
}

// GetLightNodes returns only the light nodes in the network.
func (n *Network) GetLightNodes() []*Node {
	return n.GetNodesByType(types.LightNode)
}