package evmsingle

import (
    "context"
    "fmt"
    "sort"
    "sync"

    "github.com/celestiaorg/tastora/framework/types"
    "go.uber.org/zap"
)

// App is a logical grouping of ev-node-evm-single nodes
type App struct {
    cfg      Config
    log      *zap.Logger
    testName string

    mu        sync.Mutex
    nodes     map[string]*Node
    nextIndex int
}

// GetNodes returns nodes sorted by name for consistent ordering
func (a *App) GetNodes() []*Node {
    a.mu.Lock()
    defer a.mu.Unlock()

    out := make([]*Node, 0, len(a.nodes))
    for _, n := range a.nodes {
        out = append(out, n)
    }
    sort.Slice(out, func(i, j int) bool {
        return out[i].Name() < out[j].Name()
    })
    return out
}

// AddNodes creates nodes with the provided configs but does not start them
func (a *App) AddNodes(ctx context.Context, nodeConfigs ...NodeConfig) ([]*Node, error) {
    if len(nodeConfigs) == 0 { return nil, fmt.Errorf("at least one node config required") }

    a.mu.Lock()
    start := a.nextIndex
    a.nextIndex += len(nodeConfigs)
    a.mu.Unlock()

    created := make([]*Node, 0, len(nodeConfigs))
    for i, nc := range nodeConfigs {
        idx := start + i
        n := newNode(a.cfg, a.testName, idx, nc)
        created = append(created, n)
        a.mu.Lock()
        if a.nodes == nil { a.nodes = make(map[string]*Node) }
        a.nodes[n.Name()] = n
        a.mu.Unlock()
    }
    return created, nil
}

// StartAll starts all nodes
func (a *App) StartAll(ctx context.Context) error {
    for _, n := range a.GetNodes() {
        if err := n.Start(ctx); err != nil {
            return err
        }
    }
    return nil
}

// Start starts all nodes (preferred over StartAll for consistency with other types)
func (a *App) Start(ctx context.Context) error { return a.StartAll(ctx) }

// StopAll stops all nodes
func (a *App) StopAll(ctx context.Context) error {
    var firstErr error
    for _, n := range a.GetNodes() {
        if err := n.Stop(ctx); err != nil && firstErr == nil {
            firstErr = err
        }
    }
    return firstErr
}

// Stop stops all nodes (preferred over StopAll for consistency with other types)
func (a *App) Stop(ctx context.Context) error { return a.StopAll(ctx) }

// RemoveAll removes all nodes (stopping first) with optional removal options (e.g., preserve volumes)
func (a *App) Remove(ctx context.Context, opts ...types.RemoveOption) error {
    var firstErr error
    for _, n := range a.GetNodes() {
        if err := n.Remove(ctx, opts...); err != nil && firstErr == nil {
            firstErr = err
        }
    }
    return firstErr
}
