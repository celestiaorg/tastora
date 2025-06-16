package docker

import (
	"context"
	"cosmossdk.io/math"
	"encoding/hex"
	"fmt"
	sdkacc "github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"math/rand"
	"testing"
)

func (s *DockerTestSuite) TestRollkit() {
	ctx := context.Background()
	daNetwork, err := s.provider.GetDataAvailabilityNetwork(ctx)
	s.Require().NoError(err)

	genesisHash := s.getGenesisHash(ctx)

	hostname, err := s.chain.GetNodes()[0].GetInternalHostName(ctx)
	s.Require().NoError(err, "failed to get internal hostname")

	bridgeNode := daNetwork.GetBridgeNodes()[0]
	chainID := s.chain.cfg.ChainConfig.ChainID

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

	rollkit, err := s.provider.GetRollkitChain(context.Background())
	s.Require().NoError(err)

	nodes := rollkit.GetNodes()
	s.Require().Len(nodes, 1)
	aggregatorNode := nodes[0]

	err = aggregatorNode.Init(context.Background())
	s.Require().NoError(err)

	// Get the Celestia address from the rollkit node
	rollkitAddress := aggregatorNode.(*RollkitNode).CelestiaAddress
	s.T().Logf("rollkit node celestia address: %s", rollkitAddress)

	// Fund the rollkit node address
	toAddress, err = sdk.AccAddressFromBech32(rollkitAddress)
	s.Require().NoError(err)

	// Fund the rollkit node wallet with coins
	bankSend = banktypes.NewMsgSend(fromAddress, toAddress, sdk.NewCoins(sdk.NewCoin("utia", math.NewInt(100_000_000_00))))
	_, err = s.chain.BroadcastMessages(ctx, s.chain.GetFaucetWallet(), bankSend)
	s.Require().NoError(err)

	bridgeNodeHostName, err := bridgeNode.GetInternalHostName()
	s.Require().NoError(err)

	authToken, err := bridgeNode.GetAuthToken()
	s.Require().NoError(err)
	s.T().Logf("auth token: %s", authToken)

	daAddress := fmt.Sprintf("http://%s:26658", bridgeNodeHostName)
	err = aggregatorNode.Start(context.Background(),
		"--rollkit.da.address", daAddress,
		"--rollkit.da.gas_price", "0.025",
		"--rollkit.da.auth_token", authToken,
		"--rollkit.rpc.address", "0.0.0.0:7331", // bind to 0.0.0.0 so rpc is reachable from test host.
		"--rollkit.da.namespace", GenerateValidNamespaceHex(),
	)
	s.Require().NoError(err)
}

func GenerateValidNamespaceHex() string {
	ns := make([]byte, 29)
	ns[0] = 0x00 // version 0
	// First 18 bytes of namespace ID must be zero → bytes 1-18
	for i := 1; i < 19; i++ {
		ns[i] = 0x00
	}
	// Last 10 bytes of namespace ID → random
	_, _ = rand.Read(ns[19:])
	return hex.EncodeToString(ns)
}
