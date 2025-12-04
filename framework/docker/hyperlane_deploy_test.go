package docker

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	commonpkg "github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

// TestHyperlaneDeployer_Bootstrap starts celestia-app, a DA bridge, and a reth node,
// then initializes and optionally runs the Hyperlane deployer steps.
func TestHyperlaneDeployer_Bootstrap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	testCfg := setupDockerTest(t)
	ctx, cancel := context.WithTimeout(testCfg.Ctx, 5*time.Minute)
	defer cancel()

	// Bring up full stack with defaults (celestia-app, DA bridge, reth, evm-single)
	stack, err := DeployMinimalStack(t, testCfg)
	require.NoError(t, err)
	chain := stack.celestia
	rnode := stack.reth

	// 4) Initialize the Hyperlane deployer with the reth node as a provider
	// Select a hyperlane image. Allow override via HYPERLANE_IMAGE (format repo:tag)
	hlImage := hyperlane.DefaultDeployerImage()

	d, err := hyperlane.NewDeployer(
		ctx,
		hyperlane.Config{
			Logger:          testCfg.Logger,
			DockerClient:    testCfg.DockerClient,
			DockerNetworkID: testCfg.NetworkID,
			HyperlaneImage:  hlImage,
		},
		testCfg.TestName,
		[]hyperlane.ChainConfigProvider{rnode, chain},
	)
	require.NoError(t, err)

	// 5) Validate that init wrote basic config files
	// Read relayer-config.json and ensure it contains the reth chain
	relayerBytes, err := d.ReadFile(ctx, "relayer-config.json")
	require.NoError(t, err)
	var relayerCfg hyperlane.RelayerConfig
	require.NoError(t, json.Unmarshal(relayerBytes, &relayerCfg))
	require.NotEmpty(t, relayerCfg.Chains["rethlocal"]) // name from reth provider
	// also expect the cosmos chain entry by its configured name
	require.NotEmpty(t, relayerCfg.Chains[chain.Config.Name])

	_, err = d.ReadFile(ctx, filepath.Join("registry", "chains", chain.Config.Name, "metadata.yaml"))
	require.NoError(t, err)

	// 6) Execute the Hyperlane deploy steps (must succeed)
	require.NoError(t, d.Deploy(ctx))

	// 7) Verify core contracts are deployed on reth by reading registry addresses
	onDiskSchema, err := d.GetOnDiskSchema(ctx)
	require.NoError(t, err)
	addrs := onDiskSchema.Registry.Chains["rethlocal"].Addresses

	// pick a few critical contracts to verify
	critical := map[string]string{
		"Mailbox":        addrs.Mailbox,
		"ProxyAdmin":     addrs.ProxyAdmin,
		"MerkleTreeHook": addrs.MerkleTreeHook,
	}
	for name, hex := range critical {
		require.NotEmpty(t, hex, "%s address should be present", name)
	}

	// Query code at those addresses via reth RPC to ensure contracts exist
	ec, err := rnode.GetEthClient(ctx)
	require.NoError(t, err)
	for name, hex := range critical {
		code, err := ec.CodeAt(ctx, commonpkg.HexToAddress(hex), nil)
		require.NoErrorf(t, err, "failed to fetch code for %s", name)
		require.Greaterf(t, len(code), 0, "%s should have non-empty code", name)
	}
}
