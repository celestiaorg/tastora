package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/docker/file"
	volumetypes "github.com/docker/docker/api/types/volume"
	dockerclient "github.com/moby/moby/client"
	"go.uber.org/zap"
)

// ContainerNode contains the fields and shared methods for docker nodes. (app nodes & bridge nodes)
type ContainerNode struct {
	VolumeName         string
	NetworkID          string
	DockerClient       *dockerclient.Client
	TestName           string
	Image              DockerImage
	containerLifecycle *ContainerLifecycle
	homeDir            string
	nodeType           string
	Index              int
	logger             *zap.Logger
}

// newContainerNode creates a new ContainerNode instance with the required parameters.
func newContainerNode(
	networkID string,
	dockerClient *dockerclient.Client,
	testName string,
	image DockerImage,
	homeDir string,
	idx int,
	nodeType string,
	logger *zap.Logger,
) *ContainerNode {
	return &ContainerNode{
		NetworkID:    networkID,
		DockerClient: dockerClient,
		TestName:     testName,
		Image:        image,
		homeDir:      homeDir,
		Index:        idx,
		logger:       logger,
		nodeType:     nodeType,
	}
}

// exec runs a command in the node's container.
func (n *ContainerNode) exec(ctx context.Context, logger *zap.Logger, cmd []string, env []string) ([]byte, []byte, error) {
	job := NewImage(logger, n.DockerClient, n.NetworkID, n.TestName, n.Image.Repository, n.Image.Version)
	opts := ContainerOptions{
		Env:   env,
		Binds: n.bind(),
	}
	res := job.Run(ctx, cmd, opts)
	if res.Err != nil {
		logger.Error("failed to run command", zap.String("cmd", fmt.Sprintf("%v", cmd)), zap.Error(res.Err), zap.String("stdout", string(res.Stdout)), zap.String("stderr", string(res.Stderr)), zap.Strings("env", env))
	}
	return res.Stdout, res.Stderr, res.Err
}

// bind returns the home folder bind point for running the ContainerNode.
func (n *ContainerNode) bind() []string {
	return []string{fmt.Sprintf("%s:%s", n.VolumeName, n.homeDir)}
}

// GetType returns the ContainerNode type as a string.
func (n *ContainerNode) GetType() string {
	return n.nodeType
}

// removeContainer gracefully stops and removes the container associated with the ContainerNode using the provided context.
func (n *ContainerNode) removeContainer(ctx context.Context) error {
	return n.containerLifecycle.RemoveContainer(ctx)
}

// stopContainer gracefully stops the container associated with the ContainerNode using the provided context.
func (n *ContainerNode) stopContainer(ctx context.Context) error {
	return n.containerLifecycle.StopContainer(ctx)
}

// startContainer starts the container associated with the ContainerNode using the provided context.
func (n *ContainerNode) startContainer(ctx context.Context) error {
	return n.containerLifecycle.StartContainer(ctx)
}

// ReadFile reads a file from the ContainerNode's container volume at the given relative path.
func (n *ContainerNode) ReadFile(ctx context.Context, relPath string) ([]byte, error) {
	fr := file.NewRetriever(n.logger, n.DockerClient, n.TestName)
	content, err := fr.SingleFileContent(ctx, n.VolumeName, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file at %s: %w", relPath, err)
	}
	return content, nil
}

// WriteFile accepts file contents in a byte slice and writes the contents to
// the docker filesystem. relPath describes the location of the file in the
// docker volume relative to the home directory.
func (n *ContainerNode) WriteFile(ctx context.Context, relPath string, content []byte) error {
	fw := file.NewWriter(n.logger, n.DockerClient, n.TestName)
	return fw.WriteFile(ctx, n.VolumeName, relPath, content)
}

// createAndSetupVolume creates a Docker volume for the node and sets up proper ownership.
// This consolidates the volume creation pattern used across all node types.
func (n *ContainerNode) createAndSetupVolume(ctx context.Context) error {
	v, err := n.DockerClient.VolumeCreate(ctx, volumetypes.CreateOptions{
		Labels: map[string]string{
			consts.CleanupLabel:   n.TestName,
			consts.NodeOwnerLabel: n.Name(),
		},
	})
	if err != nil {
		return fmt.Errorf("creating volume for %s: %w", n.nodeType, err)
	}

	n.VolumeName = v.Name

	// configure volume ownership
	if err := SetVolumeOwner(ctx, VolumeOwnerOptions{
		Log:        n.logger,
		Client:     n.DockerClient,
		VolumeName: v.Name,
		ImageRef:   n.Image.Ref(),
		TestName:   n.TestName,
		UidGid:     n.Image.UIDGID,
	}); err != nil {
		return fmt.Errorf("set volume owner: %w", err)
	}

	return nil
}
