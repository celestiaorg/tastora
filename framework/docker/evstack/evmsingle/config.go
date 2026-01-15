package evmsingle

import (
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
)

// Config holds chain-level configuration for ev-node-evm-single nodes
type Config struct {
	Logger          *zap.Logger
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string
	// Image is the default image for all nodes
	Image container.Image
	// Bin is the executable name (default: evm-single)
	Bin string
	// Env are default environment variables applied to all nodes
	Env []string
	// AdditionalStartArgs are default start arguments applied to all nodes
	AdditionalStartArgs []string
	// AdditionalInitArgs are appended to the init command for all nodes
	AdditionalInitArgs []string
}

// DefaultImage returns the default container image for ev-node-evm-single.
func DefaultImage() container.Image {
	return container.Image{Repository: "ghcr.io/evstack/ev-node-evm-single", Version: "main"}
}

// DefaultBinary returns the default binary name for ev-node-evm-single.
func DefaultBinary() string {
	return "evm-single"
}
