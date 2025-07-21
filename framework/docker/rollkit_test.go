package docker

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"cosmossdk.io/math"
	"github.com/celestiaorg/go-square/v2/share"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/rollkit"
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

	authToken, err := bridgeNode.GetAuthToken()
	s.Require().NoError(err)

	// Use the configured RPC port instead of hardcoded 26658
	bridgeRPCAddress, err := bridgeNode.GetInternalRPCAddress()
	s.Require().NoError(err)
	daAddress := fmt.Sprintf("http://%s", bridgeRPCAddress)

	rollkitChain, err := NewChainBuilder(s.T()).
		WithStrategy(rollkit.NewStrategy("12345678")).
		WithImage(container.Image{
			Repository: "rollkit-gm",
			Version:    "latest",
			UIDGID:     "10001:10001",
		}).
		WithDenom("utia").
		WithDockerClient(s.dockerClient).
		WithName("rollkit").
		WithDockerNetworkID(s.networkID).
		WithChainID("rollkit-test").
		WithBech32Prefix("gm").
		WithBinaryName("gmd").
		WithNode(NewChainNodeConfigBuilder().
			// Create aggregator node with rollkit-specific start arguments
			WithAdditionalStartArgs(
				"--rollkit.da.address", daAddress,
				"--rollkit.da.gas_price", "0.025",
				"--rollkit.da.auth_token", authToken,
				"--rollkit.rpc.address", "0.0.0.0:7331", // bind to 0.0.0.0 so rpc is reachable from test host.
				"--rollkit.da.namespace", generateValidNamespaceHex(),
			).
			WithPostInit(func(ctx context.Context, node *ChainNode) error {
				// Rollkit needs validators in the genesis validators array
				// Let's create the simplest possible validator to match what staking produces
				
				// Read current genesis.json
				genesisBz, err := node.ReadFile(ctx, "config/genesis.json")
				if err != nil {
					return fmt.Errorf("failed to read genesis.json: %w", err)
				}

				// Parse as generic JSON
				var genDoc map[string]interface{}
				if err := json.Unmarshal(genesisBz, &genDoc); err != nil {
					return fmt.Errorf("failed to parse genesis.json: %w", err)
				}

				// Extract validator info from the gentx to create an exact match
				appState := genDoc["app_state"].(map[string]interface{})
				genutil := appState["genutil"].(map[string]interface{})
				genTxs := genutil["gen_txs"].([]interface{})
				
				if len(genTxs) > 0 {
					gentx := genTxs[0].(map[string]interface{})
					body := gentx["body"].(map[string]interface{})
					messages := body["messages"].([]interface{})
					createValMsg := messages[0].(map[string]interface{})
					
					// Get pubkey from gentx
					gentxPubkey := createValMsg["pubkey"].(map[string]interface{})
					pubkeyValue := gentxPubkey["key"].(string)
					
					// Decode to calculate address using CometBFT method
					pubkeyBytes, _ := base64.StdEncoding.DecodeString(pubkeyValue)
					// In CometBFT, validator address is first 20 bytes of SHA256(pubkey) 
					hash := sha256.Sum256(pubkeyBytes)
					address := strings.ToUpper(hex.EncodeToString(hash[:20]))
					
					// Try putting validators in consensus.validators as the bash script shows
					consensus := genDoc["consensus"].(map[string]interface{})
					consensus["validators"] = []map[string]interface{}{
						{
							"address": address,
							"pub_key": map[string]interface{}{
								"type":  "tendermint/PubKeyEd25519",
								"value": pubkeyValue,
							},
							"power": "1",
							"name":  "rollkit-validator",
						},
					}
				}

				// Marshal and write back
				updatedGenesis, err := json.MarshalIndent(genDoc, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal genesis: %w", err)
				}

				return node.WriteFile(ctx, "config/genesis.json", updatedGenesis)
			}).
			Build()).
		Build(ctx)

	s.Require().NoError(err)

	nodes := rollkitChain.GetNodes()
	s.Require().Len(nodes, 1)

	err = rollkitChain.Start(ctx)
	s.Require().NoError(err)
}

func generateValidNamespaceHex() string {
	return hex.EncodeToString(share.RandomBlobNamespaceID())
}