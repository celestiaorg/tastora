package docker

import (
	"github.com/celestiaorg/tastora/framework/types"
	"sync"
)

var _ types.DANetwork = &DANetwork{}

func NewDANetwork(nodes ...*DANode) *DANetwork {
	return &DANetwork{
		daNodes: nodes,
	}
}

// DANetwork represents a docker network containing multiple nodes.
// It manages the lifecycle and interaction of nodes, including their addition and retrieval by specific types.
// It ensures thread-safe operations with mutex locking for concurrent access to its DANodes.
type DANetwork struct {
	mu      sync.Mutex
	daNodes []*DANode
}

func (d *DANetwork) GetBridgeNodes() []types.DANode {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.getNodesOfType(types.BridgeNode)
}

func (d *DANetwork) GetFullNodes() []types.DANode {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.getNodesOfType(types.FullNode)
}

func (d *DANetwork) GetLightNodes() []types.DANode {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.getNodesOfType(types.LightNode)
}

// getNodesOfType retrieves all nodes from the network of the specified type.
func (d *DANetwork) getNodesOfType(typ types.DANodeType) []types.DANode {
	var daNodes []types.DANode
	for _, n := range d.daNodes {
		if n.GetType() == typ {
			daNodes = append(daNodes, n)
		}
	}
	return daNodes
}
