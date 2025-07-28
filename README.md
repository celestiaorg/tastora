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

### Configurable Ports Setup

For complex setups or when running multiple networks, you can configure custom ports:

```go
// Use predefined default ports
daNetwork, err := docker.NewDANetwork(
    ctx,
    "test-network",
    docker.WithDefaultPorts(), // Uses ports 26668, 2131, 26667, 9091
)

// Or configure specific ports
daNetwork, err := docker.NewDANetwork(
    ctx,
    "test-network",
    docker.WithDANodeCoreConnection("192.168.1.100", 26657, 9090),
    docker.WithDANodePorts(26658, 2121),
)

// Configure different node types with specific ports
daNetwork, err := docker.NewDANetwork(
    ctx,
    "test-network",
    docker.WithNodePorts(types.BridgeNode, 0, 26658, 2121),
    docker.WithNodePorts(types.LightNode, 0, 26659, 2122),
)
```

## Port Configuration

Tastora supports configurable internal ports for DA nodes, solving connectivity issues in complex deployments where celestia-app runs on different servers or uses non-default ports.

### Available Configuration Options

- `WithDefaultPorts()` - Quick setup with predefined ports (26668, 2131, 26667, 9091)
- `WithDANodePorts(rpc, p2p)` - Configure DA node internal ports
- `WithDANodeCoreConnection(rpc, grpc)` - Configure connection to celestia-app
- `WithNodePorts(nodeType, nodeIndex, rpc, p2p)` - Configure ports for specific node type and index

This addresses the issue where celestia bridge nodes fail to start when trying to connect to hardcoded `localhost:26657` in multi-server setups.

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
