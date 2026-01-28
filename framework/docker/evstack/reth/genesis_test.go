package reth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultEvolveGenesisJSON(t *testing.T) {
	t.Parallel()

	t.Run("default chain id", func(t *testing.T) {
		genesis := DefaultEvolveGenesisJSON()

		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(genesis), &parsed))

		config := parsed["config"].(map[string]any)
		chainID := config["chainId"].(float64)
		require.Equal(t, float64(1234), chainID)
	})

	t.Run("custom chain id", func(t *testing.T) {
		genesis := DefaultEvolveGenesisJSON(WithChainID(9999))

		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(genesis), &parsed))

		config := parsed["config"].(map[string]any)
		chainID := config["chainId"].(float64)
		require.Equal(t, float64(9999), chainID)
	})
}
