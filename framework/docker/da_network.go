package docker

import (
	"github.com/celestiaorg/tastora/framework/types"
	"sync"
)

var _ types.DataAvailabilityNetwork = &DataAvailabilityNetwork{}

func NewDataAvailabilityNetwork() *DataAvailabilityNetwork {
	return &DataAvailabilityNetwork{
		daNodes: []*DANode{},
	}
}

// DataAvailabilityNetwork represents a docker network containing multiple nodes.
// It manages the lifecycle and interaction of nodes, including their addition and retrieval by specific types.
// It ensures thread-safe operations with mutex locking for concurrent access to its DANodes.
type DataAvailabilityNetwork struct {
	mu      sync.Mutex
	daNodes []*DANode
}

func (d *DataAvailabilityNetwork) GetBridgeNodes() []types.DANode {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.getNodesOfType(types.BridgeNode)
}

func (d *DataAvailabilityNetwork) GetFullNodes() []types.DANode {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.getNodesOfType(types.FullNode)
}

func (d *DataAvailabilityNetwork) GetLightNodes() []types.DANode {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.getNodesOfType(types.LightNode)
}

// getNodesOfType retrieves all nodes from the network of the specified type.
func (d *DataAvailabilityNetwork) getNodesOfType(typ types.DANodeType) []types.DANode {
	var daNodes []types.DANode
	for _, n := range d.daNodes {
		if n.GetType() == typ {
			daNodes = append(daNodes, n)
		}
	}
	return daNodes
}
