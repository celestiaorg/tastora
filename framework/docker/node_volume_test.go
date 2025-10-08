package docker

import (
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"testing"
)

func TestNode_VolumeRebinding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}
	t.Parallel()

	testCfg := setupDockerTest(t)

	logger := zaptest.NewLogger(t)
	image := container.Image{
		Repository: "alpine",
		Version:    "latest",
		UIDGID:     "0:0",
	}

	createNode := func() *container.Node {
		node := container.NewNode(
			testCfg.NetworkID,
			testCfg.DockerClient,
			testCfg.TestName,
			image,
			"/test",
			0,
			types.NodeTypeValidator,
			logger,
		)
		node.SetContainerLifecycle(container.NewLifecycle(logger, testCfg.DockerClient, testCfg.TestName))

		nodeName := testCfg.TestName + "-test-node-0"

		err := node.CreateAndSetupVolume(testCfg.Ctx, nodeName)
		require.NoError(t, err)
		require.NotEmpty(t, node.VolumeName)

		// create a container that will just sleep forever so the test can exec into it.
		err = node.CreateContainer(
			testCfg.Ctx,
			t.Name(),
			testCfg.NetworkID,
			image,
			nil,
			"",
			[]string{node.GetVolumeName(nodeName) + ":/test"},
			nil,
			internal.CondenseHostName(nodeName),
			[]string{"sleep", "infinity"},
			nil,
			nil,
		)
		require.NoError(t, err)
		return node
	}

	node := createNode()

	err := node.StartContainer(testCfg.Ctx)
	require.NoError(t, err)

	volumeName1 := node.VolumeName

	// write test files to volume
	testContent1 := []byte("test file 1 content")
	testContent2 := []byte("test file 2 content")

	err = node.WriteFile(testCfg.Ctx, "test1.txt", testContent1)
	require.NoError(t, err)

	err = node.WriteFile(testCfg.Ctx, "test2.txt", testContent2)
	require.NoError(t, err)

	// verify files were written
	readContent1, err := node.ReadFile(testCfg.Ctx, "test1.txt")
	require.NoError(t, err)
	require.Equal(t, testContent1, readContent1)

	readContent2, err := node.ReadFile(testCfg.Ctx, "test2.txt")
	require.NoError(t, err)
	require.Equal(t, testContent2, readContent2)

	// remove the code but preserver the volumes, this allows us to create a new node on the same volume.
	err = node.Remove(testCfg.Ctx, types.WithPreserveVolumes())
	require.NoError(t, err)

	// re-creating exact same node, which should bind to the same volume.
	node = createNode()

	volumeName2 := node.VolumeName

	// verify that the same volume was used
	require.Equal(t, volumeName1, volumeName2, "should reuse the same volume")

	// verify that files are still present
	readContent1, err = node.ReadFile(testCfg.Ctx, "test1.txt")
	require.NoError(t, err)
	require.Equal(t, testContent1, readContent1)

	readContent2, err = node.ReadFile(testCfg.Ctx, "test2.txt")
	require.NoError(t, err)
	require.Equal(t, testContent2, readContent2)
}
