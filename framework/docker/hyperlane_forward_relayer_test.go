package docker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/hyperlane"
	"github.com/cosmos/cosmos-sdk/crypto"
	"github.com/stretchr/testify/require"
)

func TestForwardRelayerAndBackendStart(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	testCfg := setupDockerTest(t)
	ctx, cancel := context.WithTimeout(testCfg.Ctx, 8*time.Minute)
	defer cancel()

	// Keep parity with other forward relayer coverage that targets this image variant.
	testCfg.ChainBuilder = testCfg.ChainBuilder.WithImage(
		container.NewImage("ghcr.io/celestiaorg/celestia-app-standalone", "feature-zk-execution-ism", "10001:10001"),
	)

	celestia, err := testCfg.ChainBuilder.Build(ctx)
	require.NoError(t, err)
	require.NoError(t, celestia.Start(ctx))

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
	require.NoError(t, backend.Start(ctx))

	backendInfo, err := backend.GetNetworkInfo(ctx)
	require.NoError(t, err)
	require.Equal(t, backendCfg.Settings.PortValue(), backendInfo.Internal.Ports.HTTP)
	require.NotEmpty(t, backendInfo.External.Ports.HTTP)
	require.NoError(t, backend.ContainerLifecycle.Running(ctx))

	networkInfo, err := celestia.GetNetworkInfo(ctx)
	require.NoError(t, err)

	keyring, err := celestia.GetNode().GetKeyring()
	require.NoError(t, err)

	armor, err := keyring.ExportPrivKeyArmor(celestia.GetFaucetWallet().GetKeyName(), "")
	require.NoError(t, err)

	privKey, _, err := crypto.UnarmorDecryptPrivKey(armor, "")
	require.NoError(t, err)

	relayerCfg := hyperlane.ForwardRelayerConfig{
		Logger:          testCfg.Logger,
		DockerClient:    testCfg.DockerClient,
		DockerNetworkID: testCfg.NetworkID,
		Image:           hyperlane.DefaultForwardRelayerImage(),
		Settings: hyperlane.ForwardRelayerSettings{
			CelestiaGRPC:  networkInfo.Internal.GRPCAddress(),
			BackendURL:    fmt.Sprintf("http://%s", backendInfo.Internal.HTTPAddress()),
			PrivateKeyHex: fmt.Sprintf("0x%x", privKey.Bytes()),
		},
	}

	relayer, err := hyperlane.NewForwardRelayer(ctx, relayerCfg, t.Name(), hyperlane.RelayerMode)
	require.NoError(t, err)
	require.NoError(t, relayer.Start(ctx))
	require.NoError(t, relayer.ContainerLifecycle.Running(ctx))

	t.Cleanup(func() {
		_ = celestia.Stop(ctx)
		_ = backend.Stop(ctx)
		_ = relayer.Stop(ctx)
	})
}
