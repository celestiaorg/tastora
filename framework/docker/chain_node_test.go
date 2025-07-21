package docker

import (
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	dockerclient "github.com/moby/moby/client"
	"go.uber.org/zap/zaptest"
)

func TestChainNodeHostName(t *testing.T) {
	// Create a chain with multiple nodes
	testName := "test-hostname"
	chainID := "test-chain"
	logger := zaptest.NewLogger(t)

	// Create nodes with different indices
	chainParams1 := ChainNodeParams{
		Validator:           true,
		ChainID:             chainID,
		BinaryName:          "test-binary",
		CoinType:            "118",
		GasPrices:           "0.025utia",
		GasAdjustment:       1.0,
		Env:                 []string{},
		AdditionalStartArgs: []string{},
		EncodingConfig:      &testutil.TestEncodingConfig{},
		ChainStrategy:       cosmos.NewStrategy(),
	}
	node1 := NewChainNode(logger, &dockerclient.Client{}, "test-network", testName, container.Image{}, "/test/home", 0, chainParams1)

	chainParams2 := ChainNodeParams{
		Validator:           true,
		ChainID:             chainID,
		BinaryName:          "test-binary",
		CoinType:            "118",
		GasPrices:           "0.025utia",
		GasAdjustment:       1.0,
		Env:                 []string{},
		AdditionalStartArgs: []string{},
		EncodingConfig:      &testutil.TestEncodingConfig{},
		ChainStrategy:       cosmos.NewStrategy(),
	}
	node2 := NewChainNode(logger, &dockerclient.Client{}, "test-network", testName, container.Image{}, "/test/home", 1, chainParams2)

	chainParams3 := ChainNodeParams{
		Validator:           false,
		ChainID:             chainID,
		BinaryName:          "test-binary",
		CoinType:            "118",
		GasPrices:           "0.025utia",
		GasAdjustment:       1.0,
		Env:                 []string{},
		AdditionalStartArgs: []string{},
		EncodingConfig:      &testutil.TestEncodingConfig{},
		ChainStrategy:       cosmos.NewStrategy(),
	}
	node3 := NewChainNode(logger, &dockerclient.Client{}, "test-network", testName, container.Image{}, "/test/home", 2, chainParams3)

	// get hostnames
	hostname1 := node1.HostName()
	hostname2 := node2.HostName()
	hostname3 := node3.HostName()

	// verify that all hostnames are different
	require.NotEqual(t, hostname1, hostname2)
	require.NotEqual(t, hostname1, hostname3)
	require.NotEqual(t, hostname2, hostname3)
}
