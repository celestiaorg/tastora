package docker

import (
	"context"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/cosmos"
	"github.com/celestiaorg/tastora/framework/docker/ibc"
	"github.com/celestiaorg/tastora/framework/docker/ibc/relayer"
	"github.com/celestiaorg/tastora/framework/testutil/config"
	"github.com/celestiaorg/tastora/framework/types"
	servercfg "github.com/cosmos/cosmos-sdk/server/config"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	dockerclient "github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"testing"
)

// createCelestiaChain creates a celestia-app chain for IBC testing
func createCelestiaChain(t *testing.T, ctx context.Context, client *dockerclient.Client, networkID string, encConfig testutil.TestEncodingConfig, testName string) (types.Chain, error) {
	builder := cosmos.NewChainBuilderWithTestName(t, testName).
		WithDockerClient(client).
		WithDockerNetworkID(networkID).
		WithChainID("chain-a").
		WithName("celestia").
		WithImage(container.NewImage("ghcr.io/celestiaorg/celestia-app", "v5.0.10", "10001:10001")).
		WithBinaryName("celestia-appd").
		WithBech32Prefix("celestia").
		WithDenom("utia").
		WithGasPrices("0.000001utia").
		WithEncodingConfig(&encConfig).
		WithAdditionalStartArgs(
			"--force-no-bbr",
			"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
			"--timeout-commit", "1s",
		).
		WithPostInit(func(ctx context.Context, node *cosmos.ChainNode) error {
			return config.Modify(ctx, node, "config/app.toml", func(cfg *servercfg.Config) {
				cfg.MinGasPrices = "0.000001utia"
				cfg.GRPC.Enable = true
				cfg.GRPC.Address = "0.0.0.0:9090"
				cfg.API.Enable = true
				cfg.MinGasPrices = "0.0utia"
			})
		}).
		WithNode(cosmos.NewChainNodeConfigBuilder().Build())

	return builder.Build(ctx)
}

// createSimappChain creates an IBC-Go simapp chain for IBC testing
func createSimappChain(t *testing.T, ctx context.Context, client *dockerclient.Client, networkID string, encConfig testutil.TestEncodingConfig, testName string) (types.Chain, error) {
	builder := cosmos.NewChainBuilderWithTestName(t, testName).
		WithDockerClient(client).
		WithDockerNetworkID(networkID).
		WithChainID("chain-b").
		WithName("simapp").
		// use the simapp from ibc-go as a simple app with basic wiring and no token filters.
		// TODO: this is a custom built simapp that has the bech32prefix as "celestia" as a workaround for the global
		// SDK config not being usable when 2 chains have a different beck32 preix (e.g. "celestia" and "cosmos" ) if it is sealed.
		WithImage(container.NewImage("ghcr.io/chatton/ibc-go-simd", "v8.5.0", "1000:1000")).
		WithBinaryName("simd").
		WithBech32Prefix("celestia").
		WithDenom("stake").
		WithGasPrices("0.000001stake").
		WithEncodingConfig(&encConfig).
		WithPostInit(func(ctx context.Context, node *cosmos.ChainNode) error {
			return config.Modify(ctx, node, "config/app.toml", func(cfg *servercfg.Config) {
				cfg.MinGasPrices = "0.000001stake"
				cfg.GRPC.Enable = true
				cfg.API.Enable = true
				cfg.API.EnableUnsafeCORS = true
			})
		}).
		WithNode(cosmos.NewChainNodeConfigBuilder().Build())

	return builder.Build(ctx)
}

// setupIBCConnection establishes a complete IBC connection and channel
func setupIBCConnection(t *testing.T, ctx context.Context, chainA, chainB types.Chain, hermes *relayer.Hermes) (ibc.Connection, ibc.Channel) {
	// create clients
	err := hermes.CreateClients(ctx, chainA, chainB)
	require.NoError(t, err)

	// create connections
	connection, err := hermes.CreateConnections(ctx, chainA, chainB)
	require.NoError(t, err)
	require.NotEmpty(t, connection.ConnectionID, "Connection ID should not be empty")

	// Create an ICS20 channel for token transfers
	channelOpts := ibc.CreateChannelOptions{
		SourcePortName: "transfer",
		DestPortName:   "transfer",
		Order:          ibc.OrderUnordered,
		Version:        "ics20-1",
	}

	channel, err := hermes.CreateChannel(ctx, chainA, connection, channelOpts)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.NotEmpty(t, channel.ChannelID, "Channel ID should not be empty")

	t.Logf("Created IBC connection: %s <-> %s", connection.ConnectionID, connection.CounterpartyID)
	t.Logf("Created IBC channel: %s <-> %s", channel.ChannelID, channel.CounterpartyID)

	return connection, channel
}
