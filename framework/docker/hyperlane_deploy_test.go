package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	ismtypes "github.com/bcp-innovations/hyperlane-cosmos/x/core/01_interchain_security/types"
	warptypes "github.com/bcp-innovations/hyperlane-cosmos/x/warp/types"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	"github.com/celestiaorg/tastora/framework/testutil/evm"
	query "github.com/celestiaorg/tastora/framework/testutil/query"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"github.com/stretchr/testify/require"
)

var (
	// NOTE: This a workaround as using the chain name "celestia" causes configuration overlay issues
	// with the Hyperlane agents container. This can be reverted when the following issue is addressed.
	// See https://github.com/hyperlane-xyz/hyperlane-monorepo/issues/7598.
	HypChainName = "celestiadev"
)

// TestHyperlaneDeployer_Bootstrap starts celestia-app, a DA bridge, and a reth node,
// then initializes and optionally runs the Hyperlane deployer steps.
func TestHyperlaneDeployer_Bootstrap(t *testing.T) {
	runCosmosToEVMTransfer(t, "")
}

// TestHyperlaneMultisigValidator is TestHyperlaneDeployer_Bootstrap with a
// messageIdMultisigIsm on the EVM mailbox and a validator agent that signs the
// Celestia checkpoints the relayer needs to deliver the transfer.
func TestHyperlaneMultisigValidator(t *testing.T) {
	runCosmosToEVMTransfer(t, "0x8b3a350cf5c34c9194ca85829a2df0ec3153be0318b5e2d3348e872092edffba")
}

