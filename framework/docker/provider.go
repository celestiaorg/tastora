package docker

import (
	"context"
	"github.com/celestiaorg/tastora/framework/types"
	"testing"
)

var _ types.Provider = &Provider{}

type Provider struct {
	t   *testing.T
	cfg Config
}

// GetDataAvailabilityNetwork returns a new instance of the DataAvailabilityNetwork.
func (p *Provider) GetDataAvailabilityNetwork(ctx context.Context) (types.DataAvailabilityNetwork, error) {
	return NewDataAvailabilityNetwork(), nil
}

// GetDANode retrieves a node of the specified type.
func (p *Provider) GetDANode(ctx context.Context, nodeType types.DANodeType) (types.DANode, error) {
	return newDANode(ctx, p.t.Name(), p.cfg, nodeType)
}

// GetChain returns an initialized Chain instance based on the provided configuration and test name context.
// It creates necessary underlying resources and validates the configuration before instantiating the Chain.
func (p *Provider) GetChain(ctx context.Context) (types.Chain, error) {
	return newChain(ctx, p.t, p.cfg)
}

// NewProvider creates and returns a new Provider instance using the provided configuration and test name.
func NewProvider(cfg Config, t *testing.T) *Provider {
	return &Provider{
		cfg: cfg,
		t:   t,
	}
}
