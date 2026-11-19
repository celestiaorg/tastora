package hyperlane

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const validatorConfigFile = "validator-config.json"

// ValidatorConfig configures a validator agent that signs checkpoints of one
// origin chain.
type ValidatorConfig struct {
	// OriginChainName is the origin chain as named in the relayer config.
	OriginChainName string
	// Key is the hex secp256k1 key the validator signs checkpoints with. Its
	// address goes into the multisig ISM; see ValidatorAddress.
	Key string
	// Interval is the polling interval in seconds; zero uses the agent default.
	Interval int
}

// CheckpointDir is the directory on the shared volume where the validator
// writes its checkpoints and announces them as a file:// location.
const CheckpointDir = hyperlaneHomeDir + "/checkpoints"

// ValidatorDBDir is the validator's database on the shared volume.
const ValidatorDBDir = hyperlaneHomeDir + "/validator-db"

type validatorConfigJSON struct {
	OriginChainName  string             `json:"originChainName"`
	Validator        SignerConfig       `json:"validator"`
	DB               string             `json:"db"`
	CheckpointSyncer checkpointSyncerJS `json:"checkpointSyncer"`
	Interval         int                `json:"interval,omitempty"`
}

type checkpointSyncerJS struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// WriteValidatorConfig writes the config a validator agent reads with the
// relayer config: the origin chain, the signing key, a database and a local
// checkpoint directory on the shared volume. The validator announces that
// directory on the origin chain with the origin's signer from the relayer
// config, so that account needs funds.
func (d *Deployer) WriteValidatorConfig(ctx context.Context, cfg ValidatorConfig) error {
	if _, ok := d.relayerCfg.Chains[cfg.OriginChainName]; !ok {
		return fmt.Errorf("origin chain %q is not in the relayer config", cfg.OriginChainName)
	}
	bz, err := validatorConfigBytes(cfg)
	if err != nil {
		return err
	}
	if err := d.WriteFile(ctx, validatorConfigFile, bz); err != nil {
		return fmt.Errorf("write validator config: %w", err)
	}
	return nil
}

func validatorConfigBytes(cfg ValidatorConfig) ([]byte, error) {
	bz, err := json.MarshalIndent(validatorConfigJSON{
		OriginChainName:  cfg.OriginChainName,
		Validator:        SignerConfig{Type: "hexKey", Key: cfg.Key},
		DB:               ValidatorDBDir,
		CheckpointSyncer: checkpointSyncerJS{Type: "localStorage", Path: CheckpointDir},
		Interval:         cfg.Interval,
	}, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("marshal validator config: %w", err)
	}
	return bz, nil
}

// ValidatorAddress returns the address of a hex secp256k1 validator key, as a
// multisig ISM lists it.
func ValidatorAddress(keyHex string) (common.Address, error) {
	key, err := crypto.HexToECDSA(strings.TrimPrefix(keyHex, "0x"))
	if err != nil {
		return common.Address{}, fmt.Errorf("parse validator key: %w", err)
	}
	return crypto.PubkeyToAddress(key.PublicKey), nil
}

// CheckpointFile is the file, relative to the shared volume, where a validator
// agent writes its signed checkpoint for a merkle tree index.
func CheckpointFile(index uint32) string {
	return path.Join("checkpoints", fmt.Sprintf("%d_with_id.json", index))
}

// CheckpointLatestIndexFile is the file, relative to the shared volume, that
// holds the highest index a validator agent has signed.
const CheckpointLatestIndexFile = "checkpoints/index.json"
