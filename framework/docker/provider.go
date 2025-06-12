package docker

import (
	"context"
	"fmt"
	"testing"

	"github.com/celestiaorg/tastora/framework/types"
)

var _ types.Provider = &Provider{}

type Provider struct {
	t   *testing.T
	cfg Config
}

// GetDataAvailabilityNetwork returns a new instance of the DataAvailabilityNetwork.
func (p *Provider) GetDataAvailabilityNetwork(ctx context.Context) (types.DataAvailabilityNetwork, error) {
	// Check if context contains a unique test name, otherwise use the provider's test name
	testName := p.t.Name()
	if ctxTestName := ctx.Value("testName"); ctxTestName != nil {
		if name, ok := ctxTestName.(string); ok && name != "" {
			testName = name
		}
	}

	// If DANodeConfig is provided, convert it to DataAvailabilityNetworkConfig
	cfg := p.cfg
	if cfg.DANodeConfig != nil && cfg.DataAvailabilityNetworkConfig == nil {
		// Convert DANodeConfig to DataAvailabilityNetworkConfig
		var image DockerImage
		if len(cfg.DANodeConfig.Images) > 0 {
			image = cfg.DANodeConfig.Images[0]
		}

		cfg.DataAvailabilityNetworkConfig = &DataAvailabilityNetworkConfig{
			BridgeNodeCount: 1, // Default to one bridge node
			LightNodeCount:  0,
			Image:           image,
		}
	}

	return newDataAvailabilityNetwork(ctx, testName, cfg)
}

// GetChain returns an initialized Chain instance based on the provided configuration and test name context.
// It creates necessary underlying resources and validates the configuration before instantiating the Chain.
func (p *Provider) GetChain(ctx context.Context) (types.Chain, error) {
	return newChain(ctx, p.t, p.cfg)
}

// GetDANode returns a single DA node of the specified type by creating a minimal DataAvailabilityNetwork.
// This is a convenience method that wraps GetDataAvailabilityNetwork for simple use cases.
func (p *Provider) GetDANode(ctx context.Context, nodeType types.DANodeType) (types.DANode, error) {
	daNetwork, err := p.GetDataAvailabilityNetwork(ctx)
	if err != nil {
		return nil, err
	}

	var nodes []types.DANode
	switch nodeType {
	case types.BridgeNode:
		nodes = daNetwork.GetBridgeNodes()
	case types.LightNode:
		nodes = daNetwork.GetLightNodes()
	default:
		return nil, fmt.Errorf("unsupported node type: %s", nodeType)
	}

	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes of type %s available", nodeType)
	}

	return nodes[0], nil
}

// NewProvider creates and returns a new Provider instance using the provided configuration and test name.
func NewProvider(cfg Config, t *testing.T) *Provider {
	return &Provider{
		cfg: cfg,
		t:   t,
	}
}
