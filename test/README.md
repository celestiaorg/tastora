# Celestia Validator Transaction Tests

This directory contains simple tests for basic Celestia validator functionality using the Tastora framework.

## Overview

The `validator_tx_test.go` file implements a straightforward test that:
1. Starts a single Celestia validator
2. Submits transactions to it
3. Verifies transactions are included in blocks

## Test Architecture

The test spins up a single Celestia validator node using Docker and performs basic transaction operations.

### Components
- **Single Validator Node** - Celestia application chain validator
- **No DA Nodes** - Simplified setup focusing on core functionality

## Test Cases

### TestSubmitTransactionAndVerifyInBlock
This test performs a complete transaction lifecycle:

1. **Setup**: Creates sender and receiver wallets
2. **Transaction Creation**: Creates a simple bank send transaction (1 TIA)
3. **Submission**: Broadcasts the transaction to the validator
4. **Block Verification**: Waits for blocks and verifies the transaction is included
5. **RPC Verification**: Uses RPC client to query the specific block and confirm transaction presence

### TestValidatorBasicFunctionality
This test verifies basic validator operations:

1. **Block Production**: Confirms the validator is producing blocks
2. **Height Progression**: Verifies block height increases over time
3. **Node Configuration**: Confirms exactly one validator node is running
4. **RPC Connectivity**: Tests RPC client connection and network status

## Configuration

### Celestia Version
- **Celestia App**: v2.3.1
- **Compatible and stable version for basic operations**

### Network Settings
- **Chain ID**: "test"
- **Denomination**: "utia"
- **Gas Prices**: "0.025utia"
- **Bech32 Prefix**: "celestia"

## Usage

Run the tests with:

```bash
# Run all tests
go test -v ./test/

# Run specific test
go test -v ./test/ -run TestSubmitTransactionAndVerifyInBlock

# Run with verbose logging
go test -v ./test/ -run TestValidatorBasicFunctionality
```

Note: Tests require Docker and may take 1-2 minutes to complete as they need to pull the Celestia image and start the validator.

## Test Flow Details

### Transaction Verification Process

1. **Initial State**: Record starting block height
2. **Wallet Creation**: Create funded sender and receiver wallets
3. **Transaction Construction**: Build `MsgSend` with 1 TIA transfer
4. **Broadcasting**: Submit transaction via `BroadcastMessages()`
5. **Confirmation**: Verify transaction response indicates success (`Code: 0`)
6. **Block Inclusion**: Wait for additional blocks to ensure finalization
7. **RPC Query**: Query the specific block containing the transaction
8. **Hash Verification**: Confirm transaction hash matches in the block
9. **Final Verification**: Ensure block height and transaction data are consistent

### Key Verification Points

- ✅ Transaction successfully submitted
- ✅ Transaction included in a block at expected height
- ✅ Block contains the correct transaction hash
- ✅ Validator continues producing blocks
- ✅ RPC connectivity and queries work correctly

## Dependencies

- Tastora framework Docker components
- Cosmos SDK bank module for send transactions
- CometBFT RPC client for block queries
- Docker for container orchestration

## Differences from Complex E2E Tests

This simplified test:
- **Single Node**: Only runs one validator (no DA nodes)
- **Basic Transactions**: Uses simple bank send transactions
- **Core Verification**: Focuses on transaction inclusion verification
- **Faster Execution**: Minimal setup reduces test time
- **Clear Learning Path**: Easier to understand and debug

This test provides a solid foundation for understanding Celestia validator operations and transaction processing before moving to more complex data availability scenarios.
