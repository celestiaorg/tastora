package container

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/docker/port"
	"github.com/celestiaorg/tastora/framework/types"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

// Example Go/Cosmos-SDK panic format is `panic: bad Duration: time: invalid duration "bad"\n`.
var panicRe = regexp.MustCompile(`panic:.*\n`)

type Lifecycle struct {
	log           *zap.Logger
	client        types.TastoraDockerClient
	containerName string
	id            string
	hostNetwork   bool

	// createOpts is retained so the container can be recreated with pinned host ports.
	createOpts client.ContainerCreateOptions
	// assignedPorts holds the host ports Docker assigned on the first start.
	assignedPorts network.PortMap
	// portsPinned is true once the container's bindings name explicit host ports,
	// so Docker reuses them on every subsequent start.
	portsPinned bool
}

func (c *Lifecycle) SetHostNetwork(enabled bool) {
	c.hostNetwork = enabled
}

func NewLifecycle(log *zap.Logger, c types.TastoraDockerClient, containerName string) *Lifecycle {
	return &Lifecycle{
		log:           log,
		client:        c,
		containerName: containerName,
	}
}

func (c *Lifecycle) CreateContainer(
	ctx context.Context,
	testName string,
	networkID string,
	image Image,
	ports network.PortMap,
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

	containerCfg := &container.Config{
		Image:      imageRef,
		Entrypoint: entrypoint,
		Cmd:        cmd,
		Env:        env,
		Hostname:   hostName,
		Labels:     map[string]string{consts.CleanupLabel: c.client.CleanupLabel()},
	}
	if image.UIDGID != "" {
		containerCfg.User = image.UIDGID
	}

	hostCfg := &container.HostConfig{
		Binds:  volumeBinds,
		Mounts: mounts,
	}

	var netCfg *network.NetworkingConfig

	if c.hostNetwork {
		hostCfg.NetworkMode = "host"
	} else {
		imageInfo, err := c.client.ImageInspect(ctx, imageRef)
		if err != nil {
			return fmt.Errorf("inspect image %s: %w", imageRef, err)
		}

		pS := network.PortSet{}
		pb := network.PortMap{}
		localhost := netip.MustParseAddr("127.0.0.1")
		if imageInfo.Config != nil {
			for exposedPort := range imageInfo.Config.ExposedPorts {
				port, parseErr := network.ParsePort(exposedPort)
				if parseErr != nil {
					return fmt.Errorf("invalid exposed port %q in image %s: %w", exposedPort, imageRef, parseErr)
				}
				pS[port] = struct{}{}
				pb[port] = []network.PortBinding{{HostIP: localhost}}
			}
		}
		for k := range ports {
			pS[k] = struct{}{}
			pb[k] = []network.PortBinding{{HostIP: localhost}}
		}
		containerCfg.ExposedPorts = pS
		hostCfg.PortBindings = pb

		var endpointSettings network.EndpointSettings
		if ipAddr != "" {
			addr, parseErr := netip.ParseAddr(ipAddr)
			if parseErr != nil {
				return fmt.Errorf("invalid container IP %q: %w", ipAddr, parseErr)
			}
			endpointSettings = network.EndpointSettings{
				IPAMConfig: &network.EndpointIPAMConfig{
					IPv4Address: addr,
				},
			}
		}
		netCfg = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				networkID: &endpointSettings,
			},
		}
	}

	opts := client.ContainerCreateOptions{
		Name:             c.containerName,
		Config:           containerCfg,
		HostConfig:       hostCfg,
		NetworkingConfig: netCfg,
	}
	cc, err := c.client.ContainerCreate(ctx, opts)
	if err != nil {
		return err
	}
	c.id = cc.ID
	c.createOpts = opts
	c.assignedPorts = nil
	c.portsPinned = len(hostCfg.PortBindings) == 0
	return nil
}

