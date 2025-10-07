package container

import (
	"context"
	"fmt"

	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/docker/file"
	"github.com/celestiaorg/tastora/framework/docker/volume"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/docker/api/types/mount"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/docker/go-connections/nat"
	dockerclient "github.com/moby/moby/client"
	"go.uber.org/zap"
)

// Node contains the fields and shared methods for docker nodes. (app nodes & bridge nodes)
type Node struct {
	VolumeName         string
	NetworkID          string
	DockerClient       *dockerclient.Client
	TestName           string
	Image              Image
	ContainerLifecycle *Lifecycle
	homeDir            string
	nodeType           types.NodeType
	Index              int
	Logger             *zap.Logger
}

// NewNode creates a new Node instance with the required parameters.
func NewNode(
	networkID string,
	dockerClient *dockerclient.Client,
	testName string,
	image Image,
	homeDir string,
	idx int,
	nodeType types.NodeType,
	logger *zap.Logger,
) *Node {
	return &Node{
		NetworkID:    networkID,
		DockerClient: dockerClient,
		TestName:     testName,
		Image:        image,
		homeDir:      homeDir,
		Index:        idx,
		Logger:       logger,
		nodeType:     nodeType,
	}
}

// SetContainerLifecycle sets the container lifecycle for the node
func (n *Node) SetContainerLifecycle(lifecycle *Lifecycle) {
	n.ContainerLifecycle = lifecycle
}

// HomeDir returns the home directory path
func (n *Node) HomeDir() string {
	return n.homeDir
}

// Exec runs a command in the node's container.
func (n *Node) Exec(ctx context.Context, logger *zap.Logger, cmd []string, env []string) ([]byte, []byte, error) {
	job := NewJob(logger, n.DockerClient, n.NetworkID, n.TestName, n.Image.Repository, n.Image.Version)
	opts := Options{
		Env:   env,
		Binds: n.Bind(),
	}
	res := job.Run(ctx, cmd, opts)
	if res.Err != nil {
		logger.Error("failed to run command", zap.String("cmd", fmt.Sprintf("%v", cmd)), zap.Error(res.Err), zap.String("stdout", string(res.Stdout)), zap.String("stderr", string(res.Stderr)), zap.Strings("env", env))
	}
	return res.Stdout, res.Stderr, res.Err
}

// Bind returns the home folder Bind point for running the Node.
func (n *Node) Bind() []string {
	return []string{fmt.Sprintf("%s:%s", n.VolumeName, n.homeDir)}
}

// GetType returns the Node type.
func (n *Node) GetType() types.NodeType {
	return n.nodeType
}

// RemoveContainer gracefully stops and removes the container associated with the Node using the provided context.
func (n *Node) RemoveContainer(ctx context.Context, opts ...types.RemoveOption) error {
	err := n.ContainerLifecycle.RemoveContainer(ctx, opts...)
	if err != nil {
		return err
	}
	return n.ContainerLifecycle.RemoveVolumes(ctx, n.TestName)
}

// StopContainer gracefully stops the container associated with the Node using the provided context.
func (n *Node) StopContainer(ctx context.Context) error {
	return n.ContainerLifecycle.StopContainer(ctx)
}

// StartContainer starts the container associated with the Node using the provided context.
func (n *Node) StartContainer(ctx context.Context) error {
	return n.ContainerLifecycle.StartContainer(ctx)
}

// Stop stops the Node container gracefully using the provided context.
func (n *Node) Stop(ctx context.Context) error {
	if err := n.StopContainer(ctx); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	return nil
}

// Remove stops and removes the Node container and its resources.
func (n *Node) Remove(ctx context.Context, opts ...types.RemoveOption) error {
	if err := n.StopContainer(ctx); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	return n.RemoveContainer(ctx, opts...)
}

func (n *Node) CreateContainer(ctx context.Context,
	testName string,
	networkID string,
	image Image,
	ports nat.PortMap,
	ipAddr string,
	volumeBinds []string,
	mounts []mount.Mount,
	hostName string,
	cmd []string,
	env []string,
	entrypoint []string) error {
	return n.ContainerLifecycle.CreateContainer(ctx, testName, networkID, image, ports, ipAddr, volumeBinds, mounts, hostName, cmd, env, entrypoint)
}

// ReadFile reads a file from the Node's container volume at the given relative path.
func (n *Node) ReadFile(ctx context.Context, relPath string) ([]byte, error) {
	fr := file.NewRetriever(n.Logger, n.DockerClient, n.TestName)
	content, err := fr.SingleFileContent(ctx, n.VolumeName, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file at %s: %w", relPath, err)
	}
	return content, nil
}

// WriteFile accepts file contents in a byte slice and writes the contents to
// the docker filesystem. relPath describes the location of the file in the
// docker volume relative to the home directory.
func (n *Node) WriteFile(ctx context.Context, relPath string, content []byte) error {
	fw := file.NewWriter(n.Logger, n.DockerClient, n.TestName)
	return fw.WriteFile(ctx, n.VolumeName, relPath, content)
}

// CreateAndSetupVolume creates a Docker volume for the node and sets up proper ownership.
// This consolidates the volume creation pattern used across all node types.
// The nodeName parameter should be the specific name for this node instance.
func (n *Node) CreateAndSetupVolume(ctx context.Context, nodeName string) error {
	// create volume with appropriate labels
	v, err := n.DockerClient.VolumeCreate(ctx, volumetypes.CreateOptions{
		Labels: map[string]string{
			consts.CleanupLabel:   n.TestName,
			consts.NodeOwnerLabel: nodeName,
		},
	})
	if err != nil {
		return fmt.Errorf("creating volume for %s: %w", n.nodeType.String(), err)
	}

	n.VolumeName = v.Name

	// configure volume ownership
	if err := volume.SetOwner(ctx, volume.OwnerOptions{
		Log:        n.Logger,
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
