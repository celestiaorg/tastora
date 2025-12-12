package deploy

import (
	"context"
	"fmt"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	reth "github.com/celestiaorg/tastora/framework/docker/evstack/reth"
	sdkacc "github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	govmodule "github.com/cosmos/cosmos-sdk/x/gov"
	transfer "github.com/cosmos/ibc-go/v8/modules/apps/transfer"
	"go.uber.org/zap/zaptest"
)

type Stack struct {
	Celestia *cosmos.Chain
	DA       *da.Network
	Reth     *reth.Node
	EVM      *evmsingle.Chain
}

// WithDefaults deploys a celestia chain, a da network, a reth node and evm single in with default values
// for when it is not important and that is not the focus of the test.
func WithDefaults(t *testing.T, dockerClient types.TastoraDockerClient, networkID, testName string) (*Stack, error) {
	t.Helper()

	ctx := context.Background()
	logger := zaptest.NewLogger(t)
	encConfig := testutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{}, transfer.AppModuleBasic{}, govmodule.AppModuleBasic{})

	chainBuilder := cosmos.NewChainBuilderWithTestName(t, testName).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID).
		WithImage(container.Image{
			Repository: "ghcr.io/celestiaorg/celestia-app",
			Version:    "v5.0.10",
			UIDGID:     "10001:10001",
		}).
		WithEncodingConfig(&encConfig).
		WithLogger(logger).
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--grpc.enable",
			"--grpc.address", "0.0.0.0:9090",
			"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
			"--timeout-commit", "1s",
			"--minimum-gas-prices", "0utia",
		).
		WithNode(cosmos.NewChainNodeConfigBuilder().Build())

	daBuilder := da.NewNetworkBuilderWithTestName(t, testName).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID).
		WithImage(container.Image{
			Repository: "ghcr.io/celestiaorg/celestia-node",
			Version:    "v0.26.4",
			UIDGID:     "10001:10001",
		}).
		WithNodes(da.NewNodeBuilder().WithNodeType(types.BridgeNode).Build())

	rethBuilder := reth.NewNodeBuilderWithTestName(t, testName).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID).
		WithGenesis([]byte(reth.DefaultEvolveGenesisJSON()))

	evmBuilder := evmsingle.NewChainBuilderWithTestName(t, testName).
		WithDockerClient(dockerClient).
		WithDockerNetworkID(networkID)

	return Deploy(ctx, chainBuilder, daBuilder, rethBuilder, evmBuilder)
}

// Deploy deploys an ev stack with the provided set of builders. Every component is wired up together and started.
func Deploy(ctx context.Context, chainBuilder *cosmos.ChainBuilder, daBuilder *da.NetworkBuilder, rethBuilder *reth.NodeBuilder, evmBuilder *evmsingle.ChainBuilder) (*Stack, error) {
	celestia, daNetwork, err := CelestiaWithDA(ctx, chainBuilder, daBuilder)
	if err != nil {
		return nil, err
	}

	rethNode, evmSingle, err := RethWithEVMSingle(ctx, rethBuilder, evmBuilder, daNetwork)
	if err != nil {
		return nil, err
	}

	return &Stack{
		Celestia: celestia,
		DA:       daNetwork,
		Reth:     rethNode,
		EVM:      evmSingle,
	}, nil
}

