package evmsingle

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Chain is a logical grouping of ev-node-evm-single nodes
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

// Start starts all nodes concurrently and waits for completion
func (c *Chain) Start(ctx context.Context) error {
	nodes := c.Nodes()
	var eg errgroup.Group
	for _, node := range nodes {
		n := node
		eg.Go(func() error {
			return n.Start(ctx)
		})
	}
	return eg.Wait()
}

// Stop stops all nodes concurrently
func (c *Chain) Stop(ctx context.Context) error {
	nodes := c.Nodes()
	var eg errgroup.Group
	for _, node := range nodes {
		n := node
		eg.Go(func() error {
			return n.Stop(ctx)
		})
	}
	return eg.Wait()
}

// Remove removes all nodes concurrently (stopping handled by Node.Remove)
func (c *Chain) Remove(ctx context.Context, opts ...types.RemoveOption) error {
	nodes := c.Nodes()
	var eg errgroup.Group
	for _, node := range nodes {
		n := node
		eg.Go(func() error {
			return n.Remove(ctx, opts...)
		})
	}
	return eg.Wait()
}
