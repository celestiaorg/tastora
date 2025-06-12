# Celestia E2E Testing Framework

This package provides a reusable test suite for Celestia end-to-end testing, extracted from common patterns used across Celestia projects. It simplifies the setup and management of multi-node Celestia networks for testing data availability scenarios.

## Features

- **Automated Network Setup**: Automatically provisions and starts a Celestia validator with DA bridge node
- **Multi-Node Support**: Support for additional DA bridge nodes connected to the network
- **Reusable Test Patterns**: Common testing utilities for wallets, blobs, and DA synchronization
- **Proper Cleanup**: Automatic resource cleanup and Docker container management
- **Configurable**: Customizable Docker images, versions, and network parameters
- **Error Handling**: Built-in handling for common DA network sync issues

## Quick Start

### Basic Usage

```go
package mytest

import (
    "testing"
    "github.com/celestiaorg/tastora/framework/e2e"
    "github.com/stretchr/testify/suite"
)

type MyTestSuite struct {
    e2e.CelestiaTestSuite
}

func (s *MyTestSuite) TestMyScenario() {
    // The network is already running with:
    // - s.Chain: Celestia validator chain
    // - s.BridgeNode: DA bridge node connected to validator
    
    // Create a test wallet
    wallet := s.CreateTestWallet("test-wallet", 10000000)
    
    // Create test blob data
    blob, namespace := s.CreateRandomBlob([]byte("test data"))
    
    // Verify network is healthy
    height, err := s.Chain.Height(s.ctx)
    s.Require().NoError(err)
    s.Assert().Greater(height, int64(0))
}

func TestMyTestSuite(t *testing.T) {
    suite.Run(t, new(MyTestSuite))
}
```

### Network Testing

```go
func (s *MyTestSuite) TestNetworkConnectivity() {
    // Test P2P connectivity of the bridge node
    p2pInfo, err := s.BridgeNode.GetP2PInfo(s.ctx)
    s.Require().NoError(err)
    s.Assert().NotEmpty(p2pInfo.PeerID)
    
    // Verify bridge node is connected to validator
    height, err := s.Chain.Height(s.ctx)
    s.Require().NoError(err)
    s.Assert().Greater(height, int64(0))
}
```

### DA Synchronization Testing

```go
func (s *MyTestSuite) TestDASync() {
    // Submit blob transaction (implementation depends on your test scenario)
    // ... blob submission logic ...
    
    // Wait for DA sync with timeout
    err := s.WaitForDASync(s.BridgeNode, targetHeight, namespace, 30*time.Second)
    s.Require().NoError(err, "DA sync should complete successfully")
}
```

## API Reference

### CelestiaTestSuite

The main test suite that provides a complete Celestia network environment.

#### Network Components

- `Chain types.Chain` - The Celestia validator chain
- `BridgeNode types.DANode` - DA bridge node connected to the validator

#### Key Methods

**Network Management:**
- `SetupSuite()` - Automatically called to initialize the network
- `TearDownSuite()` - Automatically called to clean up resources

**Testing Utilities:**
- `CreateTestWallet(name string, amount int64) types.Wallet` - Creates and funds a test wallet
- `CreateRandomBlob(data []byte) (*share.Blob, share.Namespace)` - Creates a blob with random namespace
- `WaitForDASync(daNode types.DANode, targetHeight uint64, namespace share.Namespace, timeout time.Duration) error` - Waits for DA synchronization
- `GetGenesisBlockHash() (string, error)` - Retrieves the genesis block hash

**Configuration:**
- `AppOverrides() toml.Toml` - Returns app.toml configuration overrides
- `ConfigOverrides() toml.Toml` - Returns config.toml configuration overrides

### Configuration Constants

```go
const (
    DefaultCelestiaAppImage    = "ghcr.io/celestiaorg/celestia-app"
    DefaultCelestiaAppVersion  = "v4.0.0-rc6"
    DefaultCelestiaNodeImage   = "ghcr.io/celestiaorg/celestia-node"
    DefaultCelestiaNodeVersion = "v0.23.0-mocha"
    DefaultChainID            = "test"
    DefaultDenom              = "utia"
)
```

## Advanced Usage

### Custom Configuration

You can override the default configuration by embedding `CelestiaTestSuite` and overriding its methods:

```go
type CustomTestSuite struct {
    e2e.CelestiaTestSuite
}

// Override to use different Docker images
func (s *CustomTestSuite) AppOverrides() toml.Toml {
    overrides := s.CelestiaTestSuite.AppOverrides()
    // Add custom overrides
    overrides["custom-setting"] = "custom-value"
    return overrides
}
```

### Error Handling

The framework includes built-in handling for common DA network errors:

- Connection refused errors during node startup
- Blob not found errors during sync
- Context deadline exceeded errors
- Network connectivity issues

These are automatically handled in `WaitForDASync()` and other network operations.

### Resource Management

All Docker containers, networks, and volumes are automatically cleaned up when tests complete. The framework uses Docker labels for proper resource tracking and cleanup.

## Network Topology

The framework creates the following network topology:

```
┌─────────────────┐
│   Validator     │
│   (Chain)       │
└─────────────────┘
         │
         │ Core connection
         ▼
┌─────────────────┐
│   Bridge Node   │
│   (DA)          │
└─────────────────┘
```

## Integration with celestia-node

This framework is based on patterns extracted from `celestia-node`'s e2e tests, providing similar functionality in a reusable package:

- Automatic network setup with validator and bridge node
- Standard configuration patterns
- P2P connectivity management
- DA synchronization testing utilities
- Resource cleanup and error handling

## Requirements

- Docker daemon running
- Go 1.21+
- Sufficient Docker resources for running Celestia validator and bridge node

## Troubleshooting

### Common Issues

**"Failed to start container":**
- Ensure Docker daemon is running
- Check available system resources
- Verify Docker images are accessible

**"DA sync timeout":**
- Increase timeout duration in `WaitForDASync()`
- Check that blob was actually submitted to the chain
- Verify network connectivity between nodes

**"Config is sealed" errors:**
- The framework automatically handles SDK configuration conflicts
- Multiple test suites can run safely in sequence

### Debugging

Enable verbose logging to see detailed network setup information:

```bash
go test -v ./your/test/package
```

The framework provides detailed logging for:
- Node startup and connectivity
- P2P address resolution
- DA synchronization progress
- Error conditions and retries

## Contributing

When adding new functionality to this framework:

1. Maintain backward compatibility
2. Add comprehensive error handling
3. Include example usage in tests
4. Update this documentation
5. Ensure proper resource cleanup

## Related

- [Celestia App](https://github.com/celestiaorg/celestia-app)
- [Celestia Node](https://github.com/celestiaorg/celestia-node)
- [Tastora Framework](https://github.com/celestiaorg/tastora)
