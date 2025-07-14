package docker

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestChainNodeHostName(t *testing.T) {
	// Create a chain with multiple nodes
	testName := "test-hostname"
	chainID := "test-chain"
	logger := zaptest.NewLogger(t)

	// Create nodes with different indices
	node1 := NewChainNode(logger, nil, "", testName, DockerImage{}, "", 0, ChainNodeParams{
		ChainID: chainID,
	})

	node2 := NewChainNode(logger, nil, "", testName, DockerImage{}, "", 1, ChainNodeParams{
		ChainID: chainID,
	})

	node3 := NewChainNode(logger, nil, "", testName, DockerImage{}, "", 2, ChainNodeParams{
		Validator: false,
		ChainID:   chainID,
	})

	// get hostnames
	hostname1 := node1.HostName()
	hostname2 := node2.HostName()
	hostname3 := node3.HostName()

	// verify that all hostnames are different
	require.NotEqual(t, hostname1, hostname2)
	require.NotEqual(t, hostname1, hostname3)
	require.NotEqual(t, hostname2, hostname3)
}

func TestChainNodeBinCommand_Construction(t *testing.T) {
	// This tests the binCommand method
	testName := "test-exec"
	chainID := "test-chain"
	logger := zaptest.NewLogger(t)

	node := NewChainNode(logger, nil, "", testName, DockerImage{}, "", 0, ChainNodeParams{
		ChainID:    chainID,
		BinaryName: "celestia-appd",
	})

	// Test binCommand method
	cmd := node.binCommand("keys", "show", "validator")
	expected := []string{"celestia-appd", "keys", "show", "validator", "--home", node.homeDir}
	require.Equal(t, expected, cmd)

	// Test with different command
	cmd2 := node.binCommand("query", "bank", "balances", "addr123")
	expected2 := []string{"celestia-appd", "query", "bank", "balances", "addr123", "--home", node.homeDir}
	require.Equal(t, expected2, cmd2)
}
