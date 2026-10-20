# Tastora

Tastora is a testing and development framework for Celestia blockchain applications. It provides Docker-based containerization for different node types (Bridge, Light, Full) and comprehensive testing utilities for blockchain development.

## Overview

Tastora simplifies the process of setting up and testing Celestia blockchain nodes by providing:

- Docker-based containerization for different node types.
- Abstractions for working with the Celestia Data Availability (DA) layer.
- Utilities for blockchain testing and development.

> The implementation of the docker backend is largely based on the [interchaintest](https://github.com/strangelove-ventures/interchaintest) framework. It is has been modified and tailored to specifically work within the Celestia ecosystem.

## Installation

### Prerequisites

- Go 1.23.6 or higher
- Docker
- golangci-lint (for linting)

### Installing

```bash
# Clone the repository
git clone https://github.com/celestiaorg/tastora.git
cd tastora

# Install dependencies
go mod download
```

## Usage

Tastora provides a framework for testing and developing Celestia blockchain applications. Here are some basic examples:

### Basic DA Network Setup

```go
package main

import (
    "context"
    "testing"

    "github.com/celestiaorg/tastora/framework/docker"
)

func TestCelestiaNodes(t *testing.T) {
    ctx := context.Background()

    // Create a basic DA network with default ports
    daNetwork, err := docker.NewDANetwork(ctx, "test-network")
    if err != nil {
        t.Fatal(err)
    }
    defer daNetwork.Cleanup(ctx)

    // Start the network
    err = daNetwork.Start(ctx)
    if err != nil {
        t.Fatal(err)
    }
}
```

### Network Information

All nodes provide standardized network information through `GetNetworkInfo()`:

```go
// Get network info from any node
networkInfo, err := node.GetNetworkInfo(ctx)
if err != nil {
    t.Fatal(err)
}

// Access network details
internalRPCAddr := networkInfo.Internal.RPCAddress()  // "hostname:26657"
externalRPCAddr := networkInfo.External.RPCAddress()  // "0.0.0.0:32145"
```

DA nodes support custom port configuration:

```go
// Configure DA node with custom ports
customPorts := types.Ports{
    RPC:      "27000",  // DA node RPC port
    P2P:      "3000",   // DA node P2P port  
    CoreRPC:  "26657",  // celestia-app RPC port
    CoreGRPC: "9090",   // celestia-app GRPC port
}

bridgeNodeConfig := da.NewNodeBuilder().
    WithNodeType(types.BridgeNode).
    WithInternalPorts(customPorts).
    Build()
```

## Fine-Grained Wallet Control

Tastora supports creating wallets on specific chain nodes, providing flexibility for testing scenarios that require granular control over wallet placement and key management.

### Node-Specific Wallet Creation

Create wallets on specific validators or full nodes:

```go
// Create wallet on a specific validator
wallet1, err := chain.Validators[0].CreateWallet(ctx, "test-key-1", "celestia")
if err != nil {
    t.Fatal(err)
}

// Create wallet on a different validator
wallet2, err := chain.Validators[1].CreateWallet(ctx, "test-key-2", "celestia") 
if err != nil {
    t.Fatal(err)
}

// Create wallet on a full node
wallet3, err := chain.FullNodes[0].CreateWallet(ctx, "test-key-3", "celestia")
if err != nil {
    t.Fatal(err)
}
```

### Faucet Wallet Access

The faucet wallet is accessible on all validator nodes with automatically synchronized keys:

```go
// Access faucet wallet from any validator
faucetWallet := chain.Validators[0].GetFaucetWallet()

// Or from another validator - same wallet, synchronized keys
faucetWallet2 := chain.Validators[1].GetFaucetWallet()

// Chain-level access works (delegates to Validator[0])
chainFaucetWallet := chain.GetFaucetWallet()
```

### Backward Compatibility

Existing `chain.CreateWallet()` calls continue to work unchanged:

```go
// This works exactly as before
wallet, err := chain.CreateWallet(ctx, "my-wallet")
```

## Project Structure

- `framework/` - Core framework code
  - `docker/` - Docker-based implementations of nodes and chains
  - `testutil/` - Testing utilities
  - `types/` - Core type definitions

## Testing

To run tests:

```bash
make test
```

## Linting

To run linters:

```bash
make lint
```

To automatically fix linting issues:

```bash
make lint-fix
```

## Related Projects

- [Celestia App](https://github.com/celestiaorg/celestia-app) - The Celestia Consensus Node.
- [Celestia Node](https://github.com/celestiaorg/celestia-node) - The Celestia DA Nodes.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

PR titles must follow [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/) using a standard type such as `fix:`, `feat:`, `build:`, `chore:`, `ci:`, `docs:`, `refactor:`, `perf:`, or `test:` (dependency bumps use `chore(deps):`, not `deps:`). Include an `!` (e.g. `feat!:`) for breaking changes.

The subject must start with `[high]`, `[medium]`, or `[low]` to indicate the review thoroughness needed. For example: `feat!: [high] rename Chain interface methods` or `docs: [low] clarify setup instructions`. Include the same tag in the PR description and explain why you chose it.

- `[high]`: Deep review for breaking API changes, public interfaces in `framework/types` that consumers import, security, or other changes with significant risk.
- `[medium]`: Standard review for code changes with limited impact and well-understood behavior.
- `[low]`: Focused review for straightforward docs, tests, or tooling changes with low risk.

Choose the higher level when unsure. A small diff can still need deep review. CI checks the title format; reviewers should confirm the chosen level. Dependabot titles default to `[medium]`.
