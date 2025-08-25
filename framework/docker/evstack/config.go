package evstack

import (
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

// Config contains all the configuration for docker operations
type Config struct {
	// Logger is the logger instance used for all operations
	Logger *zap.Logger
	// DockerClient is the docker client instance  
	DockerClient *client.Client
	// DockerNetworkID is the ID of the docker network to use
	DockerNetworkID string
	// ChainID, e.g. test-evstack
	ChainID string
	// Environment variables for chain nodes
	Env []string
	// Binary to execute for the evstack chain.
	Bin string
	// AggregatorPassphrase is the passphrase used when a node is an aggregator.
	AggregatorPassphrase string
	// Image specifies the Docker image used for the evstack nodes.
	Image container.Image
}