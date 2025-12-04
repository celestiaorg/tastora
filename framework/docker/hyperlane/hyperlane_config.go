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
    // Multi-arch image that includes the hyperlane CLI and works on arm64.
    // Most Node.js-based images run as uid/gid 1000 by default; set ownership accordingly
    // so the container process can write to the mounted /workspace volume.
    return container.Image{Repository: "ghcr.io/celestiaorg/hyperlane-init", Version: "latest", UIDGID: "1000:1000"}
}
