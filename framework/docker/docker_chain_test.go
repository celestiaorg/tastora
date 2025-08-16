package docker

import (
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
)

func TestChainUsingCustomGenesisFile(t *testing.T) {
	tests := []struct {
		name        string
		validators  ChainNodes
		genesisFile []byte
		expected    bool
	}{
		{
			name:        "empty validators array",
			validators:  ChainNodes{},
			genesisFile: nil,
			expected:    true,
		},
		{
			name:        "nil validators array",
			validators:  nil,
			genesisFile: nil,
			expected:    true,
		},
		{
			name: "validator with nil keyring",
			validators: ChainNodes{
				{
					ChainNodeParams: ChainNodeParams{GenesisKeyring: nil},
				},
			},
			genesisFile: nil,
			expected:    true,
		},
		{
			name: "validator with non-nil keyring and empty genesis file",
			validators: ChainNodes{
				{
					ChainNodeParams: ChainNodeParams{GenesisKeyring: keyring.NewInMemory(nil)},
				},
			},
			genesisFile: nil,
			expected:    false,
		},
		{
			name: "validator with non-nil keyring and custom genesis file",
			validators: ChainNodes{
				{
					ChainNodeParams: ChainNodeParams{GenesisKeyring: keyring.NewInMemory(nil)},
				},
			},
			genesisFile: []byte(`{"chain_id": "test"}`),
			expected:    true,
		},
		{
			name: "validator with nil keyring and custom genesis file",
			validators: ChainNodes{
				{
					ChainNodeParams: ChainNodeParams{GenesisKeyring: nil},
				},
			},
			genesisFile: []byte(`{"chain_id": "test"}`),
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := &Chain{
				Validators: tt.validators,
				cfg: Config{
					ChainConfig: &ChainConfig{
						GenesisFileBz: tt.genesisFile,
					},
				},
			}

			result := chain.usingCustomGenesisFile()

			require.Equal(t, tt.expected, result)
		})
	}
}
