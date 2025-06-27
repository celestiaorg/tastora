package docker

import (
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"cosmossdk.io/math"
	"github.com/celestiaorg/go-square/v2/share"
	sdkacc "github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

func (s *DockerTestSuite) TestRollkit() {
	ctx := context.Background()

	var err error
	s.provider = s.CreateDockerProvider()
	s.chain, err = s.builder.Build(s.ctx)
	s.Require().NoError(err)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	daNetwork, err := s.provider.GetDataAvailabilityNetwork(ctx)
	s.Require().NoError(err)

	genesisHash := s.getGenesisHash(ctx)

	hostname, err := s.chain.GetNodes()[0].GetInternalHostName(ctx)
	s.Require().NoError(err, "failed to get internal hostname")

	bridgeNode := daNetwork.GetBridgeNodes()[0]
	chainID := s.chain.GetChainID()

	s.T().Run("bridge node can be started", func(t *testing.T) {
		err = bridgeNode.Start(ctx,
			types.WithChainID(chainID),
			types.WithAdditionalStartArguments("--p2p.network", chainID, "--core.ip", hostname, "--rpc.addr", "0.0.0.0"),
			types.WithEnvironmentVariables(
				map[string]string{
					"CELESTIA_CUSTOM": types.BuildCelestiaCustomEnvVar(chainID, genesisHash, ""),
					"P2P_NETWORK":     chainID,
				},
			),
		)
		s.Require().NoError(err)
	})

	daWallet, err := bridgeNode.GetWallet()
	s.Require().NoError(err)
	s.T().Logf("da node celestia address: %s", daWallet.GetFormattedAddress())

	// Fund the da node address
	fromAddress, err := sdkacc.AddressFromWallet(s.chain.GetFaucetWallet())
	s.Require().NoError(err)

	toAddress, err := sdk.AccAddressFromBech32(daWallet.GetFormattedAddress())
	s.Require().NoError(err)

	// Fund the rollkit node wallet with coins
	bankSend := banktypes.NewMsgSend(fromAddress, toAddress, sdk.NewCoins(sdk.NewCoin("utia", math.NewInt(100_000_000_00))))
	_, err = s.chain.BroadcastMessages(ctx, s.chain.GetFaucetWallet(), bankSend)
	s.Require().NoError(err)

	rollkit, err := s.provider.GetRollkitChain(ctx)
	s.Require().NoError(err)

	nodes := rollkit.GetNodes()
	s.Require().Len(nodes, 1)
	aggregatorNode := nodes[0]

	err = aggregatorNode.Init(ctx)
	s.Require().NoError(err)

	authToken, err := bridgeNode.GetAuthToken()
	s.Require().NoError(err)

	// Use the configured RPC port instead of hardcoded 26658
	bridgeRPCAddress, err := bridgeNode.GetInternalRPCAddress()
	s.Require().NoError(err)
	daAddress := fmt.Sprintf("http://%s", bridgeRPCAddress)
	err = aggregatorNode.Start(ctx,
		"--rollkit.da.address", daAddress,
		"--rollkit.da.gas_price", "0.025",
		"--rollkit.da.auth_token", authToken,
		"--rollkit.rpc.address", "0.0.0.0:7331", // bind to 0.0.0.0 so rpc is reachable from test host.
		"--rollkit.da.namespace", generateValidNamespaceHex(),
	)
	s.Require().NoError(err)
}

func generateValidNamespaceHex() string {
	return hex.EncodeToString(share.RandomBlobNamespaceID())
}
