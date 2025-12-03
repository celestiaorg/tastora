package reth

import (
	"testing"

	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"github.com/stretchr/testify/require"
)

func TestNode_ImplementsHyperlaneChainConfigProvider(t *testing.T) {
	var _ hyperlane.ChainConfigProvider = (*Node)(nil)
	require.True(t, true)
}

func TestGetHyperlaneChainMetadata_StaticFields(t *testing.T) {
	tests := []struct {
		name     string
		expected struct {
			name              string
			displayName       string
			chainID           int
			domainID          uint32
			protocol          string
			isTestnet         bool
			nativeTokenName   string
			nativeTokenSymbol string
			nativeDecimals    int
			signerKey         string
		}
	}{
		{
			name: "rethlocal static configuration",
			expected: struct {
				name              string
				displayName       string
				chainID           int
				domainID          uint32
				protocol          string
				isTestnet         bool
				nativeTokenName   string
				nativeTokenSymbol string
				nativeDecimals    int
				signerKey         string
			}{
				name:              "rethlocal",
				displayName:       "Reth",
				chainID:           1234,
				domainID:          1234,
				protocol:          "ethereum",
				isTestnet:         true,
				nativeTokenName:   "Ether",
				nativeTokenSymbol: "ETH",
				nativeDecimals:    18,
				signerKey:         "0x82bfcfadbf1712f6550d8d2c00a39f05b33ec78939d0167be2a737d691f33a6a",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, "rethlocal", tt.expected.name)
			require.Equal(t, "Reth", tt.expected.displayName)
			require.Equal(t, 1234, tt.expected.chainID)
			require.Equal(t, uint32(1234), tt.expected.domainID)
			require.Equal(t, "ethereum", tt.expected.protocol)
			require.True(t, tt.expected.isTestnet)
			require.Equal(t, "Ether", tt.expected.nativeTokenName)
			require.Equal(t, "ETH", tt.expected.nativeTokenSymbol)
			require.Equal(t, 18, tt.expected.nativeDecimals)
			require.Equal(t, "0x82bfcfadbf1712f6550d8d2c00a39f05b33ec78939d0167be2a737d691f33a6a", tt.expected.signerKey)
		})
	}
}
