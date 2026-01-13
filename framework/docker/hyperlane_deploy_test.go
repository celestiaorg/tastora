package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	warptypes "github.com/bcp-innovations/hyperlane-cosmos/x/warp/types"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	"github.com/celestiaorg/tastora/framework/testutil/evm"
	query "github.com/celestiaorg/tastora/framework/testutil/query"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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
	chain := stack.Celestia
	rnode := stack.Reth

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

	// Validate that init wrote basic config files
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

	broadcaster := cosmos.NewBroadcaster(chain)
	faucetWallet := chain.GetNode().GetFaucetWallet()

	config, err := d.DeployCosmosNoopISM(ctx, broadcaster, faucetWallet)
	require.NoError(t, err)
	require.NotNil(t, config)

	t.Logf("Deployed cosmos-native hyperlane: ISM=%s, Hooks=%s, Mailbox=%s, Token=%s",
		config.IsmID.String(), config.HooksID.String(), config.MailboxID.String(), config.TokenID.String())

	networkInfo, err := stack.Reth.GetNetworkInfo(ctx)
	require.NoError(t, err)
	rpcURL := fmt.Sprintf("http://%s", networkInfo.External.RPCAddress())

	networkInfo, err = stack.Celestia.GetNetworkInfo(ctx)
	require.NoError(t, err)

	hash, err := enrollRemoteRouter(ctx, d, networkInfo.External.GRPCAddress(), rpcURL)
	require.NoError(t, err)
	t.Logf("Enrolled remote router: %s", hash.Hex())

	// Enroll the remote EVM router on the Cosmos chain for the created token
	// find EVM chain name and domain
	var evmName string
	for name, cfg := range onDiskSchema.RelayerConfig.Chains {
		if cfg.Protocol == "ethereum" {
			evmName = name
			break
		}
	}
	require.NotEmpty(t, evmName)
	evmDomain := onDiskSchema.Registry.Chains[evmName].Metadata.DomainID
	evmRouter := onDiskSchema.Registry.Chains[evmName].Addresses.InterchainAccountRouter
	require.NotEmpty(t, evmRouter)

	// receiverContract must be a valid 32-byte HexAddress, pad EVM router address
	evmAddr20 := gethcommon.HexToAddress(evmRouter)
	paddedReceiver := evm.PadAddress(evmAddr20)
	err = d.EnrollRemoteRouterOnCosmos(ctx, broadcaster, faucetWallet, config.TokenID, evmDomain, paddedReceiver.String())
	require.NoError(t, err)

	// capture sender utia balance before
	ci, err := stack.Celestia.GetNetworkInfo(ctx)
	require.NoError(t, err)
	celestiaGRPC := ci.External.GRPCAddress()
	cconn, err := grpc.NewClient(celestiaGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = cconn.Close() }()

	// warp module escrow balance before
	warpModuleAddr := authtypes.NewModuleAddress(warptypes.ModuleName).String()
	beforeEscrow, err := query.Balance(ctx, cconn, warpModuleAddr, stack.Celestia.Config.Denom)
	require.NoError(t, err)
	require.Equal(t, sdkmath.NewInt(0), beforeEscrow, "escrow should be empty on start")

	sendAmount := sdkmath.NewInt(1000)

	txMsg := &warptypes.MsgRemoteTransfer{
		Sender:            faucetWallet.GetFormattedAddress(),
		TokenId:           config.TokenID,
		DestinationDomain: evmDomain,
		Recipient:         paddedReceiver,
		Amount:            sendAmount,
	}

	resp, err := broadcaster.BroadcastMessages(ctx, faucetWallet, txMsg)
	require.NoError(t, err)
	require.Equal(t, resp.Code, uint32(0), "remote transfer tx should succeed: code=%d, log=%s", resp.Code, resp.RawLog)
	require.NoError(t, wait.ForBlocks(ctx, 3, stack.Celestia))

	afterEscrow, err := query.Balance(ctx, cconn, warpModuleAddr, stack.Celestia.Config.Denom)
	require.NoError(t, err)
	require.Truef(t, afterEscrow.Equal(sendAmount), "escrow should increase by sendAmount %s on success", stack.Celestia.Config.Denom)
	err = d.EnrollRemoteRouterOnCosmos(ctx, broadcaster, faucetWallet, config.TokenID, evmDomain, evmRouter)
	require.NoError(t, err)
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
