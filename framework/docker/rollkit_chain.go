package docker

import (
	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
)

var _ types.RollkitChain = &RollkitChain{}


// RollkitChain is a docker implementation of a rollkit chain.
type RollkitChain struct {
	cfg          Config
	log          *zap.Logger
	rollkitNodes []*RollkitNode
}

// GetNodes returns the nodes in the rollkit chain.
func (r *RollkitChain) GetNodes() []types.RollkitNode {
	var nodes []types.RollkitNode
	for _, node := range r.rollkitNodes {
		nodes = append(nodes, node)
	}
	return nodes
}

