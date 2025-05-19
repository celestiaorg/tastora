package file_test

import (
	"context"
	"github.com/celestiaorg/tastora/framework/docker"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/docker/file"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"testing"
)

func TestFileWriter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	t.Parallel()

	cli, network := docker.DockerSetup(t)

	ctx := context.Background()
	v, err := cli.VolumeCreate(ctx, volumetypes.CreateOptions{
		Labels: map[string]string{consts.CleanupLabel: t.Name()},
	})
	require.NoError(t, err)

	img := docker.NewImage(
		zaptest.NewLogger(t),
		cli,
		network,
		t.Name(),
		"busybox", "stable",
	)

	fw := file.NewWriter(zaptest.NewLogger(t), cli, t.Name())

	t.Run("top-level file", func(t *testing.T) {
		require.NoError(t, fw.WriteFile(context.Background(), v.Name, "hello.txt", []byte("hello world")))
		res := img.Run(
			ctx,
			[]string{"sh", "-c", "cat /mnt/test/hello.txt"},
			docker.ContainerOptions{
				Binds: []string{v.Name + ":/mnt/test"},
				User:  consts.UserRootString,
			},
		)
		require.NoError(t, res.Err)

		require.Equal(t, "hello world", string(res.Stdout))
	})

	t.Run("create nested file", func(t *testing.T) {
		require.NoError(t, fw.WriteFile(context.Background(), v.Name, "a/b/c/d.txt", []byte(":D")))
		res := img.Run(
			ctx,
			[]string{"sh", "-c", "cat /mnt/test/a/b/c/d.txt"},
			docker.ContainerOptions{
				Binds: []string{v.Name + ":/mnt/test"},
				User:  consts.UserRootString,
			},
		)
		require.NoError(t, err)

		require.Equal(t, ":D", string(res.Stdout))
	})
}
