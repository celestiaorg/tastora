package file_test

import (
	"context"
	"github.com/celestiaorg/tastora/framework/docker"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/file"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"testing"
)

func TestFileWriter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	t.Parallel()

	cli, network := docker.Setup(t)

	ctx := context.Background()
	v, err := cli.VolumeCreate(ctx, client.VolumeCreateOptions{
		Labels: map[string]string{consts.CleanupLabel: t.Name()},
	})
	require.NoError(t, err)

	img := container.NewJob(
		zaptest.NewLogger(t),
		cli,
		network,
		t.Name(),
		"busybox", "stable",
	)

	fw := file.NewWriter(zaptest.NewLogger(t), cli, t.Name())

	t.Run("top-level file", func(t *testing.T) {
		require.NoError(t, fw.WriteFile(context.Background(), v.Volume.Name, "hello.txt", []byte("hello world")))
		res := img.Run(
			ctx,
			[]string{"sh", "-c", "cat /mnt/test/hello.txt"},
			container.Options{
				Binds: []string{v.Volume.Name + ":/mnt/test"},
				User:  consts.UserRootString,
			},
		)
		require.NoError(t, res.Err)

		require.Equal(t, "hello world", string(res.Stdout))
	})

	t.Run("create nested file", func(t *testing.T) {
		require.NoError(t, fw.WriteFile(context.Background(), v.Volume.Name, "a/b/c/d.txt", []byte(":D")))
		res := img.Run(
			ctx,
			[]string{"sh", "-c", "cat /mnt/test/a/b/c/d.txt"},
			container.Options{
				Binds: []string{v.Volume.Name + ":/mnt/test"},
				User:  consts.UserRootString,
			},
		)
		require.NoError(t, res.Err)

		require.Equal(t, ":D", string(res.Stdout))
	})
}
