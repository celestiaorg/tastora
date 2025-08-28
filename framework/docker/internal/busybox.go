package internal

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/docker/docker/api/types/filters"
	dockerimagetypes "github.com/docker/docker/api/types/image"
	"github.com/moby/moby/client"
)

// Allow multiple goroutines to check for busybox
// by using a protected package-level variable.
//
// A mutex allows for retries upon error, if we ever need that;
// whereas a sync.Once would not be simple to retry.
var (
	ensureBusyboxMu sync.Mutex
	hasBusybox      bool
)

const (
	BusyboxRef = "busybox:stable"

	// Retry configuration for busybox image pull
	maxRetryAttempts  = 3
	initialRetryDelay = 1 * time.Second
	maxRetryDelay     = 10 * time.Second
)

// retryWithBackoff executes the given function with exponential backoff retry logic
func retryWithBackoff(ctx context.Context, operation func() error) error {
	var lastErr error
	delay := initialRetryDelay

	for attempt := 0; attempt < maxRetryAttempts; attempt++ {
		if err := operation(); err == nil {
			return nil
		} else {
			lastErr = err
		}

		// Don't retry on the last attempt
		if attempt == maxRetryAttempts-1 {
			break
		}

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}

		// Exponential backoff with max delay cap
		delay *= 2
		if delay > maxRetryDelay {
			delay = maxRetryDelay
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", maxRetryAttempts, lastErr)
}

func EnsureBusybox(ctx context.Context, cli *client.Client) error {
	ensureBusyboxMu.Lock()
	defer ensureBusyboxMu.Unlock()

	if hasBusybox {
		return nil
	}

	images, err := cli.ImageList(ctx, dockerimagetypes.ListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", BusyboxRef)),
	})
	if err != nil {
		return fmt.Errorf("listing images to check busybox presence: %w", err)
	}

	if len(images) > 0 {
		hasBusybox = true
		return nil
	}

	// Use retry mechanism for pulling the busybox image
	err = retryWithBackoff(ctx, func() error {
		rc, err := cli.ImagePull(ctx, BusyboxRef, dockerimagetypes.PullOptions{})
		if err != nil {
			return fmt.Errorf("pulling busybox image: %w", err)
		}

		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to pull busybox image after retries: %w", err)
	}

	hasBusybox = true
	return nil
}
