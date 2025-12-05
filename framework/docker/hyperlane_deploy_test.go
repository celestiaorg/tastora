package docker

import (
	"context"
	"encoding/json"
	"fmt"
	warptypes "github.com/bcp-innovations/hyperlane-cosmos/x/warp/types"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"github.com/stretchr/testify/require"
)

// TestHyperlaneDeployer_Bootstrap starts celestia-app, a DA bridge, and a reth node,
// then initializes and optionally runs the Hyperlane deployer steps.
func TestHyperlaneDeployer_Bootstrap(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	testCfg := setupDockerTest(t)
	ctx, cancel := context.WithTimeout(testCfg.Ctx, 10*time.Minute)
	defer cancel()

	// for this test only, use the celestia-app image built from the feature-zk-execution-ism branch.
	testCfg.ChainBuilder = testCfg.ChainBuilder.WithImage(container.NewImage("ghcr.io/celestiaorg/celestia-app-standalone", "feature-zk-execution-ism", "10001:10001"))

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

	relayerBytes, err := d.ReadFile(ctx, "relayer-config.json")
	require.NoError(t, err)
	var relayerCfg hyperlane.RelayerConfig
	require.NoError(t, json.Unmarshal(relayerBytes, &relayerCfg))
	require.NotEmpty(t, relayerCfg.Chains["rethlocal"]) // name from reth provider
	// also expect the cosmos chain entry by its configured name
	require.NotEmpty(t, relayerCfg.Chains[chain.Config.Name])

	require.NoError(t, d.Deploy(ctx))

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
		code, err := ec.CodeAt(ctx, gethcommon.HexToAddress(hex), nil)
		require.NoErrorf(t, err, "failed to fetch code for %s", name)
		require.Greaterf(t, len(code), 0, "%s should have non-empty code", name)
	}

	config, err := d.DeployCosmosNoopISM(ctx, cosmos.NewBroadcaster(chain), chain.GetNode().GetFaucetWallet())
	require.NoError(t, err)
	require.NotNil(t, config)

	t.Logf("Deployed cosmos-native hyperlane: ISM=%s, Hooks=%s, Mailbox=%s, Token=%s",
		config.IsmID.String(), config.HooksID.String(), config.MailboxID.String(), config.TokenID.String())

	networkInfo, err := stack.reth.GetNetworkInfo(ctx)
	require.NoError(t, err)
	rpcURL := fmt.Sprintf("http://%s", networkInfo.External.RPCAddress())

	networkInfo, err = stack.celestia.GetNetworkInfo(ctx)
	require.NoError(t, err)

	hash, err := enrollRemoteRouter(ctx, d, networkInfo.External.GRPCAddress(), rpcURL)
	require.NoError(t, err)
	t.Logf("Enrolled remote router: %s", hash.Hex())

}

func enrollRemoteRouter(ctx context.Context, d *hyperlane.Deployer, externalCelestiaRPCUrl, externalEVMRPCUrl string) (gethcommon.Hash, error) {
	schema, err := d.GetOnDiskSchema(ctx)
	if err != nil {
		return gethcommon.Hash{}, fmt.Errorf("failed to get on-disk schema: %w", err)
	}

	var evmName string
	for name, cfg := range schema.RelayerConfig.Chains {
		if cfg.Protocol == "ethereum" {
			evmName = name
			break
		}
	}

	if evmName == "" {
		return gethcommon.Hash{}, fmt.Errorf("no ethereum chain found in schema")
	}

	contractAddr := schema.Registry.Chains[evmName].Addresses.InterchainAccountRouter
	if contractAddr == "" {
		return gethcommon.Hash{}, fmt.Errorf("no InterchainAccountRouter address found in schema")
	}

	var cosmosName string
	for name, cfg := range schema.RelayerConfig.Chains {
		if cfg.Protocol == "cosmosnative" {
			cosmosName = name
			break
		}
	}

	if cosmosName == "" {
		return gethcommon.Hash{}, fmt.Errorf("no cosmos-native chain found in schema")
	}

	domain := schema.Registry.Chains[cosmosName].Metadata.DomainID

	warpTokens, err := queryWarpTokens(ctx, externalCelestiaRPCUrl)
	if err != nil {
		return gethcommon.Hash{}, fmt.Errorf("failed to query warp tokens: %w", err)
	}

	routerHex := warpTokens[0].IsmId.String()

	return d.EnrollRemoteRouter(ctx, contractAddr, domain, routerHex, evmName, externalEVMRPCUrl)
}

// queryWarpTokens retrieves a list of wrapped hyperlane tokens from the specified gRPC address.
func queryWarpTokens(ctx context.Context, grpcAddr string) ([]warptypes.WrappedHypToken, error) {
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial grpc %s: %w", grpcAddr, err)
	}
	defer func() {
		_ = conn.Close()
	}()

	q := warptypes.NewQueryClient(conn)
	resp, err := q.Tokens(ctx, &warptypes.QueryTokensRequest{})
	if err != nil {
		return nil, fmt.Errorf("warp tokens query failed: %w", err)
	}
	return resp.Tokens, nil
}
