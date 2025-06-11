package docker

import (
	"context"
	"fmt"
	"sync"

	"github.com/celestiaorg/tastora/framework/types"
)

var _ types.DataAvailabilityNetwork = &DataAvailabilityNetwork{}

// newDataAvailabilityNetwork initializes a DataAvailabilityNetwork using the provided context, testName, and configuration.
// Returns a pointer to DataAvailabilityNetwork and an error if initialization fails.
// It creates DANodes based on the configuration.
func newDataAvailabilityNetwork(ctx context.Context, testName string, cfg Config) (*DataAvailabilityNetwork, error) {
	if cfg.DataAvailabilityNetworkConfig == nil {
		return nil, fmt.Errorf("data availability network config is nil")
	}

	daNodes, err := createDANodes(ctx, testName, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create data availability network nodes: %w", err)
	}

	return &DataAvailabilityNetwork{
		cfg:     cfg,
		daNodes: daNodes,
	}, nil
}

// DataAvailabilityNetwork represents a docker network containing multiple nodes.
// It manages the lifecycle and interaction of nodes, including their addition and retrieval by specific types.
// It ensures thread-safe operations with mutex locking for concurrent access to its DANodes.
type DataAvailabilityNetwork struct {
	mu      sync.Mutex
	cfg     Config
	daNodes []*DANode
}

// GetBridgeNodes retrieves all nodes of type BridgeNode from the DataAvailabilityNetwork.
func (d *DataAvailabilityNetwork) GetBridgeNodes() []types.DANode {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.getNodesOfType(types.BridgeNode)
}

// GetLightNodes retrieves all nodes of type LightNode from the DataAvailabilityNetwork.
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

// createDANodes initializes and returns a list of DANodes (bridge, full, and light nodes) based on the given configuration.
// It returns an error if any node creation fails.
func createDANodes(ctx context.Context, testName string, cfg Config) ([]*DANode, error) {
	var daNodes []*DANode
	for _, nodeType := range []struct {
		count int
		typ   types.DANodeType
	}{
		{cfg.DataAvailabilityNetworkConfig.BridgeNodeCount, types.BridgeNode},
		{cfg.DataAvailabilityNetworkConfig.LightNodeCount, types.LightNode},
	} {
		for i := 0; i < nodeType.count; i++ {
			n, err := newDANode(ctx, testName, cfg, i, nodeType.typ)
			if err != nil {
				return nil, fmt.Errorf("failed to create %s node: %w", nodeType.typ, err)
			}
			daNodes = append(daNodes, n)
		}
	}
	return daNodes, nil
}
