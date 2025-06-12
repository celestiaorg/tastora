# Test Directory - Updated to use CelestiaTestSuite

This directory contains Celestia end-to-end tests that have been refactored to use the reusable `framework/e2e/CelestiaTestSuite` for streamlined testing.

## Test Files

### `validator_tx_test.go`
**Purpose**: Tests basic transaction functionality on a single Celestia validator

**Refactored Changes**:
- Now extends `e2e.CelestiaTestSuite` instead of implementing custom setup
- Uses suite utilities: `CreateTestWallet()` for wallet creation and funding
- Simplified from ~200 lines to ~130 lines by removing boilerplate setup code
- Automatic network setup and cleanup handled by the suite

**Test Cases**:
- `TestSubmitTransactionAndVerifyInBlock`: Submit bank transaction and verify inclusion
- `TestValidatorBasicFunctionality`: Verify basic validator operation and block production

### `validator_with_da_bridge_test.go`
**Purpose**: Tests blob submission and DA bridge node synchronization

**Refactored Changes**:
- Now extends `e2e.CelestiaTestSuite` instead of custom `ValidatorWithDABridgeTestSuite`
- Uses suite utilities: 
  - `CreateTestWallet()` for funding blob submission wallet
  - `CreateRandomBlob()` for blob creation
  - `WaitForDASync()` for DA synchronization testing
  - `CreateAndStartFullNode()` and `CreateAndStartLightNode()` for multi-node testing
- Simplified from ~570 lines to ~300 lines by removing custom setup code
- Automatic DA bridge setup handled by the suite

**Test Cases**:
- `TestSubmitBlobAndVerifyDASync`: Submit blob and verify DA bridge synchronization
- `TestDABridgeBasicFunctionality`: Verify DA bridge node connectivity and P2P info
- `TestMultiNodeDANetwork`: Test multi-node DA network topology (new test)
- `TestDABridgeNetworkTopology`: Test network topology and genesis hash retrieval (new test)

## Benefits of Refactoring

### 1. **Reduced Boilerplate**
- **Before**: Each test implemented custom Docker setup, provider creation, network configuration
- **After**: Single line inheritance (`e2e.CelestiaTestSuite`) provides complete setup

### 2. **Improved Reliability**
- **Before**: Manual SDK configuration with potential conflicts between test suites
- **After**: Automatic SDK configuration handling prevents "Config is sealed" errors

### 3. **Better Reusability**
- **Before**: Setup code duplicated across test files
- **After**: Common patterns extracted to reusable utilities

### 4. **Enhanced Testing Capabilities**
- **Before**: Limited to basic validator + bridge setup
- **After**: Easy multi-node network creation with `CreateAndStartFullNode()` and `CreateAndStartLightNode()`

### 5. **Cleaner Error Handling**
- **Before**: Manual error handling for DA sync and network issues
- **After**: Built-in handling for common DA network sync errors in `WaitForDASync()`

## Code Comparison

### Before (Custom Setup)
```go
type ValidatorWithDABridgeTestSuite struct {
    suite.Suite
    ctx          context.Context
    dockerClient *client.Client
    networkID    string
    logger       *zap.Logger
    encConfig    testutil.TestEncodingConfig
    provider     *docker.Provider
    chain        *docker.Chain
    daBridge     types.DANode
}

func (s *ValidatorWithDABridgeTestSuite) SetupSuite() {
    // 50+ lines of manual setup code...
    s.dockerClient, s.networkID = docker.DockerSetup(s.T())
    s.logger = zaptest.NewLogger(s.T())
    // SDK configuration...
    // Provider creation...
    // Chain startup...
    // DA bridge setup...
}
```

### After (Using CelestiaTestSuite)
```go
type ValidatorWithDABridgeTestSuite struct {
    e2e.CelestiaTestSuite  // Single line inheritance
}

func (s *ValidatorWithDABridgeTestSuite) TestMyScenario() {
    // Network already running:
    // - s.Chain: Celestia validator
    // - s.BridgeNode: DA bridge node
    
    wallet := s.CreateTestWallet("test", 10000000)
    blob, namespace := s.CreateRandomBlob([]byte("test data"))
    
    // Optional: Create additional nodes
    s.FullNode = s.CreateAndStartFullNode()
    s.LightNode = s.CreateAndStartLightNode()
}
```

## Running Tests

### Single Test
```bash
go test ./test/ -v -run TestValidatorTxSuite/TestValidatorBasicFunctionality
```

### Full Suite
```bash
go test ./test/ -v
```

### With Timeout (Recommended for CI)
```bash
go test ./test/ -v -timeout 10m
```

## Test Output Example

```
=== RUN   TestValidatorTxSuite
    logger.go:146: 2025-06-10T13:01:08.285+0200	INFO	Celestia chain started successfully	{"height": 9}
    logger.go:146: 2025-06-10T13:01:13.358+0200	INFO	DA bridge node started successfully
=== RUN   TestValidatorTxSuite/TestValidatorBasicFunctionality
    validator_tx_test.go:120: Testing basic validator functionality  
    validator_tx_test.go:148: Basic validator functionality verified - Height: 15, NodeID: 73846..., Network: test
--- PASS: TestValidatorTxSuite (56.84s)
    --- PASS: TestValidatorTxSuite/TestValidatorBasicFunctionality (1.17s)
PASS
```

## Migration Summary

| Aspect | Before | After | Improvement |
|--------|--------|-------|-------------|
| **Lines of Code** | ~770 total | ~430 total | **44% reduction** |
| **Setup Complexity** | Manual 50+ lines | Single inheritance | **Automatic** |
| **Error Handling** | Manual DA sync handling | Built-in error handling | **Robust** |
| **Multi-node Support** | Manual implementation | `CreateAndStartFullNode()` | **Simple** |
| **SDK Config Conflicts** | Manual handling | Automatic prevention | **Reliable** |
| **Reusability** | Low (duplicated code) | High (shared utilities) | **Reusable** |

The refactored tests maintain all original functionality while providing a much cleaner, more maintainable codebase that leverages the new `framework/e2e` reusable components.
