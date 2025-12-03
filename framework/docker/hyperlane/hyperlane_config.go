package hyperlane

import (
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
)

// Config holds configuration for the Hyperlane deployment coordinator
type Config struct {
	Logger          *zap.Logger
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string

	// HyperlaneImage for hyperlane CLI (contains hyperlane binary)
	HyperlaneImage container.Image
}

// DefaultHyperlaneImage returns the default hyperlane CLI image
func DefaultHyperlaneImage() container.Image {
	return container.Image{Repository: "gcr.io/abacus-labs-dev/hyperlane-agent", Version: "latest"}
}
