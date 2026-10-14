package internal

import (
	"context"
	"github.com/celestiaorg/tastora/framework/types"
	"time"

	"github.com/moby/moby/client"
)

// StartContainer attempts to start the container with the given ID.
func StartContainer(ctx context.Context, cli types.TastoraDockerClient, id string) error {
	// add a deadline for the request if the calling context does not provide one
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	_, err := cli.ContainerStart(ctx, id, client.ContainerStartOptions{})
	if err != nil {
		return err
	}

	return nil
}
