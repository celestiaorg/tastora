package e2e_test

import (
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/e2e"
	"github.com/stretchr/testify/suite"
)

// ExampleCelestiaTestSuite demonstrates how to use the reusable CelestiaTestSuite
type ExampleCelestiaTestSuite struct {
	e2e.CelestiaTestSuite
}

// TestBasicNetworkFunctionality demonstrates basic network functionality
func (s *ExampleCelestiaTestSuite) TestBasicNetworkFunctionality() {
	// Verify the chain is producing blocks
	height, err := s.Chain.Height(s.T().Context())
	s.Require().NoError(err)
	s.Require().Greater(height, int64(0))

	// Verify bridge node connectivity
	p2pInfo, err := s.BridgeNode.GetP2PInfo(s.T().Context())
	s.Require().NoError(err)
	s.Assert().NotEmpty(p2pInfo.PeerID)

	s.T().Logf("Network is healthy - Chain height: %d, Bridge PeerID: %s", height, p2pInfo.PeerID)
}

// TestCreateTestWallet demonstrates wallet creation and funding
func (s *ExampleCelestiaTestSuite) TestCreateTestWallet() {
	// Create and fund a test wallet with 10000000 utia
	wallet := s.CreateTestWallet("example-wallet", 10000000)
	s.Assert().NotNil(wallet)
	s.Assert().NotEmpty(wallet.GetFormattedAddress())

	s.T().Logf("Created wallet: %s", wallet.GetFormattedAddress())
}

// TestBlobCreation demonstrates blob creation utilities
func (s *ExampleCelestiaTestSuite) TestBlobCreation() {
	testData := []byte("This is example blob data for testing")
	blob, namespace := s.CreateRandomBlob(testData)

	s.Assert().NotNil(blob)
	s.Assert().NotEmpty(namespace)
	s.Assert().Equal(testData, blob.Data)

	s.T().Logf("Created blob with namespace: %s", namespace.String())
}

// TestBridgeNodeConnectivity demonstrates how to test DA bridge node functionality
func (s *ExampleCelestiaTestSuite) TestBridgeNodeConnectivity() {
	// Verify bridge node connectivity
	p2pInfo, err := s.BridgeNode.GetP2PInfo(s.T().Context())
	s.Require().NoError(err)
	s.Assert().NotEmpty(p2pInfo.PeerID)

	// Light node functionality is currently disabled due to P2P networking issues
	// TODO: Re-enable light node testing in a separate PR

	s.T().Logf("Bridge node connectivity verified - PeerID: %s", p2pInfo.PeerID)
}

// TestDASync demonstrates DA synchronization testing
func (s *ExampleCelestiaTestSuite) TestDASync() {
	// This test demonstrates how to use WaitForDASync for DA testing
	// In a real scenario, you would submit a blob first and then wait for sync

	testData := []byte("Sync test data")
	_, namespace := s.CreateRandomBlob(testData)

	// Example: Wait for DA sync (this will timeout as no blob was actually submitted)
	err := s.WaitForDASync(s.BridgeNode, 1, namespace, 5*time.Second)

	// We expect this to fail since no blob was submitted, demonstrating error handling
	s.Assert().Error(err)
	s.T().Logf("DA sync test completed (expected timeout): %v", err)
}

// TestExampleCelestiaTestSuite runs the example test suite
func TestExampleCelestiaTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}
	suite.Run(t, new(ExampleCelestiaTestSuite))
}
