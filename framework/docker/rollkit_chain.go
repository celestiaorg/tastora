package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/types"
	volumetypes "github.com/docker/docker/api/types/volume"
	"go.uber.org/zap"
)

var _ types.RollkitChain = &RollkitChain{}

func newRollkitChain(ctx context.Context, name string, cfg Config) (types.RollkitChain, error) {
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

type RollkitChain struct {
	cfg          Config
	log          *zap.Logger
	rollkitNodes []*RollkitNode
}

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

	v, err := cfg.DockerClient.VolumeCreate(ctx, volumetypes.CreateOptions{
		Labels: map[string]string{
			consts.CleanupLabel:   testName,
			consts.NodeOwnerLabel: rn.Name(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume for rollkit node: %w", err)
	}

	rn.VolumeName = v.Name

	if err := SetVolumeOwner(ctx, VolumeOwnerOptions{
		Log:        cfg.Logger,
		Client:     cfg.DockerClient,
		VolumeName: v.Name,
		ImageRef:   image.Ref(),
		TestName:   testName,
		UidGid:     image.UIDGID,
	}); err != nil {
		return nil, fmt.Errorf("set volume owner: %w", err)
	}

	return rn, nil
}
