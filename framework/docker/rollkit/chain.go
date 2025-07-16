package rollkit

import (
	"context"
	"fmt"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

// Chain is a docker implementation of a rollkit chain.
type Chain struct {
	name                 string
	chainID              string
	binaryName           string
	aggregatorPassphrase string
	dockerClient         *client.Client
	dockerNetworkID      string
	log                  *zap.Logger
	nodes                []*Node
}

// GetNodes returns the nodes in the rollkit chain.
func (c *Chain) GetNodes() []*Node {
	return c.nodes
}

// Init initializes all nodes in the chain
func (c *Chain) Init(ctx context.Context, additionalInitArgs ...string) error {
	for _, node := range c.nodes {
		if err := node.Init(ctx, additionalInitArgs...); err != nil {
			return fmt.Errorf("failed to initialize node %d: %w", node.Index, err)
		}
	}
	return nil
}

// Start starts all nodes in the chain
func (c *Chain) Start(ctx context.Context, additionalStartArgs ...string) error {
	for _, node := range c.nodes {
		if err := node.Start(ctx, additionalStartArgs...); err != nil {
			return fmt.Errorf("failed to start node %d: %w", node.Index, err)
		}
	}
	return nil
}
