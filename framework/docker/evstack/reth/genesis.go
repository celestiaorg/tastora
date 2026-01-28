package reth

import (
	"fmt"

	"github.com/celestiaorg/tastora/framework/testutil/maps"
)

// GenesisOpt modifies the bytes of the genesis before it is returned to allow for arbitrary modifications.
type GenesisOpt func([]byte) ([]byte, error)

// WithChainID updates the chainId of the evolve genesis.
func WithChainID(chainID int) GenesisOpt {
	return func(genBz []byte) ([]byte, error) {
		return maps.SetField(genBz, "config.chainId", chainID)
	}
}

// DefaultEvolveGenesisJSON returns a stable EVM genesis JSON used to align
// ev-node (sequencer) and the execution client (reth) during tests.
//
// using a hard coded genesis to align with the e2e tests in the ev-node repo. This can be modified with WithChainID.
func DefaultEvolveGenesisJSON(opts ...GenesisOpt) string {

	// NOTE: the 0xaF9053bB6c4346381C77C2FeD279B17ABAfCDf4d address here is associated with the
	// default hyperlane private key that is used in tests.
	genesis := `{
  "config": {
    "chainId": 1234,
    "homesteadBlock": 0,
    "eip150Block": 0,
    "eip155Block": 0,
    "eip158Block": 0,
    "byzantiumBlock": 0,
    "constantinopleBlock": 0,
    "petersburgBlock": 0,
    "istanbulBlock": 0,
    "berlinBlock": 0,
    "londonBlock": 0,
    "mergeNetsplitBlock": 0,
    "terminalTotalDifficulty": 0,
    "terminalTotalDifficultyPassed": true,
    "shanghaiTime": 0,
    "cancunTime": 0,
    "pragueTime": 0
  },
  "nonce": "0x0",
  "timestamp": "0x0",
  "extraData": "0x",
  "gasLimit": "0x1c9c38000",
  "difficulty": "0x0",
  "mixHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
  "coinbase": "0x0000000000000000000000000000000000000000",
  "alloc": {
    "0xd143C405751162d0F96bEE2eB5eb9C61882a736E": {
      "balance": "0x4a47e3c12448f4ad000000"
    },
    "0x944fDcD1c868E3cC566C78023CcB38A32cDA836E": {
      "balance": "0x4a47e3c12448f4ad000000"
    },
    "0x4567BF59F76c18cEa2131BDA24A7b70744308f54": {
      "balance": "0x4a47e3c12448f4ad000000"
    },
    "0xaF9053bB6c4346381C77C2FeD279B17ABAfCDf4d": {
      "balance": "0x4a47e3c12448f4ad000000"
    }
  },
  "number": "0x0",
  "gasUsed": "0x0",
  "parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
  "baseFeePerGas": "0x3b9aca00"
}`

	genBz := []byte(genesis)
	for _, opt := range opts {
		var err error
		genBz, err = opt(genBz)
		if err != nil {
			panic(fmt.Sprint("failed to apply option: %w", err))
		}
	}

	return string(genBz)
}
