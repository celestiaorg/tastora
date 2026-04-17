package volume

import (
	"context"
	"fmt"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/consts"
	dockerinternal "github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/testutil/random"
	"github.com/celestiaorg/tastora/framework/types"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

// OwnerOptions contain the configuration for the SetOwner function.
type OwnerOptions struct {
	Log        *zap.Logger
	Client     types.TastoraDockerClient
	VolumeName string
	ImageRef   string
	TestName   string
	UidGid     string //nolint: stylecheck
}

// SetOwner configures the owner of a volume to match the default user in the supplied image reference.
func SetOwner(ctx context.Context, opts OwnerOptions) error {
	owner := opts.UidGid
	if owner == "" {
		owner = consts.UserRootString
	}

	// Start a one-off container to chmod and chown the volume.

	containerName := fmt.Sprintf("%s-volumeowner-%d-%s", consts.CelestiaDockerPrefix, time.Now().UnixNano(), random.LowerCaseLetterString(5))

	if err := dockerinternal.EnsureBusybox(ctx, opts.Client); err != nil {
		return err
	}

	const mountPath = "/mnt/dockervolume"

	cc, err := opts.Client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: containerName,
		Config: &container.Config{
			Image:      dockerinternal.BusyboxRef, // Using busybox image which has chown and chmod.
			Entrypoint: []string{"sh", "-c"},
			Cmd: []string{
				`chown "$2" "$1" && chmod 0700 "$1"`,
				"_", // Meaningless arg0 for sh -c with positional args.
				mountPath,
				owner,
			},
			// Root user so we have permissions to set ownership and mode.
			User:   consts.UserRootString,
			Labels: map[string]string{consts.CleanupLabel: opts.Client.CleanupLabel()},
		},
		HostConfig: &container.HostConfig{
			Binds: []string{opts.VolumeName + ":" + mountPath},
		},
	})
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	defer func() {
		// Always remove the container since we're not using AutoRemove
		if _, err := opts.Client.ContainerRemove(ctx, cc.ID, client.ContainerRemoveOptions{
			Force: true,
		}); err != nil {
			opts.Log.Warn("Failed to remove volume-owner container", zap.String("container_id", cc.ID), zap.Error(err))
		}
	}()

	if _, err := opts.Client.ContainerStart(ctx, cc.ID, client.ContainerStartOptions{}); err != nil {
		return fmt.Errorf("starting volume-owner container: %w", err)
	}

	waitResult := opts.Client.ContainerWait(ctx, cc.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNotRunning})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-waitResult.Error:
		return err
	case res := <-waitResult.Result:
		if res.Error != nil {
			return fmt.Errorf("waiting for volume-owner container: %s", res.Error.Message)
		}

		if res.StatusCode != 0 {
			return fmt.Errorf("configuring volume exited %d", res.StatusCode)
		}
	}

	return nil
}
