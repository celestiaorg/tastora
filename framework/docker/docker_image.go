package docker

import (
	"context"
	"fmt"
	dockerimagetypes "github.com/docker/docker/api/types/image"
	"github.com/moby/moby/client"
	"io"
)

type DockerImage struct {
	Repository string
	Version    string
	UIDGID     string
}

func NewDockerImage(repository, version, uidGID string) DockerImage {
	return DockerImage{
		Repository: repository,
		Version:    version,
		UIDGID:     uidGID,
	}
}

// Ref returns the reference to use when e.g. creating a container.
func (i DockerImage) Ref() string {
	if i.Version == "" {
		return i.Repository + ":latest"
	}

	return i.Repository + ":" + i.Version
}

func (i DockerImage) PullImage(ctx context.Context, client *client.Client) error {
	ref := i.Ref()
	_, _, err := client.ImageInspectWithRaw(ctx, ref)
	if err != nil {
		rc, err := client.ImagePull(ctx, ref, dockerimagetypes.PullOptions{})
		if err != nil {
			return fmt.Errorf("pull image %s: %w", ref, err)
		}
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
	return nil
}
