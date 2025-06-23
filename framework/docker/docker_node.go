package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/file"
	dockerclient "github.com/moby/moby/client"
	"go.uber.org/zap"
)

// node contains the fields and shared methods for docker nodes. (app nodes & bridge nodes)
type node struct {
	VolumeName         string
	NetworkID          string
	DockerClient       *dockerclient.Client
	TestName           string
	Image              DockerImage
	containerLifecycle *ContainerLifecycle
	homeDir            string
	nodeType           string
	Index              int
	Logger             *zap.Logger
}

// newNode creates a new node instance with the required parameters.
func newNode(
	networkID string,
	dockerClient *dockerclient.Client,
	testName string,
	image DockerImage,
	homeDir string,
	idx int,
	nodeType string,
	logger *zap.Logger,
) *node {
	return &node{
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

// exec runs a command in the node's container.
func (n *node) exec(ctx context.Context, logger *zap.Logger, cmd []string, env []string) ([]byte, []byte, error) {
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

// bind returns the home folder bind point for running the node.
func (n *node) bind() []string {
	return []string{fmt.Sprintf("%s:%s", n.VolumeName, n.homeDir)}
}

// GetType returns the node type as a string.
func (n *node) GetType() string {
	return n.nodeType
}

// removeContainer gracefully stops and removes the container associated with the node using the provided context.
func (n *node) removeContainer(ctx context.Context) error {
	return n.containerLifecycle.RemoveContainer(ctx)
}

// stopContainer gracefully stops the container associated with the node using the provided context.
func (n *node) stopContainer(ctx context.Context) error {
	return n.containerLifecycle.StopContainer(ctx)
}

// startContainer starts the container associated with the node using the provided context.
func (n *node) startContainer(ctx context.Context) error {
	return n.containerLifecycle.StartContainer(ctx)
}

// ReadFile reads a file from the node's container volume at the given relative path.
func (n *node) ReadFile(ctx context.Context, relPath string) ([]byte, error) {
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
func (n *node) WriteFile(ctx context.Context, relPath string, content []byte) error {
	fw := file.NewWriter(n.Logger, n.DockerClient, n.TestName)
	return fw.WriteFile(ctx, n.VolumeName, relPath, content)
}
