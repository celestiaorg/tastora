package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
)

var _ types.RollkitChain = &RollkitChain{}

// newRollkitChain initializes and returns a new RollkitChain instance using the provided context, test name, and configuration.
func newRollkitChain(ctx context.Context, name string, cfg Config) (types.RollkitChain, error) {
	if cfg.RollkitChainConfig == nil {
		return nil, fmt.Errorf("rollkit chain config is nil")
	}

	var nodes []*RollkitNode
	for i := range cfg.RollkitChainConfig.NumNodes {
		rollkitNode, err := newRollkitNode(ctx, cfg, name, cfg.RollkitChainConfig.Image, i)
		if err != nil {
			return nil, fmt.Errorf("failed to create rollkit node: %w", err)
		}
		nodes = append(nodes, rollkitNode)
	}

	return &RollkitChain{
		cfg:          cfg,
		log:          cfg.Logger,
		rollkitNodes: nodes,
	}, nil
}

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

// newRollkitNode constructs a new rollkit node with a docker volume.
func newRollkitNode(
	ctx context.Context,
	cfg Config,
	testName string,
	image DockerImage,
	index int,
) (*RollkitNode, error) {
	rn := NewRollkitNode(cfg, testName, image, index)

	if err := rn.createAndSetupVolume(ctx, rn.Name()); err != nil {
		return nil, err
	}

	return rn, nil
}
