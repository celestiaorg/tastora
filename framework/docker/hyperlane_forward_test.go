package docker

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"github.com/celestiaorg/tastora/framework/testutil/deploy"
	wallet "github.com/celestiaorg/tastora/framework/testutil/wallet"
	"github.com/cosmos/cosmos-sdk/crypto"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func TestHyperlaneForwardRelayer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	testCfg := setupDockerTest(t)
	ctx, cancel := context.WithTimeout(testCfg.Ctx, 12*time.Minute)
	defer cancel()

	testCfg.ChainBuilder = testCfg.ChainBuilder.WithImage(container.NewImage("ghcr.io/celestiaorg/celestia-app-standalone", "feature-zk-execution-ism", "10001:10001"))

	celestia, daNetwork, err := deploy.CelestiaWithDA(ctx, testCfg.ChainBuilder, testCfg.DANetworkBuilder)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = celestia.Stop(ctx)
	})

	bridge := daNetwork.GetBridgeNodes()[0]
	bridgeNodeNetworkInfo, err := bridge.GetNetworkInfo(ctx)
	require.NoError(t, err)
	daAddress := fmt.Sprintf("http://%s:%s", bridgeNodeNetworkInfo.Internal.IP, bridgeNodeNetworkInfo.Internal.Ports.RPC)
	_ = daAddress
	// reth0, _ := BuildEvolveEVM(t, ctx, testCfg, daAddress, reth0ChainName, reth0ChainID)
	// reth1, _ := BuildEvolveEVM(t, ctx, testCfg, daAddress, reth1ChainName, reth1ChainID)

	// d, err := hyperlane.NewDeployer(
	// 	ctx,
	// 	hyperlane.Config{
	// 		Logger:          testCfg.Logger,
	// 		DockerClient:    testCfg.DockerClient,
	// 		DockerNetworkID: testCfg.NetworkID,
	// 		HyperlaneImage:  hyperlane.DefaultDeployerImage(),
	// 	},
	// 	testCfg.TestName,
	// 	[]hyperlane.ChainConfigProvider{reth0, reth1, celestia},
	// )
	// require.NoError(t, err)

	// relayerBytes, err := d.ReadFile(ctx, "relayer-config.json")
	// require.NoError(t, err)

	// var relayerCfg hyperlane.RelayerConfig
	// require.NoError(t, json.Unmarshal(relayerBytes, &relayerCfg))
	// require.NotEmpty(t, relayerCfg.Chains[reth0ChainName])
	// require.NotEmpty(t, relayerCfg.Chains[reth1ChainName])
	// require.NotEmpty(t, relayerCfg.Chains[HypChainName])

	// require.NoError(t, d.Deploy(ctx))

	// schema, err := d.GetOnDiskSchema(ctx)
	// require.NoError(t, err)

	// assertMailbox(t, ctx, schema, reth0, reth0ChainName)
	// assertMailbox(t, ctx, schema, reth1, reth1ChainName)

	// broadcaster := cosmos.NewBroadcaster(celestia)
	// faucet := celestia.GetNode().GetFaucetWallet()

	// config, err := d.DeployCosmosNoopISM(ctx, broadcaster, faucet)
	// require.NoError(t, err)
	// require.NotNil(t, config)

	// cosmosEntry, ok := schema.Registry.Chains[HypChainName]
	// require.True(t, ok, "missing registry entry for %s", HypChainName)
	// cosmosDomain := cosmosEntry.Metadata.DomainID

	// networkInfo, err := celestia.GetNetworkInfo(ctx)
	// require.NoError(t, err)

	// warpTokens, err := queryWarpTokens(ctx, networkInfo.External.GRPCAddress())
	// require.NoError(t, err)
	// require.NotEmpty(t, warpTokens)
	// routerHex := warpTokens[0].Id

	// tokenRouter, err := d.GetEVMWarpTokenAddress()
	// require.NoError(t, err)

	// enrollRemote := func(chainName string, node *reth.Node) {
	// 	t.Helper()

	// 	networkInfo, err := node.GetNetworkInfo(ctx)
	// 	require.NoError(t, err)

	// 	rpcURL := fmt.Sprintf("http://%s", networkInfo.External.RPCAddress())

	// 	txHash, err := d.EnrollRemoteRouter(ctx, tokenRouter.Hex(), cosmosDomain, routerHex, chainName, rpcURL)
	// 	require.NoError(t, err)
	// 	t.Logf("Enrolled remote router for %s: %s", chainName, txHash.Hex())

	// 	entry, ok := schema.Registry.Chains[chainName]
	// 	require.True(t, ok, "missing registry entry for %s", chainName)
	// 	evmDomain := entry.Metadata.DomainID

	// 	remoteTokenRouter := evm.PadAddress(tokenRouter) // leftpad to bytes32
	// 	require.NoError(t, d.EnrollRemoteRouterOnCosmos(ctx, broadcaster, faucet, config.TokenID, evmDomain, remoteTokenRouter.String()))
	// }

	// enrollRemote(reth0ChainName, reth0)
	// enrollRemote(reth1ChainName, reth1)

	backendCfg := hyperlane.ForwardRelayerConfig{
		Logger:          testCfg.Logger,
		DockerClient:    testCfg.DockerClient,
		DockerNetworkID: testCfg.NetworkID,
		Image:           hyperlane.DefaultForwardRelayerImage(),
		Settings: hyperlane.ForwardRelayerSettings{
			Port: "8080",
		},
	}

	backend, err := hyperlane.NewForwardRelayer(ctx, backendCfg, t.Name(), hyperlane.BackendMode)
	require.NoError(t, err)

	err = backend.Start(ctx)
	require.NoError(t, err)

	backendNetworkInfo, err := backend.GetNetworkInfo(ctx)
	require.NoError(t, err)
	require.Equal(t, backendCfg.Settings.PortValue(), backendNetworkInfo.Internal.Ports.HTTP)
	require.NotEmpty(t, backendNetworkInfo.External.Ports.HTTP)

	backendExternalURL := fmt.Sprintf("http://%s", backendNetworkInfo.External.HTTPAddress())
	require.Eventually(t, func() bool {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, reqErr := client.Get(backendExternalURL)
		if reqErr != nil {
			return false
		}
		_ = resp.Body.Close()
		return true
	}, 30*time.Second, time.Second)

	amount := sdkmath.NewInt(1000000)
	sendAmount := sdk.NewCoins(sdk.NewCoin(celestia.Config.Denom, amount))
	forwardRlyWallet, err := wallet.CreateAndFund(ctx, "forward-rly", sendAmount, celestia)
	require.NoError(t, err)

	chainNode := celestia.GetNode()
	keyring, err := chainNode.GetKeyring()
	require.NoError(t, err)

	armor, err := keyring.ExportPrivKeyArmor(forwardRlyWallet.GetKeyName(), "")
	require.NoError(t, err)

	privKey, _, err := crypto.UnarmorDecryptPrivKey(armor, "")
	require.NoError(t, err)

	networkInfo, err := celestia.GetNetworkInfo(ctx)
	require.NoError(t, err)

	forwardRlyCfg := hyperlane.ForwardRelayerConfig{
		Logger:          testCfg.Logger,
		DockerClient:    testCfg.DockerClient,
		DockerNetworkID: testCfg.NetworkID,
		Image:           hyperlane.DefaultForwardRelayerImage(),
		Settings: hyperlane.ForwardRelayerSettings{
			ChainID:       celestia.GetChainID(),
			CelestiaRPC:   fmt.Sprintf("http://%s", networkInfo.Internal.RPCAddress()),
			CelestiaGRPC:  networkInfo.Internal.GRPCAddress(),
			BackendURL:    fmt.Sprintf("http://%s", backendNetworkInfo.Internal.HTTPAddress()),
			PrivateKeyHex: fmt.Sprintf("0x%x", privKey.Bytes()),
		},
	}

	forwardRly, err := hyperlane.NewForwardRelayer(ctx, forwardRlyCfg, t.Name(), hyperlane.RelayerMode)
	require.NoError(t, err)

	err = forwardRly.Start(ctx)
	require.NoError(t, err)

	forwardRlyNetworkInfo, err := forwardRly.GetNetworkInfo(ctx)
	require.NoError(t, err)
	require.Empty(t, forwardRlyNetworkInfo.External.Ports.HTTP)

}
