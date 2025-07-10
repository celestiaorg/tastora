package docker

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	dockerclient "github.com/moby/moby/client"
)

func TestChainNodeHostName(t *testing.T) {
	// Create a chain with multiple nodes
	testName := "test-hostname"
	chainID := "test-chain"
	logger := zaptest.NewLogger(t)

	// Create nodes with different indices
	chainParams1 := ChainNodeParams{
		Validator:       true,
		ChainID:         chainID,
		BinaryName:      "test-binary",
		CoinType:        "118",
		GasPrices:       "0.025utia",
		GasAdjustment:   1.0,
		Env:             []string{},
		AdditionalStartArgs: []string{},
		EncodingConfig:  &testutil.TestEncodingConfig{},
	}
	node1 := NewChainNode(logger, &dockerclient.Client{}, "test-network", testName, DockerImage{}, "/test/home", 0, chainParams1)

	chainParams2 := ChainNodeParams{
		Validator:       true,
		ChainID:         chainID,
		BinaryName:      "test-binary",
		CoinType:        "118",
		GasPrices:       "0.025utia",
		GasAdjustment:   1.0,
		Env:             []string{},
		AdditionalStartArgs: []string{},
		EncodingConfig:  &testutil.TestEncodingConfig{},
	}
	node2 := NewChainNode(logger, &dockerclient.Client{}, "test-network", testName, DockerImage{}, "/test/home", 1, chainParams2)

	chainParams3 := ChainNodeParams{
		Validator:       false,
		ChainID:         chainID,
		BinaryName:      "test-binary",
		CoinType:        "118",
		GasPrices:       "0.025utia",
		GasAdjustment:   1.0,
		Env:             []string{},
		AdditionalStartArgs: []string{},
		EncodingConfig:  &testutil.TestEncodingConfig{},
	}
	node3 := NewChainNode(logger, &dockerclient.Client{}, "test-network", testName, DockerImage{}, "/test/home", 2, chainParams3)

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

	node := NewDockerChainNode(logger, true, Config{
		ChainConfig: &ChainConfig{
			ChainID: chainID,
			Bin:     "celestia-appd",
		},
	}, testName, DockerImage{}, 0)

	// Test binCommand method
	cmd := node.binCommand("keys", "show", "validator")
	expected := []string{"celestia-appd", "keys", "show", "validator", "--home", node.homeDir}
	require.Equal(t, expected, cmd)

	// Test with different command
	cmd2 := node.binCommand("query", "bank", "balances", "addr123")
	expected2 := []string{"celestia-appd", "query", "bank", "balances", "addr123", "--home", node.homeDir}
	require.Equal(t, expected2, cmd2)
}
