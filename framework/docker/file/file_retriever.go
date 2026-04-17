package file

import (
	"archive/tar"
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/types"
	"io"
	"path"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/consts"
	internaldocker "github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/testutil/random"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

// Retriever allows retrieving a single file from a Docker volume.
type Retriever struct {
	log      *zap.Logger
	cli      types.TastoraDockerClient
	testName string
}

// NewRetriever returns a new Retriever.
func NewRetriever(log *zap.Logger, cli types.TastoraDockerClient, testName string) *Retriever {
	return &Retriever{log: log, cli: cli, testName: testName}
}

// SingleFileContent returns the content of the file named at relPath,
// inside the volume specified by volumeName.
func (r *Retriever) SingleFileContent(ctx context.Context, volumeName, relPath string) ([]byte, error) {
	const mountPath = "/mnt/dockervolume"

	if err := internaldocker.EnsureBusybox(ctx, r.cli); err != nil {
		return nil, err
	}

	containerName := fmt.Sprintf("%s-getfile-%d-%s", consts.CelestiaDockerPrefix, time.Now().UnixNano(), random.LowerCaseLetterString(5))

	cc, err := r.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Name: containerName,
		Config: &container.Config{
			Image: internaldocker.BusyboxRef,
			// Use root user to avoid permission issues when reading files from the volume.
			User:   consts.UserRootString,
			Labels: map[string]string{consts.CleanupLabel: r.cli.CleanupLabel()},
		},
		HostConfig: &container.HostConfig{
			Binds: []string{volumeName + ":" + mountPath},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating container: %w", err)
	}

	defer func() {
		if _, err := r.cli.ContainerRemove(ctx, cc.ID, client.ContainerRemoveOptions{
			Force: true,
		}); err != nil {
			r.log.Warn("Failed to remove file content container", zap.String("container_id", cc.ID), zap.Error(err))
		}
	}()

	copyResult, err := r.cli.CopyFromContainer(ctx, cc.ID, client.CopyFromContainerOptions{
		SourcePath: path.Join(mountPath, relPath),
	})
	if err != nil {
		return nil, fmt.Errorf("copying from container: %w", err)
	}
	rc := copyResult.Content
	defer func() {
		_ = rc.Close()
	}()

	wantPath := path.Base(relPath)
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar from container: %w", err)
		}
		if hdr.Name != wantPath {
			r.log.Debug("Unexpected path", zap.String("want", relPath), zap.String("got", hdr.Name))
			continue
		}

		return io.ReadAll(tr)
	}

	return nil, fmt.Errorf("path %q not found in tar from container", relPath)
}