// CelestiaWithDA deploys and  starts a celestia chain and bridge node with the provided builders.
func CelestiaWithDA(ctx context.Context, chainBuilder *cosmos.ChainBuilder, daBuilder *da.NetworkBuilder) (*cosmos.Chain, *da.Network, error) {
	chain, err := chainBuilder.Build(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("build celestia-app: %w", err)
	}
	if err := chain.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("start celestia-app: %w", err)
	}

	chainID := chain.GetChainID()
	cni, err := chain.GetNodes()[0].GetNetworkInfo(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("chain network info: %w", err)
	}
	coreHost := cni.Internal.Hostname
	coreGenesisHash, err := getGenesisHash(ctx, chain)
	if err != nil {
		return nil, nil, fmt.Errorf("get genesis hash: %w", err)
	}

	daNetwork, err := daBuilder.WithChainID(chainID).Build(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("build da network: %w", err)
	}

	bridge := daNetwork.GetBridgeNodes()[0]
	if err := bridge.Start(ctx,
		da.WithChainID(chainID),
		da.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", coreHost, "--rpc.addr", "0.0.0.0"),
		da.WithEnvironmentVariables(map[string]string{
			"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, coreGenesisHash, ""),
			"P2P_NETWORK":     chainID,
		}),
	); err != nil {
		return nil, nil, fmt.Errorf("start da bridge: %w", err)
	}

	daWallet, err := bridge.GetWallet()
	if err != nil {
		return nil, nil, fmt.Errorf("get da wallet: %w", err)
	}
	from, err := sdkacc.AddressFromWallet(chain.GetFaucetWallet())
	if err != nil {
		return nil, nil, fmt.Errorf("faucet address: %w", err)
	}
	to, err := sdk.AccAddressFromBech32(daWallet.GetFormattedAddress())
	if err != nil {
		return nil, nil, fmt.Errorf("da address: %w", err)
	}
	send := banktypes.NewMsgSend(from, to, sdk.NewCoins(sdk.NewCoin("utia", sdkmath.NewInt(100_000_000_00))))
	if _, err := chain.BroadcastMessages(ctx, chain.GetFaucetWallet(), send); err != nil {
		return nil, nil, fmt.Errorf("fund da wallet: %w", err)
	}

	return chain, daNetwork, nil
}

// RethWithEVMSingle deploys a reth node and evmsingle wired up to use the provided da network.
func RethWithEVMSingle(ctx context.Context, rethBuilder *reth.NodeBuilder, evmBuilder *evmsingle.ChainBuilder, danet *da.Network) (*reth.Node, *evmsingle.Chain, error) {
	bridge := danet.GetBridgeNodes()[0]
	bridgeNodeNetworkInfo, err := bridge.GetNetworkInfo(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("bridge network info: %w", err)
	}
	daAddress := fmt.Sprintf("http://%s:%s", bridgeNodeNetworkInfo.Internal.IP, bridgeNodeNetworkInfo.Internal.Ports.RPC)

	rnode, err := rethBuilder.Build(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("build reth: %w", err)
	}
	if err := rnode.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("start reth: %w", err)
	}

	rni, err := rnode.GetNetworkInfo(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("reth network info: %w", err)
	}
	evmEthURL := fmt.Sprintf("http://%s:%s", rni.Internal.Hostname, rni.Internal.Ports.RPC)
	evmEngineURL := fmt.Sprintf("http://%s:%s", rni.Internal.Hostname, rni.Internal.Ports.Engine)
	rGenesisHash, err := rnode.GenesisHash(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("reth genesis hash: %w", err)
	}

	evNodeCfg := evmsingle.NewNodeConfigBuilder().
		WithEVMEngineURL(evmEngineURL).
		WithEVMETHURL(evmEthURL).
		WithEVMJWTSecret(rnode.JWTSecretHex()).
		WithEVMSignerPassphrase("secret").
		WithEVMBlockTime("1s").
		WithEVMGenesisHash(rGenesisHash).
		WithDAAddress(daAddress).
		Build()

	evmSingle, err := evmBuilder.WithNodes(evNodeCfg).Build(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("build evm-single: %w", err)
	}
	if err := evmSingle.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("start evm-single: %w", err)
	}

	return rnode, evmSingle, nil
}

func getGenesisHash(ctx context.Context, chain *cosmos.Chain) (string, error) {
	node := chain.GetNodes()[0]
	c, err := node.GetRPCClient()
	if err != nil {
		return "", fmt.Errorf("failed to get node client: %v", err)
	}

	first := int64(1)
	block, err := c.Block(ctx, &first)
	if err != nil {
		return "", fmt.Errorf("failed to get block: %v", err)
	}

	genesisHash := block.Block.Header.Hash().String()
	if genesisHash == "" {
		return "", fmt.Errorf("genesis hash is empty")
	}

	return genesisHash, nil
}
