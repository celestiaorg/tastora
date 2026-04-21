package types

import (
	"github.com/moby/moby/client"
)

// TastoraDockerClient extends the Docker client.APIClient interface
// with a CleanupLabel method for resource tagging and cleanup.
type TastoraDockerClient interface {
	client.APIClient
	CleanupLabel() string
}
