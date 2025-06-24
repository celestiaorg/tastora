package docker

import (
	"github.com/stretchr/testify/require"
	"testing"

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
	node1 := NewChainNode(ChainNodeParams{
		Logger:          logger,
		Validator:       true,
		DockerClient:    &dockerclient.Client{},
		DockerNetworkID: "test-network",
		TestName:        testName,
		Image:           DockerImage{},
		Index:           0,
		ChainID:         chainID,
		BinaryName:      "test-binary",
		CoinType:        "118",
		GasPrices:       "0.025utia",
		GasAdjustment:   1.0,
		Env:             []string{},
		AdditionalStartArgs: []string{},
		EncodingConfig:  &testutil.TestEncodingConfig{},
		ChainNodeConfig: nil,
		HomeDir:         "/test/home",
	})

	node2 := NewChainNode(ChainNodeParams{
		Logger:          logger,
		Validator:       true,
		DockerClient:    &dockerclient.Client{},
		DockerNetworkID: "test-network",
		TestName:        testName,
		Image:           DockerImage{},
		Index:           1,
		ChainID:         chainID,
		BinaryName:      "test-binary",
		CoinType:        "118",
		GasPrices:       "0.025utia",
		GasAdjustment:   1.0,
		Env:             []string{},
		AdditionalStartArgs: []string{},
		EncodingConfig:  &testutil.TestEncodingConfig{},
		ChainNodeConfig: nil,
		HomeDir:         "/test/home",
	})

	node3 := NewChainNode(ChainNodeParams{
		Logger:          logger,
		Validator:       false,
		DockerClient:    &dockerclient.Client{},
		DockerNetworkID: "test-network",
		TestName:        testName,
		Image:           DockerImage{},
		Index:           2,
		ChainID:         chainID,
		BinaryName:      "test-binary",
		CoinType:        "118",
		GasPrices:       "0.025utia",
		GasAdjustment:   1.0,
		Env:             []string{},
		AdditionalStartArgs: []string{},
		EncodingConfig:  &testutil.TestEncodingConfig{},
		ChainNodeConfig: nil,
		HomeDir:         "/test/home",
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
