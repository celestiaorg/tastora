package container

import (
	"context"
	"fmt"
	dockerimagetypes "github.com/docker/docker/api/types/image"
	"github.com/moby/moby/client"
	"io"
)

type Image struct {
	Repository string
	Version    string
	UIDGID     string
}

func NewImage(repository, version, uidGID string) Image {
	return Image{
		Repository: repository,
		Version:    version,
		UIDGID:     uidGID,
	}
}

// Ref returns the reference to use when e.g. creating a container.
func (i Image) Ref() string {
	if i.Version == "" {
		return i.Repository + ":latest"
	}

	return i.Repository + ":" + i.Version
}

func (i Image) PullImage(ctx context.Context, client client.CommonAPIClient) error {
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