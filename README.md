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
// Use predefined non-conflicting ports
daNetwork, err := docker.NewDANetwork(
    ctx,
    "test-network",
    docker.WithNonConflictingPorts(), // Uses ports 26668, 2131, 26667, 9091
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
    docker.WithBridgeNodePorts(26658, 2121, 26657, 9090),
    docker.WithLightNodePorts(26659, 2122, 26657, 9090),
)
```

## Port Configuration

Tastora supports configurable internal ports for DA nodes, solving connectivity issues in complex deployments where celestia-app runs on different servers or uses non-default ports.

### Available Configuration Options

- `WithNonConflictingPorts()` - Quick setup with predefined ports (26668, 2131, 26667, 9091)
- `WithDANodePorts(rpc, p2p)` - Configure DA node internal ports
- `WithDANodeCoreConnection(host, rpc, grpc)` - Configure connection to celestia-app
- `WithBridgeNodePorts()`, `WithLightNodePorts()`, `WithFullNodePorts()` - Per-node type configuration

This addresses the issue where celestia bridge nodes fail to start when trying to connect to hardcoded `localhost:26657` in multi-server setups.

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
