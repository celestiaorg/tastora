package hyperlane

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestValidatorConfigBytes(t *testing.T) {
	bz, err := validatorConfigBytes(ValidatorConfig{OriginChainName: "celestiadev", Key: "0x01", Interval: 1})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(bz, &got))
	require.Equal(t, map[string]any{
		"originChainName":  "celestiadev",
		"validator":        map[string]any{"type": "hexKey", "key": "0x01"},
		"db":               "/workspace/validator-db",
		"checkpointSyncer": map[string]any{"type": "localStorage", "path": "/workspace/checkpoints"},
		"interval":         float64(1),
	}, got)
}

func TestValidatorAddress(t *testing.T) {
	// The address of private key 1 is well known.
	addr, err := ValidatorAddress("0x0000000000000000000000000000000000000000000000000000000000000001")
	require.NoError(t, err)
	require.Equal(t, "0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf", addr.Hex())

	_, err = ValidatorAddress("not a key")
	require.Error(t, err)
}

func TestCoreConfigMultisigIsm(t *testing.T) {
	bz, err := yaml.Marshal(CoreConfig{DefaultIsm: IsmCfg{
		Type:       "messageIdMultisigIsm",
		Validators: []QuotedHexAddress{"0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf"},
		Threshold:  1,
	}})
	require.NoError(t, err)
	require.Contains(t, string(bz), "defaultIsm:\n    type: messageIdMultisigIsm\n    validators:\n        - \"0x7E5F4552091A69125d5DfCb7b8C2659029395Bdf\"\n    threshold: 1\n")

	bz, err = yaml.Marshal(CoreConfig{DefaultIsm: IsmCfg{Type: "testIsm"}})
	require.NoError(t, err)
	require.Contains(t, string(bz), "defaultIsm:\n    type: testIsm\n")
}

func TestEVMWarpTokenFromArtifact(t *testing.T) {
	chains := map[string]RelayerChainConfig{
		"rethlocal":   {Protocol: "ethereum"},
		"celestiadev": {Protocol: "cosmosnative"},
	}
	artifact := []byte(`tokens:
  - chainName: celestiadev
    addressOrDenom: "0x726f757465725f61707000000000000000000000000000010000000000000000"
    standard: CosmosNativeHypCollateral
  - chainName: rethlocal
    addressOrDenom: "0x345a583028762De4d733852c9D4f419077093A48"
    standard: EvmHypSynthetic
`)
	addr, ok, err := evmWarpTokenFromArtifact(artifact, chains)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "0x345a583028762De4d733852c9D4f419077093A48", addr.Hex())

	_, ok, err = evmWarpTokenFromArtifact(nil, chains)
	require.NoError(t, err)
	require.False(t, ok)
}
