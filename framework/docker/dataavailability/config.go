package dataavailability

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
	// ChainID, e.g. test-chain
	ChainID string
	// Environment variables for nodes
	Env []string
	// Binary to execute for the node (e.g., "celestia")
	Bin string
	// Image specifies the Docker image used for the nodes.
	Image container.Image
}
