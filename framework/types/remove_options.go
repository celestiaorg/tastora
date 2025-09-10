package types

import "github.com/docker/docker/api/types/container"

// Container removal functional options

// RemoveOption is a functional option for configuring container removal.
type RemoveOption func(*container.RemoveOptions)

// WithPreserveVolumes configures removal to preserve volumes (useful for upgrades).
func WithPreserveVolumes() RemoveOption {
	return func(opts *container.RemoveOptions) {
		opts.RemoveVolumes = false
	}
}

// WithForce configures removal to force kill running containers.
func WithForce(force bool) RemoveOption {
	return func(opts *container.RemoveOptions) {
		opts.Force = force
	}
}
