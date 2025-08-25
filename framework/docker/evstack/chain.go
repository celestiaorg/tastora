package evstack

import (
	"go.uber.org/zap"
)

// Chain is a docker implementation of an evstack chain.
type Chain struct {
	cfg   Config
	log   *zap.Logger
	nodes []*Node
}

// GetNodes returns the nodes in the evstack chain.
func (c *Chain) GetNodes() []*Node {
	return c.nodes
}