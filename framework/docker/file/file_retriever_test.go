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

func TestFileRetriever(t *testing.T) {
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

	res := img.Run(
		ctx,
		[]string{"sh", "-c", "chmod 0700 /mnt/test && printf 'hello world' > /mnt/test/hello.txt"},
		container.Options{
			Binds: []string{v.Volume.Name + ":/mnt/test"},
			User:  consts.UserRootString,
		},
	)
	require.NoError(t, res.Err)
	res = img.Run(
		ctx,
		[]string{"sh", "-c", "mkdir -p /mnt/test/foo/bar/ && printf 'test' > /mnt/test/foo/bar/baz.txt"},
		container.Options{
			Binds: []string{v.Volume.Name + ":/mnt/test"},
			User:  consts.UserRootString,
		},
	)
	require.NoError(t, res.Err)

	fr := file.NewRetriever(zaptest.NewLogger(t), cli, t.Name())

	t.Run("top-level file", func(t *testing.T) {
		b, err := fr.SingleFileContent(ctx, v.Volume.Name, "hello.txt")
		require.NoError(t, err)
		require.Equal(t, "hello world", string(b))
	})

	t.Run("nested file", func(t *testing.T) {
		b, err := fr.SingleFileContent(ctx, v.Volume.Name, "foo/bar/baz.txt")
		require.NoError(t, err)
		require.Equal(t, "test", string(b))
	})
}
