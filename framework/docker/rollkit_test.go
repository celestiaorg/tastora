package docker

import (
	"context"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"testing"
)

func rollkitProvider(t *testing.T) types.Provider {
	client, network := DockerSetup(t)

	cfg := Config{
		Logger:          zaptest.NewLogger(t),
		DockerClient:    client,
		DockerNetworkID: network,
		RollkitChainConfig: &RollkitChainConfig{
			ChainID: "test",
			Bin:     "testapp",
		},
	}
	return NewProvider(cfg, t)
}

func TestRollkit(t *testing.T) {
	provider := rollkitProvider(t)

	rollkit, err := provider.GetRollkitChain(context.Background())
	require.NoError(t, err)

	_ = rollkit
}
