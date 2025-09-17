package internal

import (
	"context"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/moby/moby/client"
)

// portAssignmentMu prevents race conditions during the critical window between
// closing temporary listeners and starting containers. Multiple containers starting
// concurrently could otherwise claim the same ports.
var portAssignmentMu sync.Mutex

// StartContainer attempts to start the container with the given ID.
func StartContainer(ctx context.Context, cli *client.Client, id string) error {
	// add a deadline for the request if the calling context does not provide one
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	err := cli.ContainerStart(ctx, id, container.StartOptions{})
	if err != nil {
		return err
	}

	return nil
}

// LockPortAssignment locks the port assignment mutex to prevent race conditions
// during the critical window between closing temporary listeners and starting containers.
func LockPortAssignment() {
	portAssignmentMu.Lock()
}

// UnlockPortAssignment unlocks the port assignment mutex.
func UnlockPortAssignment() {
	portAssignmentMu.Unlock()
}
