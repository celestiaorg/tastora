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
			ChainID:              "test",
			Bin:                  "testapp",
			AggregatorPassphrase: "12345678",
			NumNodes:             1,
			Image: DockerImage{
				Repository: "rollkit",
				Version:    "latest",
				UIDGID:     "2000",
			},
		},
	}
	return NewProvider(cfg, t)
}

func TestRollkit(t *testing.T) {
	provider := rollkitProvider(t)

	rollkit, err := provider.GetRollkitChain(context.Background())
	require.NoError(t, err)

	nodes := rollkit.GetNodes()
	require.Len(t, nodes, 1)
	aggregatorNode := nodes[0]

	err = aggregatorNode.Init(context.Background())
	require.NoError(t, err)

	err = aggregatorNode.Start(context.Background())
	require.NoError(t, err)

}
