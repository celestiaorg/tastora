package e2e

import (
	"context"
	"fmt"
	"time"

	"github.com/celestiaorg/celestia-app/v4/app"
	"github.com/celestiaorg/go-square/v2/share"
	"github.com/celestiaorg/tastora/framework/docker"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/testutil/wallet"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

const (
	// DefaultCelestiaAppImage is the default Docker image for Celestia App
	DefaultCelestiaAppImage = "ghcr.io/celestiaorg/celestia-app"
	// DefaultCelestiaAppVersion is the default version for Celestia App
	DefaultCelestiaAppVersion = "v4.0.0-rc6"
	// DefaultCelestiaNodeImage is the default Docker image for Celestia Node
	DefaultCelestiaNodeImage = "ghcr.io/celestiaorg/celestia-node"
	// DefaultCelestiaNodeVersion is the default version for Celestia Node
	DefaultCelestiaNodeVersion = "v0.23.0-mocha"
	// DefaultChainID is the default chain ID for test networks
	DefaultChainID = "test"
	// DefaultDenom is the default denomination for the test network
	DefaultDenom = "utia"
)

// CelestiaTestSuite provides a reusable test suite for Celestia end-to-end testing
type CelestiaTestSuite struct {
	suite.Suite
	ctx          context.Context
	dockerClient *client.Client
	networkID    string
	logger       *zap.Logger
	encConfig    testutil.TestEncodingConfig
	provider     *docker.Provider

	// Network components
	Chain      types.Chain
	BridgeNode types.DANode
	LightNode  types.DANode
}

// SetupSuite initializes the test suite and sets up the Celestia network
func (s *CelestiaTestSuite) SetupSuite() {
	s.ctx = context.Background()
	s.dockerClient, s.networkID = docker.DockerSetup(s.T())
	s.logger = zaptest.NewLogger(s.T())

	// Configure SDK for Celestia
	s.configureCelestiaSDK()

	s.encConfig = testutil.MakeTestEncodingConfig(app.ModuleEncodingRegisters...)
	s.provider = s.createProvider()

	// Start the network components
	s.Chain = s.createAndStartChain()
	s.BridgeNode = s.createAndStartBridgeNode()

	// Setup cleanup
	s.T().Cleanup(func() {
		s.cleanup()
	})
}

// TearDownSuite cleans up Docker resources
func (s *CelestiaTestSuite) TearDownSuite() {
	docker.DockerCleanup(s.T(), s.dockerClient)()
}

// configureCelestiaSDK sets up SDK configuration for Celestia
func (s *CelestiaTestSuite) configureCelestiaSDK() {
	sdkConf := sdk.GetConfig()
	if sdkConf.GetBech32AccountAddrPrefix() != "celestia" {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Info("SDK config already sealed, continuing with existing configuration")
			}
		}()
		sdkConf.SetBech32PrefixForAccount("celestia", "celestiapub")
		sdkConf.Seal()
	}
}

// createProvider creates a Docker provider with standard Celestia configuration
func (s *CelestiaTestSuite) createProvider() *docker.Provider {
	numValidators := 1

	cfg := docker.Config{
		Logger:          s.logger,
		DockerClient:    s.dockerClient,
		DockerNetworkID: s.networkID,
		ChainConfig: &docker.ChainConfig{
			ConfigFileOverrides: map[string]any{
				"config/app.toml":    s.AppOverrides(),
				"config/config.toml": s.ConfigOverrides(),
			},
			Type:          "celestia",
			Name:          "celestia",
			Version:       DefaultCelestiaAppVersion,
			NumValidators: &numValidators,
			ChainID:       DefaultChainID,
			Images: []docker.DockerImage{
				{
					Repository: DefaultCelestiaAppImage,
					Version:    DefaultCelestiaAppVersion,
					UIDGID:     "10001:10001",
				},
			},
			Bin:            "celestia-appd",
			Bech32Prefix:   "celestia",
			Denom:          DefaultDenom,
			CoinType:       "118",
			GasPrices:      "0.1utia",
			GasAdjustment:  2.0,
			EncodingConfig: &s.encConfig,
			AdditionalStartArgs: []string{
				"--force-no-bbr",
				"--grpc.enable",
				"--grpc.address", "0.0.0.0:9090",
				"--rpc.grpc_laddr=tcp://0.0.0.0:9098",
				"--timeout-commit", "1s",
			},
		},
		DANodeConfig: &docker.DANodeConfig{
			ChainID: DefaultChainID,
			Images: []docker.DockerImage{
				{
					Repository: DefaultCelestiaNodeImage,
					Version:    DefaultCelestiaNodeVersion,
					UIDGID:     "10001:10001",
				},
			},
		},
	}

	return docker.NewProvider(cfg, s.T())
}

// AppOverrides returns the standard app.toml configuration overrides for Celestia testing
func (s *CelestiaTestSuite) AppOverrides() toml.Toml {
	tomlCfg := make(toml.Toml)
	txIndex := make(toml.Toml)
	txIndex["indexer"] = "kv"
	tomlCfg["tx-index"] = txIndex
	return tomlCfg
}

