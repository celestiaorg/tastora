package docker

import (
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keyring"
)

func TestChainUsingCustomGenesisFile(t *testing.T) {
	tests := []struct {
		name       string
		validators ChainNodes
		expected   bool
	}{
		{
			name:       "empty validators array",
			validators: ChainNodes{},
			expected:   true,
		},
		{
			name:       "nil validators array",
			validators: nil,
			expected:   true,
		},
		{
			name: "validator with nil keyring",
			validators: ChainNodes{
				{
					ChainNodeParams: ChainNodeParams{GenesisKeyring: nil},
				},
			},
			expected: true,
		},
		{
			name: "validator with non-nil keyring",
			validators: ChainNodes{
				{
					ChainNodeParams: ChainNodeParams{GenesisKeyring: keyring.NewInMemory(nil)},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := &Chain{
				Validators: tt.validators,
			}

			result := chain.usingCustomGenesisFile()

			require.Equal(t, tt.expected, result)
		})
	}
}
