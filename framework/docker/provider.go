package docker

import (
	"context"
	"github.com/celestiaorg/tastora/framework/types"
	"testing"
)

var _ types.Provider = &Provider{}

type Provider struct {
	t        *testing.T
	testName string
	cfg      Config
}

// GetDataAvailabilityNetwork returns a new instance of the DataAvailabilityNetwork.
func (p *Provider) GetDataAvailabilityNetwork(ctx context.Context) (types.DataAvailabilityNetwork, error) {
	return newDataAvailabilityNetwork(ctx, p.testName, p.cfg)
}

// NewProvider creates and returns a new Provider instance using the provided configuration and test name.
func NewProvider(cfg Config, t *testing.T) *Provider {
	return NewProviderWithTestName(cfg, t, t.Name())
}

// NewProviderWithTestName creates and returns a new Provider instance with a custom test name for parallel execution.
func NewProviderWithTestName(cfg Config, t *testing.T, testName string) *Provider {
	return &Provider{
		cfg:      cfg,
		t:        t,
		testName: testName,
	}
}
