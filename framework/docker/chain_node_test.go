package docker

import (
	"github.com/stretchr/testify/require"
	"testing"

	"go.uber.org/zap/zaptest"
)

func TestChainNodeHostName(t *testing.T) {
	// Create a chain with multiple nodes
	testName := "test-hostname"
	chainID := "test-chain"
	logger := zaptest.NewLogger(t)

	// Create nodes with different indices
	node1 := NewDockerChainNode(logger, true, Config{
		ChainConfig: &ChainConfig{
			ChainID: chainID,
		},
	}, testName, DockerImage{}, 0)

	node2 := NewDockerChainNode(logger, true, Config{
		ChainConfig: &ChainConfig{
			ChainID: chainID,
		},
	}, testName, DockerImage{}, 1)

	node3 := NewDockerChainNode(logger, false, Config{
		ChainConfig: &ChainConfig{
			ChainID: chainID,
		},
	}, testName, DockerImage{}, 2)

	// Get hostnames
	hostname1 := node1.HostName()
	hostname2 := node2.HostName()
	hostname3 := node3.HostName()

	// verify that all hostnames are different
	require.NotEqual(t, hostname1, hostname2)
	require.NotEqual(t, hostname1, hostname3)
	require.NotEqual(t, hostname2, hostname3)
}
