package evstack

import (
	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
)

var _ types.EVStackChain = &Chain{}

// Chain is a docker implementation of an evstack chain.
type Chain struct {
	cfg   Config
	log   *zap.Logger
	nodes []*Node
}

// GetNodes returns the nodes in the evstack chain.
func (c *Chain) GetNodes() []types.EVStackNode {
	var nodes []types.EVStackNode
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}