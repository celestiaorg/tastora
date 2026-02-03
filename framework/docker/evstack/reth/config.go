package reth

import (
	"fmt"
	"regexp"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
)

var chainNameRegex = regexp.MustCompile(`^[A-Za-z0-9]*$`)

// Config holds node-level configuration for Reth
type Config struct {
	Logger          *zap.Logger
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string

	// Image is the default image for all nodes
	Image container.Image
	// Bin is the executable name (default: ev-reth)
	Bin string

	// Env are default environment variables applied to all nodes
	Env []string
	// AdditionalStartArgs are default start arguments applied to all nodes
	AdditionalStartArgs []string

	// JWTSecretHex sets the node JWT secret in hex; if empty, it will be generated at start.
	JWTSecretHex string

	// GenesisFileBz, if provided, will be written to each node before start at <home>/chain/genesis.json
	// If omitted, Start will return an error until automatic genesis initialization is implemented.
	GenesisFileBz []byte

	// HyperlaneChainName overrides the chain name used in Hyperlane configs.
	HyperlaneChainName string
	// HyperlaneChainID overrides the chain ID used in Hyperlane configs (0 means derive or default).
	HyperlaneChainID uint64
	// HyperlaneDomainID overrides the domain ID used in Hyperlane configs (0 means derive or default).
	HyperlaneDomainID uint32
}

// Validate checks the config for common errors.
func (c Config) Validate() error {
	if !chainNameRegex.MatchString(c.HyperlaneChainName) {
		return fmt.Errorf("invalid hyperlane chain name %q: must be alphanumeric", c.HyperlaneChainName)
	}

	return nil
}

// DefaultImage returns the default container image for Reth nodes.
func DefaultImage() container.Image {
	return container.Image{Repository: "ghcr.io/evstack/ev-reth", Version: "latest"}
}
