package docker

import (
	"fmt"
	"testing"

	"github.com/celestiaorg/tastora/framework/testutil/deploy"
)

// DeployMinimalStack is a test helper that spins up a simple, fully-wired stack using defaults:
// - celestia-app (1 validator)
// - celestia-node (1 bridge) pointed at celestia-app
// - reth (1 node)
// - evm-single (1 node) pointed at reth + DA
func DeployMinimalStack(t *testing.T, cfg *TestSetupConfig) (deploy.Stack, error) {
	t.Helper()

	deployed, err := deploy.Deploy(cfg.Ctx, cfg.ChainBuilder, cfg.DANetworkBuilder, cfg.RethBuilder, cfg.EVMSingleChainBuilder)
	if err != nil {
		return deploy.Stack{}, fmt.Errorf("deploy: %w", err)
	}

	return deploy.Stack{Celestia: deployed.Celestia, DA: deployed.DA, Reth: deployed.Reth, EvmSeq: deployed.EvmSeq}, nil
}

// DeployMultiChainStack is a test helper that spins up a fully-wired stack:
// - celestia-app (1 validator)
// - celestia-node (1 bridge) pointed at celestia-app
// - two evolve chain stacks containg (1 evm-single sequencer node and 1 reth execution client)
func DeployMultiChainStack(t *testing.T, cfg *TestSetupConfig) (*deploy.MultiChainStack, error) {
	t.Helper()

	deployed, err := deploy.DeployMultiChain(cfg.Ctx, cfg.ChainBuilder, cfg.DANetworkBuilder, cfg.RethBuilder, cfg.EVMSingleChainBuilder)
	if err != nil {
		return &deploy.MultiChainStack{}, fmt.Errorf("deploy: %w", err)
	}

	return deployed, nil
}
