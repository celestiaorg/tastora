package file_test

import (
	"context"
	"github.com/chatton/celestia-test/framework/docker"
	"github.com/chatton/celestia-test/framework/docker/consts"
	"github.com/chatton/celestia-test/framework/docker/file"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"testing"
)

func TestFileRetriever(t *testing.T) {
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

	res := img.Run(
		ctx,
		[]string{"sh", "-c", "chmod 0700 /mnt/test && printf 'hello world' > /mnt/test/hello.txt"},
		docker.ContainerOptions{
			Binds: []string{v.Name + ":/mnt/test"},
			User:  consts.UserRootString,
		},
	)
	require.NoError(t, res.Err)
	res = img.Run(
		ctx,
		[]string{"sh", "-c", "mkdir -p /mnt/test/foo/bar/ && printf 'test' > /mnt/test/foo/bar/baz.txt"},
		docker.ContainerOptions{
			Binds: []string{v.Name + ":/mnt/test"},
			User:  consts.UserRootString,
		},
	)
	require.NoError(t, res.Err)

	fr := file.NewRetriever(zaptest.NewLogger(t), cli, t.Name())

	t.Run("top-level file", func(t *testing.T) {
		b, err := fr.SingleFileContent(ctx, v.Name, "hello.txt")
		require.NoError(t, err)
		require.Equal(t, "hello world", string(b))
	})

	t.Run("nested file", func(t *testing.T) {
		b, err := fr.SingleFileContent(ctx, v.Name, "foo/bar/baz.txt")
		require.NoError(t, err)
		require.Equal(t, "test", string(b))
	})
}
