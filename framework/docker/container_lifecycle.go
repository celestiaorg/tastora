package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	dockerclient "github.com/moby/moby/client"
	"github.com/moby/moby/errdefs"
	"go.uber.org/zap"
)

// Example Go/Cosmos-SDK panic format is `panic: bad Duration: time: invalid duration "bad"\n`.
var panicRe = regexp.MustCompile(`panic:.*\n`)

type ContainerLifecycle struct {
	log               *zap.Logger
	client            *dockerclient.Client
	containerName     string
	id                string
	preStartListeners Listeners
}

func NewContainerLifecycle(log *zap.Logger, client *dockerclient.Client, containerName string) *ContainerLifecycle {
	return &ContainerLifecycle{
		log:           log,
		client:        client,
		containerName: containerName,
	}
}

func (c *ContainerLifecycle) CreateContainer(
	ctx context.Context,
	testName string,
	networkID string,
	image DockerImage,
	ports nat.PortMap,
	ipAddr string,
	volumeBinds []string,
	mounts []mount.Mount,
	hostName string,
	cmd []string,
	env []string,
	entrypoint []string,
) error {
	imageRef := image.Ref()
	c.log.Info(
		"Will run command",
		zap.String("image", imageRef),
		zap.String("container", c.containerName),
		zap.String("command", strings.Join(cmd, " ")), //nolint:gosec // testing only so safe to expose credentials in cli
	)

	if err := image.PullImage(ctx, c.client); err != nil {
		return err
	}

	pS := nat.PortSet{}
	for k := range ports {
		pS[k] = struct{}{}
	}

	pb, listeners, err := GeneratePortBindings(ports)
	if err != nil {
		return fmt.Errorf("failed to generate port bindings: %w", err)
	}

	c.preStartListeners = listeners

	var endpointSettings network.EndpointSettings
	if ipAddr == "" {
		endpointSettings = network.EndpointSettings{}
	} else {
		endpointSettings = network.EndpointSettings{
			IPAMConfig: &network.EndpointIPAMConfig{
				IPv4Address: ipAddr,
			},
		}
	}

	cc, err := c.client.ContainerCreate(
		ctx,
		&container.Config{
			Image: imageRef,

			Entrypoint:   entrypoint,
			Cmd:          cmd,
			Env:          env,
			Hostname:     hostName,
			Labels:       map[string]string{consts.CleanupLabel: testName},
			ExposedPorts: pS,
		},
		&container.HostConfig{
			Binds:           volumeBinds,
			PortBindings:    pb,
			PublishAllPorts: true,
			AutoRemove:      false,
			DNS:             []string{},
			Mounts:          mounts,
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				networkID: &endpointSettings,
			},
		},
		nil,
		c.containerName,
	)
	if err != nil {
		listeners.CloseAll()
		c.preStartListeners = []net.Listener{}
		return err
	}
	c.id = cc.ID
	return nil
}

func (c *ContainerLifecycle) StartContainer(ctx context.Context) error {
	// lock port allocation for the time between freeing the ports from the
	// temporary listeners to the consumption of the ports by the container
	mu.RLock()
	defer mu.RUnlock()

	c.preStartListeners.CloseAll()
	c.preStartListeners = []net.Listener{}

	if err := StartContainer(ctx, c.client, c.id); err != nil {
		return err
	}

	if err := c.checkForFailedStart(ctx, time.Second*2); err != nil {
		return err
	}

	c.log.Info("Container started", zap.String("container", c.containerName))
	return nil
}

// checkForFailedStart checks if the container failed to start by analyzing logs and inspecting its state after waiting.
func (c *ContainerLifecycle) checkForFailedStart(ctx context.Context, wait time.Duration) error {
	time.Sleep(wait)

	containerLogs, err := c.client.ContainerLogs(ctx, c.id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})

	if err != nil {
		return fmt.Errorf("failed to read logs from container %s: %w", c.containerName, err)
	}
	defer func() { _ = containerLogs.Close() }()

	logs := new(strings.Builder)
	_, err = io.Copy(logs, containerLogs)
	if err != nil {
		return fmt.Errorf("failed to read logs from container %s: %w", c.containerName, err)
	}

	if err := parseSDKPanicFromText(logs.String()); err != nil {
		fmt.Printf("\nContainer name: %s.\nerror: %s.\nlogs\n%s\n", c.containerName, err.Error(), logs.String())
		return fmt.Errorf("container %s failed to start: %w", c.containerName, err)
	}

	inspect, err := c.client.ContainerInspect(ctx, c.id)
	if err != nil {
		return fmt.Errorf("failed to inspect container %s: %w", c.containerName, err)
	}

	if !inspect.State.Running {
		return fmt.Errorf("container %s exited early (status: %s, exit code: %d)\nlogs:\n %s", c.containerName, inspect.State.Status, inspect.State.ExitCode, logs)
	}

	return nil
}

// parseSDKPanicFromText returns a panic line if it exists in the logs so
// that it can be returned to the user in a proper error message instead of
// hanging.
func parseSDKPanicFromText(text string) error {
	if !strings.Contains(text, "panic: ") {
		return nil
	}

	match := panicRe.FindString(text)
	if match != "" {
		panicMessage := strings.TrimSpace(match)
		return fmt.Errorf("%s", panicMessage)
	}

	return nil
}

func (c *ContainerLifecycle) PauseContainer(ctx context.Context) error {
	return c.client.ContainerPause(ctx, c.id)
}

func (c *ContainerLifecycle) UnpauseContainer(ctx context.Context) error {
	return c.client.ContainerUnpause(ctx, c.id)
}

func (c *ContainerLifecycle) StopContainer(ctx context.Context) error {
	var timeout container.StopOptions
	timeoutSec := 30
	timeout.Timeout = &timeoutSec

	return c.client.ContainerStop(ctx, c.id, timeout)
}

func (c *ContainerLifecycle) RemoveContainer(ctx context.Context) error {
	err := c.client.ContainerRemove(ctx, c.id, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove container %s: %w", c.containerName, err)
	}
	return nil
}

func (c *ContainerLifecycle) ContainerID() string {
	return c.id
}

func (c *ContainerLifecycle) GetHostPorts(ctx context.Context, portIDs ...string) ([]string, error) {
	cjson, err := c.client.ContainerInspect(ctx, c.id)
	if err != nil {
		return nil, err
	}
	ports := make([]string, len(portIDs))
	for i, p := range portIDs {
		ports[i] = GetHostPort(cjson, p)
	}
	return ports, nil
}

// Running will inspect the container and check its state to determine if it is currently running.
// If the container is running nil will be returned, otherwise an error is returned.
func (c *ContainerLifecycle) Running(ctx context.Context) error {
	cjson, err := c.client.ContainerInspect(ctx, c.id)
	if err != nil {
		return err
	}
	if cjson.State.Running {
		return nil
	}
	return fmt.Errorf("container with name %s and id %s is not running", c.containerName, c.id)
}
