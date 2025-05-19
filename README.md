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

Tastora provides a framework for testing and developing Celestia blockchain applications. Here's a basic example of how to use it:

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
