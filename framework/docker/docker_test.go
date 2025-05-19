package docker

import (
	"context"
	"github.com/moby/moby/client"
	"testing"

	"github.com/celestiaorg/tastora/framework/testutil/maps"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	"github.com/cosmos/cosmos-sdk/x/bank"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// DockerTestSuite is a test suite which can be used to perform tests which spin up a docker network.
type DockerTestSuite struct {
	suite.Suite
	ctx          context.Context
	dockerClient *client.Client
	networkID    string
	logger       *zap.Logger
	encConfig    testutil.TestEncodingConfig
	provider     *Provider
	chain        *Chain
}

// SetupSuite runs once before all tests in the suite.
func (s *DockerTestSuite) SetupSuite() {
	s.ctx = context.Background()

	// configure Bech32 prefix, this needs to be set as account.String() uses the global config.
	sdkConf := sdk.GetConfig()
	sdkConf.SetBech32PrefixForAccount("celestia", "celestiapub")
	sdkConf.Seal()

	s.dockerClient, s.networkID = Setup(s.T())

	s.logger = zaptest.NewLogger(s.T())
	s.encConfig = testutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{})

	s.provider = s.createDefaultProvider()
	chain, err := s.provider.GetChain(s.ctx)
	s.Require().NoError(err)
	s.chain = chain.(*Chain)

	err = s.chain.Start(s.ctx)
	s.Require().NoError(err)

	s.T().Cleanup(func() {
		err := s.chain.Stop(s.ctx)
		if err != nil {
			s.T().Logf("Failed to stop chain: %s", err)
		}
	})
}

// TearDownSuite removes docker resources.
func (s *DockerTestSuite) TearDownSuite() {
	Cleanup(s.T(), s.dockerClient)()
}

// createDefaultProvider returns a provider with the standard Celestia config used for all tests.
func (s *DockerTestSuite) createDefaultProvider() *Provider {
	numValidators := 1
	numFullNodes := 0

	cfg := Config{
		Logger:          s.logger,
		DockerClient:    s.dockerClient,
		DockerNetworkID: s.networkID,
		ChainConfig: &ChainConfig{
			ConfigFileOverrides: map[string]any{
				"config/app.toml":    appOverrides(),
				"config/config.toml": configOverrides(),
			},
			Type:          "celestia",
			Name:          "celestia",
			Version:       "v4.0.0-rc4",
			NumValidators: &numValidators,
			NumFullNodes:  &numFullNodes,
			ChainID:       "test",
			Images: []DockerImage{
				{
					Repository: "ghcr.io/celestiaorg/celestia-app",
					Version:    "v4.0.0-rc4",
					UIDGID:     "10001:10001",
				},
			},
			Bin:           "celestia-appd",
			Bech32Prefix:  "celestia",
			Denom:         "utia",
			CoinType:      "118",
			GasPrices:     "0.025utia",
			GasAdjustment: 1.3,
			ModifyGenesis: func(config Config, bytes []byte) ([]byte, error) {
				return maps.SetField(bytes, "consensus.params.version.app", "4")
			},
			EncodingConfig:      &s.encConfig,
			AdditionalStartArgs: []string{"--force-no-bbr", "--grpc.enable", "--grpc.address", "0.0.0.0:9090", "--rpc.grpc_laddr=tcp://0.0.0.0:9098"},
		},
	}

	return NewProvider(cfg, s.T())
}

// enable indexing of transactions so Broadcasting of transactions works.
func appOverrides() toml.Toml {
	tomlCfg := make(toml.Toml)
	txIndex := make(toml.Toml)
	txIndex["indexer"] = "kv"
	tomlCfg["tx-index"] = txIndex
	return tomlCfg
}

func configOverrides() toml.Toml {
	tomlCfg := make(toml.Toml)
	txIndex := make(toml.Toml)
	txIndex["indexer"] = "kv"
	tomlCfg["tx_index"] = txIndex
	return tomlCfg
}

func TestDockerSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	suite.Run(t, new(DockerTestSuite))
}
