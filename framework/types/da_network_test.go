package types

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBuildCelestiaCustomEnvVar(t *testing.T) {
	tests := []struct {
		name             string
		chainID          string
		genesisBlockHash string
		p2pAddress       string
		expected         string
	}{
		{
			name:             "OnlyChainID",
			chainID:          "testchain",
			genesisBlockHash: "",
			p2pAddress:       "",
			expected:         "testchain",
		},
		{
			name:             "ChainIDAndGenesisBlock",
			chainID:          "testchain",
			genesisBlockHash: "hash123",
			p2pAddress:       "",
			expected:         "testchain:hash123",
		},
		{
			name:             "ChainIDAndP2PAddress",
			chainID:          "testchain",
			genesisBlockHash: "",
			p2pAddress:       "p2p.node:12345",
			expected:         "testchain:p2p.node:12345",
		},
		{
			name:             "AllValues",
			chainID:          "testchain",
			genesisBlockHash: "hash123",
			p2pAddress:       "p2p.node:12345",
			expected:         "testchain:hash123:p2p.node:12345",
		},
		{
			name:             "EmptyValues",
			chainID:          "",
			genesisBlockHash: "",
			p2pAddress:       "",
			expected:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildCelestiaCustomEnvVar(tt.chainID, tt.genesisBlockHash, tt.p2pAddress)
			require.Equal(t, tt.expected, result, "expected %s, got %s", tt.expected, result)
		})
	}
}
