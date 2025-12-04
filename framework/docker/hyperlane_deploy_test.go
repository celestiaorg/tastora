package docker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"github.com/celestiaorg/tastora/framework/types"
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

	// 1) Start a Celestia App chain (needed for DA network)
	chain, err := testCfg.ChainBuilder.Build(ctx)
	require.NoError(t, err)
	require.NoError(t, chain.Start(ctx))
	t.Cleanup(func() { _ = chain.Remove(context.Background()) })

	// 2) Start only the DA bridge node (minimal for this test) with custom network settings
	chainID := chain.GetChainID()
	chainNI, err := chain.GetNodes()[0].GetNetworkInfo(ctx)
	require.NoError(t, err)
	coreHost := chainNI.Internal.Hostname
	genesisHash, err := getGenesisHash(ctx, chain)
	require.NoError(t, err)

	danet, err := testCfg.DANetworkBuilder.WithChainID(chainID).Build(ctx)
	require.NoError(t, err)
	bridge := danet.GetBridgeNodes()[0]
	require.NoError(t, bridge.Start(ctx,
		da.WithChainID(chainID),
		da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", coreHost, "--rpc.addr", "0.0.0.0"),
		da.WithEnvironmentVariables(map[string]string{
			"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
			"P2P_NETWORK":     chainID,
		}),
	))
	t.Cleanup(func() { _ = bridge.Remove(context.Background()) })

	// 3) Start a single reth node
	rnode, err := testCfg.RethBuilder.Build(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rnode.Remove(context.Background()) })
	require.NoError(t, rnode.Start(ctx))

	// Wait until reth RPC becomes responsive
	require.Eventually(t, func() bool {
		ec, err := rnode.GetEthClient(ctx)
		if err != nil {
			return false
		}
		_, err = ec.BlockNumber(ctx)
		return err == nil
	}, 45*time.Second, 1*time.Second)

	// 4) Initialize the Hyperlane deployer with the reth node as a provider
	// Select a hyperlane image. Allow override via HYPERLANE_IMAGE (format repo:tag)
	hlImage := hyperlane.DefaultHyperlaneImage()
	if env := os.Getenv("HYPERLANE_IMAGE"); env != "" {
		// very small parser: expect repo:tag
		if i := len(env); i > 0 {
			// split on last ':' to allow registry prefixes
			last := -1
			for idx := range env {
				if env[idx] == ':' {
					last = idx
				}
			}
			if last > 0 && last < len(env)-1 {
				hlImage.Repository = env[:last]
				hlImage.Version = env[last+1:]
			}
		}
	}

	d, err := hyperlane.NewHyperlane(
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

	// Registry metadata for reth should exist
	_, err = d.ReadFile(ctx, filepath.Join("registry", "chains", "rethlocal", "metadata.yaml"))
	require.NoError(t, err)

	// 6) Execute the Hyperlane deploy steps
	require.NoError(t, d.Deploy(ctx))
}
