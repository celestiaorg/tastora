package container

// Options contains configuration for container execution
type Options struct {
	// bind mounts: https://docs.docker.com/storage/bind-mounts/
	Binds []string
	// Environment variables
	Env []string
	// If blank, defaults to the container's default user.
	User string
	// If non-zero, will limit the amount of log lines returned.
	LogTail int
}