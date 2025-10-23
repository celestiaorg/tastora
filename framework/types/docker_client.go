package types

import (
	"github.com/moby/moby/client"
)

// TastoraDockerClient extends the Docker client.CommonAPIClient interface
// with a CleanupLabel method for resource tagging and cleanup.
type TastoraDockerClient interface {
	client.CommonAPIClient
	CleanupLabel() string
}
