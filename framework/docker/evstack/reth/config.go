package reth

import (
    "github.com/celestiaorg/tastora/framework/docker/container"
    dockerclient "github.com/moby/moby/client"
    "go.uber.org/zap"
)

// Config holds chain-level configuration for Reth
type Config struct {
    Logger          *zap.Logger
    DockerClient    *dockerclient.Client
    DockerNetworkID string

    // Image is the default image for all nodes
    Image container.Image
    // Bin is the executable name (default: ev-reth)
    Bin string

    // Env are default environment variables applied to all nodes
    Env []string
    // AdditionalStartArgs are default start arguments applied to all nodes
    AdditionalStartArgs []string

    // GenesisFileBz, if provided, will be written to each node before start at <home>/chain/genesis.json
    // If omitted, Start will return an error until automatic genesis initialization is implemented.
    GenesisFileBz []byte
}

// DefaultImage returns the default container image for Reth nodes.
func DefaultImage() container.Image {
    return container.Image{Repository: "ghcr.io/evstack/ev-reth", Version: "latest"}
}