// runCosmosToEVMTransfer deploys Hyperlane on celestia-app and reth and relays a
// warp transfer from Celestia to the EVM chain. With a validator key, the EVM
// mailbox uses a 1-of-1 messageIdMultisigIsm of that validator, which runs as
// an agent before the transfer.
func runCosmosToEVMTransfer(t *testing.T, validatorKey string) {
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

	hlImage := hyperlane.DefaultDeployerImage()

	deployerCfg := hyperlane.Config{
		Logger:          testCfg.Logger,
		DockerClient:    testCfg.DockerClient,
		DockerNetworkID: testCfg.NetworkID,
		HyperlaneImage:  hlImage,
	}
	var validatorAddr gethcommon.Address
	if validatorKey != "" {
		validatorAddr, err = hyperlane.ValidatorAddress(validatorKey)
		require.NoError(t, err)
		deployerCfg.DefaultIsm = &hyperlane.IsmCfg{
			Type:       "messageIdMultisigIsm",
			Validators: []hyperlane.QuotedHexAddress{hyperlane.QuotedHexAddress(validatorAddr.Hex())},
			Threshold:  1,
		}
	}

	d, err := hyperlane.NewDeployer(
		ctx,
		deployerCfg,
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
	require.NotEmpty(t, relayerCfg.Chains[HypChainName])

	require.NoError(t, d.Deploy(ctx))

	onDiskSchema, err := d.GetOnDiskSchema(ctx)
	require.NoError(t, err)

	addrs := onDiskSchema.Registry.Chains["rethlocal"].Addresses

	// pick a few critical contracts to verify
	critical := map[string]string{
		"Mailbox":        string(addrs.Mailbox),
		"ProxyAdmin":     string(addrs.ProxyAdmin),
		"MerkleTreeHook": string(addrs.MerkleTreeHook),
	}

	for name, hex := range critical {
		require.NotEmpty(t, hex, "%s address should be present", name)
	}

	addrsBz, _ := json.Marshal(addrs)
	t.Logf("Deployed ADDRS: %s", string(addrsBz))

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

	t.Logf("Deployed cosmos-native hyperlane: ISM=%s, Hooks=%s, Mailbox=%s, Token=%s, MerkleTreeHook=%s",
		config.IsmID.String(), config.HooksID.String(), config.MailboxID.String(), config.TokenID.String(), config.MerkleTreeHookID.String())

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

	// get the EVM warp token address from the registry (deployed by hyperlane warp deploy)
	evmAddr20, err := d.EVMWarpTokenAddress(ctx)
	require.NoError(t, err)
	t.Logf("EVM warp token address: %s", evmAddr20.String())

	// verify the warp token contract actually exists
	warpTokenCode, err := ec.CodeAt(ctx, evmAddr20, nil)
	require.NoError(t, err, "failed to fetch code for warp token")
	require.Greater(t, len(warpTokenCode), 0, "warp token should have non-empty code at %s", evmAddr20.Hex())
	t.Logf("Warp token contract has %d bytes of code", len(warpTokenCode))

	paddedReceiver := evm.PadAddress(evmAddr20)
	err = d.EnrollRemoteRouterOnCosmos(ctx, broadcaster, faucetWallet, config.TokenID, evmDomain, paddedReceiver)
	require.NoError(t, err)

	// Create and start a Hyperlane relayer agent using the on-disk relayer config
	agentCfg := hyperlane.Config{
		Logger:          testCfg.Logger,
		DockerClient:    testCfg.DockerClient,
		DockerNetworkID: testCfg.NetworkID,
		HyperlaneImage:  container.NewImage("damiannolan/hyperlane-agent", "test", "1000:1000"),
	}

	if validatorKey != "" {
		// The validator announces its checkpoint directory with the faucet
		// account, so wait for that before the test sends from it.
		require.NoError(t, d.WriteValidatorConfig(ctx, hyperlane.ValidatorConfig{OriginChainName: HypChainName, Key: validatorKey, Interval: 1}))
		validator, err := hyperlane.NewAgent(ctx, agentCfg, testCfg.TestName, hyperlane.AgentTypeValidator, d)
		require.NoError(t, err)
		require.NoError(t, validator.Start(ctx))
		waitForAnnouncement(ctx, t, stack.Celestia, config.MailboxID.String(), validatorAddr.Hex())
	}

	// capture sender utia balance before
	ci, err := stack.Celestia.GetNetworkInfo(ctx)
	require.NoError(t, err)
	celestiaGRPC := ci.External.GRPCAddress()
	cconn, err := grpc.NewClient(celestiaGRPC, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() {
		_ = cconn.Close()
	}()

	// warp module escrow balance before
	warpModuleAddr := authtypes.NewModuleAddress(warptypes.ModuleName).String()
	beforeEscrow, err := query.Balance(ctx, cconn, warpModuleAddr, stack.Celestia.Config.Denom)
	require.NoError(t, err)
	require.Equal(t, sdkmath.NewInt(0), beforeEscrow, "escrow should be empty on start")

	sendAmount := sdkmath.NewInt(1000)

	receiver := gethcommon.HexToAddress("0xaF9053bB6c4346381C77C2FeD279B17ABAfCDf4d")

	txMsg := &warptypes.MsgRemoteTransfer{
		Sender:            faucetWallet.GetFormattedAddress(),
		TokenId:           config.TokenID,
		DestinationDomain: evmDomain,
		Recipient:         evm.PadAddress(receiver),
		Amount:            sendAmount,
	}
	if validatorKey != "" {
		// The cosmos mailbox hooks are noop; the merkle tree hook puts the
		// message into the tree the validator signs.
		txMsg.CustomHookId = &config.MerkleTreeHookID

		// The warp token on the EVM chain must be secured by the multisig.
		requireRecipientMultisig(ctx, t, ec, onDiskSchema.Registry.Chains[evmName].Addresses.Mailbox, evmAddr20, validatorAddr)
	}

	resp, err := broadcaster.BroadcastMessages(ctx, faucetWallet, txMsg)
	require.NoError(t, err)
	require.Equal(t, resp.Code, uint32(0), "remote transfer tx should succeed: code=%d, log=%s", resp.Code, resp.RawLog)
	require.NoError(t, wait.ForBlocks(ctx, 3, stack.Celestia))

	afterEscrow, err := query.Balance(ctx, cconn, warpModuleAddr, stack.Celestia.Config.Denom)
	require.NoError(t, err)
	require.Truef(t, afterEscrow.Equal(sendAmount), "escrow should increase by sendAmount %s on success", stack.Celestia.Config.Denom)

	// Fetch the updated schema after cosmos deployment (which automatically updates the relayer config)
	onDiskSchema, err = d.GetOnDiskSchema(ctx)
	require.NoError(t, err)

	relayerCfg = *onDiskSchema.RelayerConfig

	agent, err := hyperlane.NewAgent(ctx, agentCfg, testCfg.TestName, hyperlane.AgentTypeRelayer, d)
	require.NoError(t, err)
	require.NoError(t, agent.Start(ctx))

	// wait for the relayer to process the message and mint tokens on EVM side
	// NOTE: Escrow should NOT drain for Cosmos→EVM transfers. Escrow only drains when tokens are sent back (EVM→Cosmos).
	require.Eventually(t, func() bool {
		// query ERC20 balanceOf for the recipient
		balance, err := evm.GetERC20Balance(ctx, ec, evmAddr20, receiver)
		if err != nil {
			t.Logf("error querying EVM warp token balance: %v", err)
			return false
		}
		t.Logf("EVM recipient warp token balance: %s", balance.String())
		return balance.Cmp(sendAmount.BigInt()) >= 0
	}, 2*time.Minute, 5*time.Second, "EVM recipient should receive minted warp tokens")

	// verify escrow remains full (tokens are locked, not drained)
	finalEscrow, err := query.Balance(ctx, cconn, warpModuleAddr, stack.Celestia.Config.Denom)
	require.NoError(t, err)
	require.Equal(t, sendAmount, finalEscrow, "escrow should remain at sendAmount for Cosmos→EVM transfer")

	if validatorKey != "" {
		// The delivery needed the validator's signed checkpoint of the first message.
		bz, err := d.ReadFile(ctx, hyperlane.CheckpointFile(0))
		require.NoError(t, err, "validator wrote no checkpoint for index 0")
		require.Contains(t, string(bz), "message_id")
	}
}

// requireRecipientMultisig checks that the ISM the EVM mailbox uses for the
// recipient is a 1-of-1 multisig of the validator.
func requireRecipientMultisig(ctx context.Context, t *testing.T, ec *ethclient.Client, mailbox hyperlane.QuotedHexAddress, recipient, validator gethcommon.Address) {
	t.Helper()
	parsed, err := abi.JSON(strings.NewReader(`[
		{"type":"function","name":"recipientIsm","stateMutability":"view","inputs":[{"name":"recipient","type":"address"}],"outputs":[{"type":"address"}]},
		{"type":"function","name":"validatorsAndThreshold","stateMutability":"view","inputs":[{"name":"message","type":"bytes"}],"outputs":[{"type":"address[]"},{"type":"uint8"}]}
	]`))
	require.NoError(t, err)
	call := func(to gethcommon.Address, method string, args ...any) []any {
		data, err := parsed.Pack(method, args...)
		require.NoError(t, err)
		out, err := ec.CallContract(ctx, ethereum.CallMsg{To: &to, Data: data}, nil)
		require.NoError(t, err, "call %s", method)
		res, err := parsed.Unpack(method, out)
		require.NoError(t, err, "unpack %s", method)
		return res
	}
	ism := call(gethcommon.HexToAddress(string(mailbox)), "recipientIsm", recipient)[0].(gethcommon.Address)
	res := call(ism, "validatorsAndThreshold", []byte{})
	require.Equal(t, []gethcommon.Address{validator}, res[0], "recipient ISM %s has other validators", ism.Hex())
	require.Equal(t, uint8(1), res[1], "recipient ISM %s has another threshold", ism.Hex())
}

// waitForAnnouncement waits until a validator has announced a storage location
// on the Celestia mailbox.
func waitForAnnouncement(ctx context.Context, t *testing.T, chain *cosmos.Chain, mailboxID, validator string) {
	t.Helper()
	conn, err := grpc.NewClient(chain.GetNode().GrpcConn.Target(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	client := ismtypes.NewQueryClient(conn)
	require.Eventually(t, func() bool {
		res, err := client.AnnouncedStorageLocations(ctx, &ismtypes.QueryAnnouncedStorageLocationsRequest{MailboxId: mailboxID, ValidatorAddress: validator})
		return err == nil && len(res.StorageLocations) > 0
	}, 2*time.Minute, 2*time.Second, "validator %s did not announce a storage location", validator)
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

	warpTokenAddr, err := d.EVMWarpTokenAddress(ctx)
	if err != nil {
		return gethcommon.Hash{}, fmt.Errorf("failed to get warp token address: %w", err)
	}
	contractAddr := warpTokenAddr.Hex()

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

	routerHex := warpTokens[0].Id

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
