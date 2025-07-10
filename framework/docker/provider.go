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

func (p *Provider) GetRollkitChain(ctx context.Context) (types.RollkitChain, error) {
	return newRollkitChain(ctx, p.t.Name(), p.cfg)
}

// GetDataAvailabilityNetwork returns a new instance of the DataAvailabilityNetwork.
func (p *Provider) GetDataAvailabilityNetwork(ctx context.Context) (types.DataAvailabilityNetwork, error) {
	return newDataAvailabilityNetwork(ctx, p.t.Name(), p.cfg)
}

// NewProvider creates and returns a new Provider instance using the provided configuration and test name.
func NewProvider(cfg Config, t *testing.T) *Provider {
	return &Provider{
		cfg: cfg,
		t:   t,
	}
}
