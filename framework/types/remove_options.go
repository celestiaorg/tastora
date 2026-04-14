package types

import "github.com/moby/moby/client"

// Container removal functional options

// RemoveOption is a functional option for configuring container removal.
type RemoveOption func(*client.ContainerRemoveOptions)

// WithPreserveVolumes configures removal to preserve volumes (useful for upgrades).
func WithPreserveVolumes() RemoveOption {
	return func(opts *client.ContainerRemoveOptions) {
		opts.RemoveVolumes = false
	}
}

// WithForce configures removal to force kill running containers.
func WithForce(force bool) RemoveOption {
	return func(opts *client.ContainerRemoveOptions) {
		opts.Force = force
	}
}
