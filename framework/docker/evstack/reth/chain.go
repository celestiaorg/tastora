package reth

import (
    "context"
    "fmt"
    "sort"
    "sync"

    gethrpc "github.com/ethereum/go-ethereum/rpc"
    "github.com/ethereum/go-ethereum/ethclient"
    "github.com/celestiaorg/tastora/framework/types"
    "go.uber.org/zap"
)

// Chain is a logical grouping of Reth nodes
type Chain struct {
    cfg       Config
    log       *zap.Logger
    testName  string

    mu        sync.Mutex
    nodes     map[string]*Node
    nextIndex int
}

// GetNodes returns nodes sorted by name for consistent ordering
func (c *Chain) GetNodes() []*Node {
    c.mu.Lock(); defer c.mu.Unlock()
    out := make([]*Node, 0, len(c.nodes))
    for _, n := range c.nodes { out = append(out, n) }
    sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
    return out
}

// AddNodes creates nodes with the provided configs but does not start them
func (c *Chain) AddNodes(ctx context.Context, nodeConfigs ...NodeConfig) ([]*Node, error) {
    if len(nodeConfigs) == 0 { return nil, fmt.Errorf("at least one node config required") }

    c.mu.Lock(); start := c.nextIndex; c.nextIndex += len(nodeConfigs); c.mu.Unlock()

    created := make([]*Node, 0, len(nodeConfigs))
    for i, nc := range nodeConfigs {
        idx := start + i
        n := newNode(c.cfg, c.testName, idx, nc)
        created = append(created, n)
        c.mu.Lock()
        if c.nodes == nil { c.nodes = make(map[string]*Node) }
        c.nodes[n.Name()] = n
        c.mu.Unlock()
    }
    return created, nil
}

// StartAll starts all nodes
func (c *Chain) StartAll(ctx context.Context) error {
    for _, n := range c.GetNodes() { if err := n.Start(ctx); err != nil { return err } }
    return nil
}

// StopAll stops all nodes
func (c *Chain) StopAll(ctx context.Context) error {
    var firstErr error
    for _, n := range c.GetNodes() { if err := n.Stop(ctx); err != nil && firstErr == nil { firstErr = err } }
    return firstErr
}

// RemoveAll removes all nodes (stopping first) with optional removal options (e.g., preserve volumes)
func (c *Chain) RemoveAll(ctx context.Context, opts ...types.RemoveOption) error {
    var firstErr error
    for _, n := range c.GetNodes() { if err := n.Remove(ctx, opts...); err != nil && firstErr == nil { firstErr = err } }
    return firstErr
}

// GetRPCClient returns a go-ethereum RPC client for the first node in the chain.
func (c *Chain) GetRPCClient(ctx context.Context) (*gethrpc.Client, error) {
    nodes := c.GetNodes()
    if len(nodes) == 0 { return nil, fmt.Errorf("no reth nodes in chain") }
    return nodes[0].GetRPCClient(ctx)
}

// GetEthClient returns a go-ethereum ethclient.Client for the first node in the chain.
func (c *Chain) GetEthClient(ctx context.Context) (*ethclient.Client, error) {
    nodes := c.GetNodes()
    if len(nodes) == 0 { return nil, fmt.Errorf("no reth nodes in chain") }
    return nodes[0].GetEthClient(ctx)
}

// GetClient returns a bound Ethereum JSON-RPC client for the first node in the
// chain. Returns an error if the chain has no nodes or the node is not started.
// go-ethereum clients are exposed under build tag with_geth (see geth_chain_client.go)
