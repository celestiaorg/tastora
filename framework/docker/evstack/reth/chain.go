package reth

import (
	"context"
	"fmt"
	"golang.org/x/sync/errgroup"
	"sort"
	"sync"

	"github.com/celestiaorg/tastora/framework/types"
	"github.com/ethereum/go-ethereum/ethclient"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
	"go.uber.org/zap"
)

// Chain is a logical grouping of Reth nodes
type Chain struct {
	cfg      Config
	log      *zap.Logger
	testName string

	mu        sync.Mutex
	nodes     map[string]*Node
	nextIndex int
}

// Nodes returns nodes sorted by name for consistent ordering
func (c *Chain) Nodes() []*Node {
	c.mu.Lock()
	defer c.mu.Unlock()

	out := make([]*Node, 0, len(c.nodes))
	for _, n := range c.nodes {
		out = append(out, n)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})

	return out
}

// AddNodes creates nodes with the provided configs but does not start them
func (c *Chain) AddNodes(ctx context.Context, nodeConfigs ...NodeConfig) ([]*Node, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(nodeConfigs) == 0 {
		return nil, fmt.Errorf("at least one node config required")
	}

	start := c.nextIndex
	c.nextIndex += len(nodeConfigs)

	created := make([]*Node, 0, len(nodeConfigs))
	for i, nc := range nodeConfigs {
		idx := start + i
		n, err := newNode(ctx, c.cfg, c.testName, idx, nc)
		if err != nil {
			return nil, err
		}
		created = append(created, n)
		if c.nodes == nil {
			c.nodes = make(map[string]*Node)
		}
		c.nodes[n.Name()] = n
	}
	return created, nil
}

// Start starts all nodes in the chain.
func (c *Chain) Start(ctx context.Context) error {
	var eg errgroup.Group
	for _, n := range c.Nodes() {
		n := n
		eg.Go(func() error {
			return n.Start(ctx)
		})
	}
	return eg.Wait()
}

// Stop stops all nodes in the chain.
func (c *Chain) Stop(ctx context.Context) error {
	var eg errgroup.Group
	for _, n := range c.Nodes() {
		n := n
		eg.Go(func() error {
			return n.Stop(ctx)
		})
	}
	return eg.Wait()
}

// Remove removes all nodes in the Chain.
func (c *Chain) Remove(ctx context.Context, opts ...types.RemoveOption) error {
	var eg errgroup.Group
	for _, n := range c.Nodes() {
		n := n
		eg.Go(func() error {
			return n.Remove(ctx, opts...)
		})
	}
	return eg.Wait()
}

// GetRPCClient returns a go-ethereum RPC client for the first node in the chain.
func (c *Chain) GetRPCClient(ctx context.Context) (*gethrpc.Client, error) {
	nodes := c.Nodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no reth nodes in chain")
	}
	return nodes[0].GetRPCClient(ctx)
}

// GetEthClient returns a go-ethereum ethclient.Client for the first node in the chain.
func (c *Chain) GetEthClient(ctx context.Context) (*ethclient.Client, error) {
	nodes := c.Nodes()
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no reth nodes in chain")
	}
	return nodes[0].GetEthClient(ctx)
}
