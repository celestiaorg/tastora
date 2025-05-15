package docker

import (
	"context"
	"fmt"
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
}

// newNode creates a new node instance with the required parameters.
func newNode(
	networkID string,
	dockerClient *dockerclient.Client,
	testName string,
	image DockerImage,
	homeDir string,
	nodeType string,
) *node {
	return &node{
		NetworkID:    networkID,
		DockerClient: dockerClient,
		TestName:     testName,
		Image:        image,
		homeDir:      homeDir,
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
		logger.Error("failed to run command", zap.String("cmd", fmt.Sprintf("%v", cmd)), zap.Error(res.Err), zap.String("stderr", string(res.Stderr)), zap.Strings("env", env))
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

// RemoveContainer gracefully stops and removes the container associated with the node using the provided context.
func (n *node) RemoveContainer(ctx context.Context) error {
	return n.containerLifecycle.RemoveContainer(ctx)
}