// ConfigOverrides returns the standard config.toml configuration overrides for Celestia testing
func (s *CelestiaTestSuite) ConfigOverrides() toml.Toml {
	tomlCfg := make(toml.Toml)
	txIndex := make(toml.Toml)
	txIndex["indexer"] = "kv"
	tomlCfg["tx_index"] = txIndex
	return tomlCfg
}

// createAndStartChain creates and starts a Celestia chain
func (s *CelestiaTestSuite) createAndStartChain() types.Chain {
	chain, err := s.provider.GetChain(s.ctx)
	s.Require().NoError(err)

	err = chain.Start(s.ctx)
	s.Require().NoError(err)

	// Verify the chain is producing blocks
	height, err := chain.Height(s.ctx)
	s.Require().NoError(err)
	s.Require().Greater(height, int64(0))

	s.logger.Info("Celestia chain started successfully", zap.Int64("height", height))
	return chain
}

// createAndStartBridgeNode creates and starts a DA bridge node
func (s *CelestiaTestSuite) createAndStartBridgeNode() types.DANode {
	// Get the validator's core IP and genesis hash
	validatorNode := s.Chain.GetNodes()[0]
	coreIP, err := validatorNode.GetInternalHostName(s.ctx)
	s.Require().NoError(err)

	genesisHash, err := s.GetGenesisBlockHash()
	s.Require().NoError(err)

	// Create and start DA bridge node
	bridgeNode, err := s.provider.GetDANode(s.ctx, types.BridgeNode)
	s.Require().NoError(err)

	err = bridgeNode.Start(s.ctx,
		types.WithCoreIP(coreIP),
		types.WithGenesisBlockHash(genesisHash),
	)
	s.Require().NoError(err)

	s.logger.Info("DA bridge node started successfully",
		zap.String("core_ip", coreIP),
		zap.String("genesis_hash", genesisHash))

	return bridgeNode
}

// GetGenesisBlockHash retrieves the genesis block hash from the validator
func (s *CelestiaTestSuite) GetGenesisBlockHash() (string, error) {
	node := s.Chain.GetNodes()[0]
	rpcClient, err := node.GetRPCClient()
	if err != nil {
		return "", err
	}

	height := int64(1)
	block, err := rpcClient.Block(s.ctx, &height)
	if err != nil {
		return "", err
	}

	return block.BlockID.Hash.String(), nil
}

// CreateTestWallet creates and funds a test wallet
func (s *CelestiaTestSuite) CreateTestWallet(name string, amount int64) types.Wallet {
	sendAmount := sdk.NewCoins(sdk.NewInt64Coin(DefaultDenom, amount))
	testWallet, err := wallet.CreateAndFund(s.ctx, name, sendAmount, s.Chain)
	s.Require().NoError(err, "failed to create test wallet")
	s.Require().NotNil(testWallet, "wallet is nil")
	return testWallet
}

// CreateRandomBlob creates a random blob for testing
func (s *CelestiaTestSuite) CreateRandomBlob(data []byte) (*share.Blob, share.Namespace) {
	namespace := share.RandomNamespace()
	blob, err := share.NewBlob(namespace, data, share.ShareVersionZero, nil)
	s.Require().NoError(err)
	return blob, namespace
}

// WaitForDASync waits for a DA node to sync to a specific height with blob data
func (s *CelestiaTestSuite) WaitForDASync(daNode types.DANode, targetHeight uint64, namespace share.Namespace, timeout time.Duration) error {
	s.logger.Info("Waiting for DA node to sync",
		zap.Uint64("target_height", targetHeight),
		zap.String("namespace", namespace.String()))

	ctx, cancel := context.WithTimeout(s.ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for DA sync to height %d", targetHeight)
		case <-ticker.C:
			blobs, err := daNode.GetAllBlobs(ctx, targetHeight, []share.Namespace{namespace})
			if err != nil {
				// Expected errors during sync
				if isExpectedSyncError(err) {
					continue
				}
				return fmt.Errorf("unexpected error during sync: %w", err)
			}

			if len(blobs) > 0 {
				s.logger.Info("DA node successfully synced",
					zap.Uint64("height", targetHeight),
					zap.Int("blob_count", len(blobs)))
				return nil
			}
		}
	}
}

// isExpectedSyncError checks if an error is expected during DA sync
func isExpectedSyncError(err error) bool {
	errStr := err.Error()
	return contains(errStr, "blob: not found") ||
		contains(errStr, "syncing in progress") ||
		contains(errStr, "connection refused") ||
		contains(errStr, "dial tcp")
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(len(substr) == 0 || findSubstring(s, substr))
}

// findSubstring finds if substr exists in s
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// cleanup stops all nodes and cleans up resources
func (s *CelestiaTestSuite) cleanup() {
	if s.LightNode != nil {
		if err := s.LightNode.Stop(s.ctx); err != nil {
			s.T().Logf("Failed to stop light node: %s", err)
		}
	}

	if s.BridgeNode != nil {
		if err := s.BridgeNode.Stop(s.ctx); err != nil {
			s.T().Logf("Failed to stop bridge node: %s", err)
		}
	}
	if s.Chain != nil {
		if err := s.Chain.Stop(s.ctx); err != nil {
			s.T().Logf("Failed to stop chain: %s", err)
		}
	}
}