// StartContainer starts the container. Docker assigns host ports on the first start;
// they are recorded, and later starts recreate the container with those ports pinned so
// host addresses remain stable across stop/start cycles.
func (c *Lifecycle) StartContainer(ctx context.Context) error {
	if c.assignedPorts != nil && !c.portsPinned {
		if err := c.recreateWithAssignedPorts(ctx); err != nil {
			return fmt.Errorf("pin host ports for %s: %w", c.containerName, err)
		}
	}

	if err := internal.StartContainer(ctx, c.client, c.id); err != nil {
		return err
	}

	if err := c.checkForFailedStart(ctx, time.Second*2); err != nil {
		return err
	}

	if c.assignedPorts == nil && !c.portsPinned {
		if err := c.recordAssignedPorts(ctx); err != nil {
			return fmt.Errorf("record host ports for %s: %w", c.containerName, err)
		}
	}

	c.log.Info("Container started", zap.String("container", c.containerName))
	return nil
}

// recordAssignedPorts saves the host ports Docker assigned to the running container.
func (c *Lifecycle) recordAssignedPorts(ctx context.Context) error {
	res, err := c.client.ContainerInspect(ctx, c.id, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	if res.Container.NetworkSettings == nil {
		return fmt.Errorf("container has no network settings")
	}
	assigned := make(network.PortMap, len(c.createOpts.HostConfig.PortBindings))
	for p, requested := range c.createOpts.HostConfig.PortBindings {
		hostIP := requested[0].HostIP
		for _, b := range res.Container.NetworkSettings.Ports[p] {
			if b.HostIP == hostIP && b.HostPort != "" {
				assigned[p] = []network.PortBinding{{HostIP: hostIP, HostPort: b.HostPort}}
				break
			}
		}
		if _, ok := assigned[p]; !ok {
			return fmt.Errorf("no host port assigned for %s", p)
		}
	}
	c.assignedPorts = assigned
	return nil
}

// recreateWithAssignedPorts replaces the stopped container with an identical one whose
// bindings name the previously assigned host ports. Docker cannot change the bindings of
// an existing container. Named volumes and bind mounts are reattached; anonymous volumes
// are carried over by name. Writes to the container filesystem outside volumes are lost.
func (c *Lifecycle) recreateWithAssignedPorts(ctx context.Context) error {
	res, err := c.client.ContainerInspect(ctx, c.id, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	if res.Container.State != nil && res.Container.State.Running {
		// already running with its assigned ports; pin on a later start
		return nil
	}

	hostCfg := *c.createOpts.HostConfig
	hostCfg.PortBindings = c.assignedPorts
	hostCfg.Mounts = append(slices.Clone(hostCfg.Mounts), anonymousVolumeMounts(res.Container.Mounts, &hostCfg)...)

	if _, err := c.client.ContainerRemove(ctx, c.id, client.ContainerRemoveOptions{}); err != nil {
		return fmt.Errorf("remove container: %w", err)
	}

	opts := c.createOpts
	opts.HostConfig = &hostCfg
	cc, err := c.client.ContainerCreate(ctx, opts)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	c.id = cc.ID
	c.createOpts = opts
	c.portsPinned = true
	return nil
}

// anonymousVolumeMounts returns mounts for volumes attached to the container that are not
// declared in hostCfg, such as those created from image VOLUME directives.
func anonymousVolumeMounts(current []container.MountPoint, hostCfg *container.HostConfig) []mount.Mount {
	declared := make(map[string]bool)
	for _, b := range hostCfg.Binds {
		if parts := strings.Split(b, ":"); len(parts) > 1 {
			declared[parts[1]] = true
		}
	}
	for _, m := range hostCfg.Mounts {
		declared[m.Target] = true
	}
	var out []mount.Mount
	for _, mp := range current {
		if mp.Type != mount.TypeVolume || mp.Name == "" || declared[mp.Destination] {
			continue
		}
		out = append(out, mount.Mount{Type: mount.TypeVolume, Source: mp.Name, Target: mp.Destination, ReadOnly: !mp.RW})
	}
	return out
}

// ContainerName returns the container's name, which stays stable when the container is
// recreated, unlike its ID.
func (c *Lifecycle) ContainerName() string {
	return c.containerName
}

// checkForFailedStart checks if the container failed to start by analyzing logs and inspecting its state after waiting.
func (c *Lifecycle) checkForFailedStart(ctx context.Context, wait time.Duration) error {
	time.Sleep(wait)

	containerLogs, err := c.client.ContainerLogs(ctx, c.id, client.ContainerLogsOptions{
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

	inspectResult, err := c.client.ContainerInspect(ctx, c.id, client.ContainerInspectOptions{})
	if err != nil {
		return fmt.Errorf("failed to inspect container %s: %w", c.containerName, err)
	}
	inspect := inspectResult.Container

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

func (c *Lifecycle) PauseContainer(ctx context.Context) error {
	_, err := c.client.ContainerPause(ctx, c.id, client.ContainerPauseOptions{})
	return err
}

func (c *Lifecycle) UnpauseContainer(ctx context.Context) error {
	_, err := c.client.ContainerUnpause(ctx, c.id, client.ContainerUnpauseOptions{})
	return err
}

func (c *Lifecycle) StopContainer(ctx context.Context) error {
	timeoutSec := 30
	_, err := c.client.ContainerStop(ctx, c.id, client.ContainerStopOptions{Timeout: &timeoutSec})
	if err != nil && errdefs.IsNotModified(err) {
		// container is already stopped, this is not an error
		return nil
	}
	return err
}

func (c *Lifecycle) RemoveContainer(ctx context.Context, opts ...types.RemoveOption) error {
	// default to force removal and remove volumes
	// Note: RemoveVolumes only removes anonymous volumes attached to the container.
	// Named volumes created with VolumeCreate() must be removed separately.
	// Reference: https://github.com/docker/cli/issues/4028 - Docker API behavior for volume removal
	_, err := c.client.ContainerRemove(ctx, c.id, ApplyRemoveOptions(opts...))
	if err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("remove container %s: %w", c.containerName, err)
	}
	return nil
}

func (c *Lifecycle) RemoveVolumes(ctx context.Context) error {
	volumeList, err := c.client.VolumeList(ctx, client.VolumeListOptions{
		Filters: make(client.Filters).Add("label", fmt.Sprintf("%s=%s", consts.CleanupLabel, c.client.CleanupLabel())),
	})
	if err != nil {
		return fmt.Errorf("failed to list volumes: %w", err)
	}

	for _, vol := range volumeList.Items {
		_, err := c.client.VolumeRemove(ctx, vol.Name, client.VolumeRemoveOptions{Force: true})
		if err != nil {
			c.log.Warn("Failed to force remove volume", zap.String("volume", vol.Name), zap.Error(err))
		}
	}
	return nil
}

func (c *Lifecycle) ContainerID() string {
	return c.id
}

func (c *Lifecycle) GetHostPorts(ctx context.Context, portIDs ...string) ([]string, error) {
	cjsonResult, err := c.client.ContainerInspect(ctx, c.id, client.ContainerInspectOptions{})
	if err != nil {
		return nil, err
	}
	ports := make([]string, len(portIDs))
	for i, p := range portIDs {
		ports[i] = port.GetForHost(cjsonResult.Container, p)
	}
	return ports, nil
}

// Running will inspect the container and check its state to determine if it is currently running.
// If the container is running nil will be returned, otherwise an error is returned.
func (c *Lifecycle) Running(ctx context.Context) error {
	cjsonResult, err := c.client.ContainerInspect(ctx, c.id, client.ContainerInspectOptions{})
	if err != nil {
		return err
	}
	if cjsonResult.Container.State.Running {
		return nil
	}
	return fmt.Errorf("container with name %s and id %s is not running", c.containerName, c.id)
}

func ApplyRemoveOptions(opts ...types.RemoveOption) client.ContainerRemoveOptions {
	removeOpts := client.ContainerRemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	}
	for _, opt := range opts {
		opt(&removeOpts)
	}
	return removeOpts
}
