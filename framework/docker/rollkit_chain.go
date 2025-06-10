package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/types"
	//"github.com/cosmos/cosmos-sdk/codec"
	volumetypes "github.com/docker/docker/api/types/volume"
	"go.uber.org/zap"
	"sync"
	"testing"
)

var _ types.RollkitChain = &RollkitChain{}

func newRollkitChain(ctx context.Context, name string, cfg Config) (types.RollkitChain, error) {
	return nil, nil
}

type RollkitChain struct {
	t   *testing.T
	cfg Config
	//cdc          *codec.ProtoCodec
	log          *zap.Logger
	findTxMu     sync.Mutex
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
func (c *RollkitChain) newRollkitNode(
	ctx context.Context,
	testName string,
	image DockerImage,
	index int,
) (*RollkitNode, error) {
	rn := NewRollkitNode(c.log, c.cfg, testName, image, index)

	v, err := c.cfg.DockerClient.VolumeCreate(ctx, volumetypes.CreateOptions{
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
		Log:        c.log,
		Client:     c.cfg.DockerClient,
		VolumeName: v.Name,
		ImageRef:   image.Ref(),
		TestName:   testName,
		UidGid:     image.UIDGID,
	}); err != nil {
		return nil, fmt.Errorf("set volume owner: %w", err)
	}

	return rn, nil
}
