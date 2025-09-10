package dataavailability

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"sort"
	"sync"

	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
)

// Network is a docker implementation of a data availability network.
type Network struct {
	cfg         Config
	log         *zap.Logger
	nodeMap     map[string]*Node // map from node name to node
	nextNodeIdx int              // incrementing index for unique container names
	testName    string           // original test name for unique container naming
	mu          sync.Mutex
}

// GetNodes returns all nodes in the data availability network.
func (n *Network) GetNodes() []*Node {
	n.mu.Lock()
	defer n.mu.Unlock()

	nodes := make([]*Node, 0, len(n.nodeMap))
	for _, node := range n.nodeMap {
		nodes = append(nodes, node)
	}

	// Sort nodes by name for consistent ordering
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name() < nodes[j].Name()
	})

	return nodes
}

// GetNodesByType returns nodes filtered by the specified node type.
func (n *Network) GetNodesByType(nodeType types.DANodeType) []*Node {
	n.mu.Lock()
	defer n.mu.Unlock()

	var filtered []*Node
	for _, node := range n.nodeMap {
		if node.nodeType == nodeType {
			filtered = append(filtered, node)
		}
	}

	// Sort nodes by name for consistent ordering
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Name() < filtered[j].Name()
	})

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

// AddNode adds a single node to the DA network with the given configuration.
// The node is created and initialized but not started - call Start() on the returned node to start it.
func (n *Network) AddNode(ctx context.Context, nodeConfig NodeConfig) (*Node, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	nodeIndex := n.nextNodeIdx

	// node index is always unique and incrementing and does not decrement if nodes are removed.
	// this ensures all created containers are given unique names.
	n.nextNodeIdx++

	// create a builder that will build nodes associated with this network.
	builder := NewNetworkBuilderFromNetwork(n)
	node, err := builder.newNode(ctx, nodeConfig, nodeIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to create node: %w", err)
	}

	// node names will be unique due to the incrementing index
	n.nodeMap[node.Name()] = node

	return node, nil
}

// RemoveNode removes a node from the DA network by name, stopping and cleaning up its resources.
func (n *Network) RemoveNode(ctx context.Context, nodeNames ...string) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(nodeNames) == 0 {
		return fmt.Errorf("at least one node name must be provided")
	}

	for _, nodeName := range nodeNames {
		if _, exists := n.nodeMap[nodeName]; !exists {
			return fmt.Errorf("node with name %s not found in network", nodeName)
		}
	}

	eg, egCtx := errgroup.WithContext(ctx)
	for _, nodeName := range nodeNames {
		node := n.nodeMap[nodeName]
		// remove all nodes concurrently
		eg.Go(func() error {
			return node.Stop(egCtx)
		})
	}

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("failed to remove nodes: %w", err)
	}

	// remove all stopped nodes from the internal mapping.
	for _, nodeName := range nodeNames {
		delete(n.nodeMap, nodeName)
	}

	return nil
}
