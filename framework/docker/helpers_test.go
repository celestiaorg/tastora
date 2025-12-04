package docker

import (
	"context"
	"fmt"
	"testing"

	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	da "github.com/celestiaorg/tastora/framework/docker/dataavailability"
	evmsingle "github.com/celestiaorg/tastora/framework/docker/evstack/evmsingle"
	reth "github.com/celestiaorg/tastora/framework/docker/evstack/reth"
	sdkacc "github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

type Stack struct {
	celestia *cosmos.Chain
	da       *da.Network
	reth     *reth.Node
	evm      *evmsingle.Chain
}

// DeployCelestiaWithDABridgeNode deploys celestia-app and a celestia-node bridge, returning
// the chain, DA network.
func DeployCelestiaWithDABridgeNode(t *testing.T, cfg *TestSetupConfig) (*cosmos.Chain, *da.Network, error) {
	t.Helper()

	ctx := context.Background()

	chain, err := cfg.ChainBuilder.Build(ctx)
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

	bridgeCfg := da.NewNodeBuilder().WithNodeType(types.BridgeNode).Build()
	danet, err := cfg.DANetworkBuilder.
		WithNodes(bridgeCfg).
		WithChainID(chainID).
		Build(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("build da network: %w", err)
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
		return nil, nil, fmt.Errorf("start da bridge: %w", err)
	}

	// fund DA wallet for data submissions
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

	return chain, danet, nil
}

// DeployRethWithEVMSingle starts reth and evm-single wired together and to the provided DA address.
func DeployRethWithEVMSingle(t *testing.T, cfg *TestSetupConfig, danet *da.Network) (*reth.Node, *evmsingle.Chain, error) {
	t.Helper()
	ctx := context.Background()

	rnode, err := cfg.RethBuilder.Build(ctx)
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

	bridgeNodeNetworkInfo, err := danet.GetBridgeNodes()[0].GetNetworkInfo(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("bridge network info: %w", err)
	}
	daAddress := fmt.Sprintf("http://%s:%s", bridgeNodeNetworkInfo.Internal.IP, bridgeNodeNetworkInfo.Internal.Ports.RPC)

	evNodeCfg := evmsingle.NewNodeConfigBuilder().
		WithEVMEngineURL(evmEngineURL).
		WithEVMETHURL(evmEthURL).
		WithEVMJWTSecret(rnode.JWTSecretHex()).
		WithEVMSignerPassphrase("secret").
		WithEVMBlockTime("1s").
		WithEVMGenesisHash(rGenesisHash).
		WithDAAddress(daAddress).
		Build()

	evmSingle, err := cfg.EVMSingleChainBuilder.WithNodes(evNodeCfg).Build(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("build evm-single: %w", err)
	}
	if err := evmSingle.Start(ctx); err != nil {
		return nil, nil, fmt.Errorf("start evm-single: %w", err)
	}

	return rnode, evmSingle, nil
}

// DeployMinimalStack is a test helper that spins up a simple, fully-wired stack using defaults:
// - celestia-app (1 validator)
// - celestia-node (1 bridge) pointed at celestia-app
// - reth (1 node)
// - evm-single (1 node) pointed at reth + DA
func DeployMinimalStack(t *testing.T, cfg *TestSetupConfig) (Stack, error) {
	t.Helper()

	chain, daNetwork, err := DeployCelestiaWithDABridgeNode(t, cfg)
	if err != nil {
		return Stack{}, err
	}

	rethNode, evChain, err := DeployRethWithEVMSingle(t, cfg, daNetwork)
	if err != nil {
		return Stack{}, err
	}

	return Stack{celestia: chain, da: daNetwork, reth: rethNode, evm: evChain}, nil
}
