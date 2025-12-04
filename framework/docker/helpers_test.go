package docker

import (
	"context"
	"fmt"
	"testing"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	reth "github.com/celestiaorg/tastora/framework/docker/evstack/reth"
	sdkacc "github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govmodule "github.com/cosmos/cosmos-sdk/x/gov"
	"github.com/cosmos/ibc-go/v8/modules/apps/transfer"
)

type Stack struct {
	celestia *cosmos.Chain
	da       *da.Network
	reth     *reth.Node
	evm      *evmsingle.Chain
}

// DeployMinimalStack is a test helper that spins up a simple, fully-wired stack using defaults:
// - celestia-app (1 validator)
// - celestia-node (1 bridge) pointed at celestia-app
// - reth (1 node)
// - evm-single (1 node) pointed at reth + DA
func DeployMinimalStack(t *testing.T) (Stack, error) {
	t.Helper()

	ctx := context.Background()
	dockerClient, networkID := Setup(t)

	// Bech32 account prefix for celestia
	sdkConf := sdk.GetConfig()
	sdkConf.SetBech32PrefixForAccount("celestia", "celestiapub")

	// 1) celestia-app chain
	enc := testutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{}, transfer.AppModuleBasic{}, govmodule.AppModuleBasic{})
	appImage := container.Image{Repository: "ghcr.io/celestiaorg/celestia-app", Version: "v5.0.10", UIDGID: "10001:10001"}
	chainBuilder := cosmos.NewChainBuilderWithTestName(t, t.Name()).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID).
		WithImage(appImage).
		WithEncodingConfig(&enc).
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--grpc.enable",
			"--grpc.address", "0.0.0.0:9090",
			"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
			"--timeout-commit", "1s",
			"--minimum-gas-prices", "0utia",
		).
		WithNode(cosmos.NewChainNodeConfigBuilder().Build())

	chain, err := chainBuilder.Build(ctx)
	if err != nil {
		return Stack{}, fmt.Errorf("build celestia-app: %w", err)
	}
	if err := chain.Start(ctx); err != nil {
		return Stack{}, fmt.Errorf("start celestia-app: %w", err)
	}

	chainID := chain.GetChainID()
	cni, err := chain.GetNodes()[0].GetNetworkInfo(ctx)
	if err != nil {
		return Stack{}, fmt.Errorf("chain network info: %w", err)
	}
	coreHost := cni.Internal.Hostname
	coreGenesisHash, err := getGenesisHash(ctx, chain)
	if err != nil {
		return Stack{}, fmt.Errorf("get genesis hash: %w", err)
	}

	// 2) DA bridge
	daImage := container.Image{Repository: "ghcr.io/celestiaorg/celestia-node", Version: "v0.26.4", UIDGID: "10001:10001"}
	bridgeCfg := da.NewNodeBuilder().WithNodeType(types.BridgeNode).Build()
	danet, err := da.NewNetworkBuilderWithTestName(t, t.Name()).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID).
		WithImage(daImage).
		WithNodes(bridgeCfg).
		WithChainID(chainID).
		Build(ctx)
	if err != nil {
		return Stack{}, fmt.Errorf("build da network: %w", err)
	}
	bridge := danet.GetBridgeNodes()[0]
	if err := bridge.Start(ctx,
		da.WithChainID(chainID),
		da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", coreHost, "--rpc.addr", "0.0.0.0"),
		da.WithEnvironmentVariables(map[string]string{
			"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, coreGenesisHash, ""),
			"P2P_NETWORK":     chainID,
		}),
	); err != nil {
		return Stack{}, fmt.Errorf("start da bridge: %w", err)
	}

	// fund DA wallet
	daWallet, err := bridge.GetWallet()
	if err != nil {
		return Stack{}, fmt.Errorf("get da wallet: %w", err)
	}
	from, err := sdkacc.AddressFromWallet(chain.GetFaucetWallet())
	if err != nil {
		return Stack{}, fmt.Errorf("faucet address: %w", err)
	}
	to, err := sdk.AccAddressFromBech32(daWallet.GetFormattedAddress())
	if err != nil {
		return Stack{}, fmt.Errorf("da address: %w", err)
	}
	send := banktypes.NewMsgSend(from, to, sdk.NewCoins(sdk.NewCoin("utia", sdkmath.NewInt(100_000_000_00))))
	if _, err := chain.BroadcastMessages(ctx, chain.GetFaucetWallet(), send); err != nil {
		return Stack{}, fmt.Errorf("fund da wallet: %w", err)
	}

	// internal DA RPC for evm-single
	bni, err := bridge.GetNetworkInfo(ctx)
	if err != nil {
		return Stack{}, fmt.Errorf("bridge network info: %w", err)
	}
	daAddress := fmt.Sprintf("http://%s:%s", bni.Internal.IP, bni.Internal.Ports.RPC)

	// 3) reth
	rnode, err := reth.NewNodeBuilderWithTestName(t, t.Name()).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID).
		WithGenesis([]byte(reth.DefaultEvolveGenesisJSON())).
		Build(ctx)
	if err != nil {
		return Stack{}, fmt.Errorf("build reth: %w", err)
	}
	if err := rnode.Start(ctx); err != nil {
		return Stack{}, fmt.Errorf("start reth: %w", err)
	}

	rni, err := rnode.GetNetworkInfo(ctx)
	if err != nil {
		return Stack{}, fmt.Errorf("reth network info: %w", err)
	}
	evmEthURL := fmt.Sprintf("http://%s:%s", rni.Internal.Hostname, rni.Internal.Ports.RPC)
	evmEngineURL := fmt.Sprintf("http://%s:%s", rni.Internal.Hostname, rni.Internal.Ports.Engine)
	rGenesisHash, err := rnode.GenesisHash(ctx)
	if err != nil {
		return Stack{}, fmt.Errorf("reth genesis hash: %w", err)
	}

	// 4) evm-single
	evNodeCfg := evmsingle.NewNodeConfigBuilder().
		WithEVMEngineURL(evmEngineURL).
		WithEVMETHURL(evmEthURL).
		WithEVMJWTSecret(rnode.JWTSecretHex()).
		WithEVMSignerPassphrase("secret").
		WithEVMBlockTime("1s").
		WithEVMGenesisHash(rGenesisHash).
		WithDAAddress(daAddress).
		Build()
	evChain, err := evmsingle.NewChainBuilderWithTestName(t, t.Name()).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID).
		WithNodes(evNodeCfg).
		Build(ctx)
	if err != nil {
		return Stack{}, fmt.Errorf("build evm-single: %w", err)
	}
	if err := evChain.Start(ctx); err != nil {
		return Stack{}, fmt.Errorf("start evm-single: %w", err)
	}

	return Stack{
		celestia: chain,
		da:       danet,
		reth:     rnode,
		evm:      evChain,
	}, nil
}
