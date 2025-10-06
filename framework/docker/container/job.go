package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/testutil/random"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockerimagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/moby/moby/client"
	"github.com/moby/moby/errdefs"
	"github.com/moby/moby/pkg/stdcopy"
	"go.uber.org/zap"
)

// Job is a docker job runner.
type Job struct {
	log             *zap.Logger
	client          *client.Client
	repository, tag string

	networkID string
	testName  string
}

// NewJob returns a valid Job.
//
// "pool" and "networkID" are likely from DockerSetup.
// "testName" is from a (*testing.T).Name() and should match the t.Name() from DockerSetup to ensure proper cleanup.
//
// Most arguments (except tag) must be non-zero values or this function panics.
// If tag is absent, defaults to "latest".
// Currently, only public docker images are supported.
func NewJob(logger *zap.Logger, cli *client.Client, networkID string, testName string, repository, tag string) *Job {
	if logger == nil {
		panic(errors.New("nil Logger"))
	}
	if cli == nil {
		panic(errors.New("client cannot be nil"))
	}
	if networkID == "" {
		panic(errors.New("networkID cannot be empty"))
	}
	if testName == "" {
		panic("testName cannot be empty")
	}
	if repository == "" {
		panic(errors.New("repository cannot be empty"))
	}
	if tag == "" {
		tag = "latest"
	}

	i := &Job{
		client:     cli,
		networkID:  networkID,
		repository: repository,
		tag:        tag,
		testName:   testName,
	}
	// Assign log after creating, so the imageRef method can be used.
	i.log = logger.With(
		zap.String("image", i.imageRef()),
		zap.String("test_name", testName),
	)
	return i
}

// Options optionally configures starting a Container.
type Options struct {
	// Bind mounts: https://docs.docker.com/storage/bind-mounts/
	Binds []string

	// Environment variables
	Env []string

	// If blank, defaults to the container's default user.
	User string

	// If non-zero, will limit the amount of log lines returned.
	LogTail uint64

	// mounts directories
	Mounts []mount.Mount

	// working directory to launch cmd from
	WorkingDir string
}

// ExecResult is a wrapper type that wraps an exit code and associated output from stderr & stdout, along with
// an error in the case of some error occurring during container execution.
type ExecResult struct {
	Err            error // Err is nil, unless some error occurs during the container lifecycle.
	ExitCode       int
	Stdout, Stderr []byte
}

// Run creates and runs a container invoking "cmd". The container resources are removed after exit.
//
// Run blocks until the command completes. Thus, Run is not suitable for daemons or servers. Use Start instead.
// A non-zero status code returns an error.
func (job *Job) Run(ctx context.Context, cmd []string, opts Options) ExecResult {
	c, err := job.Start(ctx, cmd, opts)
	if err != nil {
		return ExecResult{
			Err:      err,
			ExitCode: -1,
			Stdout:   nil,
			Stderr:   nil,
		}
	}
	return c.Wait(ctx, opts.LogTail)
}

func (job *Job) imageRef() string {
	return job.repository + ":" + job.tag
}

// EnsurePulled can only pull public images.
func (job *Job) EnsurePulled(ctx context.Context) error {
	ref := job.imageRef()
	_, _, err := job.client.ImageInspectWithRaw(ctx, ref)
	if err != nil {
		rc, err := job.client.ImagePull(ctx, ref, dockerimagetypes.PullOptions{})
		if err != nil {
			return fmt.Errorf("pull image %s: %w", ref, err)
		}
		_, _ = io.Copy(io.Discard, rc)
		_ = rc.Close()
	}
	return nil
}

func (job *Job) CreateContainer(ctx context.Context, containerName, hostName string, cmd []string, opts Options) (string, error) {
	// Although this shouldn't happen because the name includes randomness, in reality there seems to intermittent
	// chances of collisions.

	containers, err := job.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", containerName)),
	})
	if err != nil {
		return "", fmt.Errorf("unable to list containers: %w", err)
	}

	for _, c := range containers {
		if err := job.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{
			RemoveVolumes: true,
			Force:         true,
		}); err != nil {
			return "", fmt.Errorf("unable to remove container %s: %w", containerName, err)
		}
	}

	cc, err := job.client.ContainerCreate(
		ctx,
		&container.Config{
			Image: job.imageRef(),

			Entrypoint: []string{},
			WorkingDir: opts.WorkingDir,
			Cmd:        cmd,

			Env: opts.Env,

			Hostname: hostName,
			User:     opts.User,

			Labels: map[string]string{consts.CleanupLabel: job.testName},
		},
		&container.HostConfig{
			Binds:           opts.Binds,
			PublishAllPorts: true, // Because we publish all ports, no need to expose specific ports.
			AutoRemove:      false,
			Mounts:          opts.Mounts,
		},
		&network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				job.networkID: {},
			},
		},
		nil,
		containerName,
	)
	if err != nil {
		return "", err
	}
	return cc.ID, nil
}

// Start pulls the image if not present, creates a container, and runs it.
func (job *Job) Start(ctx context.Context, cmd []string, opts Options) (*Container, error) {
	if len(cmd) == 0 {
		panic(errors.New("cmd cannot be empty"))
	}

	if err := job.EnsurePulled(ctx); err != nil {
		return nil, job.WrapErr(err)
	}

	var (
		containerName = internal.SanitizeDockerResourceName(job.testName + "-" + random.LowerCaseLetterString(6))
		hostName      = internal.CondenseHostName(containerName)
		logger        = job.log.With(
			zap.String("command", strings.Join(cmd, " ")), //nolint:gosec // testing only so safe to expose credentials in cli
			zap.Strings("env,", opts.Env),
			zap.String("hostname", hostName),
			zap.String("container", containerName),
		)
	)

	cID, err := job.CreateContainer(ctx, containerName, hostName, cmd, opts)
	if err != nil {
		return nil, job.WrapErr(fmt.Errorf("create container %s: %w", containerName, err))
	}

	logger.Info("Exec")

	err = internal.StartContainer(ctx, job.client, cID)
	if err != nil {
		return nil, job.WrapErr(fmt.Errorf("start container %s: %w", containerName, err))
	}

	return &Container{
		Name:        containerName,
		Hostname:    hostName,
		log:         logger,
		job:         job,
		containerID: cID,
	}, nil
}

func (job *Job) WrapErr(err error) error {
	return fmt.Errorf("job %s:%s: %w", job.repository, job.tag, err)
}

// Container is a docker container. Use (*Image).Start to create a new container.
type Container struct {
	Name     string
	Hostname string

	log         *zap.Logger
	job         *Job
	containerID string
}

// Wait blocks until the container exits. Calling wait is not suitable for daemons and servers.
// A non-zero status code returns an error.
//
// Wait implicitly calls Stop.
// If logTail is non-zero, the stdout and stderr logs will be truncated at the end to that number of lines.
func (c *Container) Wait(ctx context.Context, logTail uint64) ExecResult {
	waitCh, errCh := c.job.client.ContainerWait(ctx, c.containerID, container.WaitConditionNotRunning)
	var exitCode int
	select {
	case <-ctx.Done():
		return ExecResult{
			Err:      ctx.Err(),
			ExitCode: 1,
			Stdout:   nil,
			Stderr:   nil,
		}
	case err := <-errCh:
		return ExecResult{
			Err:      err,
			ExitCode: 1,
			Stdout:   nil,
			Stderr:   nil,
		}
	case res := <-waitCh:
		exitCode = int(res.StatusCode)
		if res.Error != nil {
			return ExecResult{
				Err:      errors.New(res.Error.Message),
				ExitCode: exitCode,
				Stdout:   nil,
				Stderr:   nil,
			}
		}
	}

	var (
		stdoutBuf = new(bytes.Buffer)
		stderrBuf = new(bytes.Buffer)
	)

	logOpts := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	}
	if logTail != 0 {
		logOpts.Tail = strconv.FormatUint(logTail, 10)
	}

	rc, err := c.job.client.ContainerLogs(ctx, c.containerID, logOpts)
	if err != nil {
		return ExecResult{
			Err:      err,
			ExitCode: exitCode,
			Stdout:   nil,
			Stderr:   nil,
		}
	}
	defer func() { _ = rc.Close() }()

	// Logs are multiplexed into one stream; see docs for ContainerLogs.
	_, err = stdcopy.StdCopy(stdoutBuf, stderrBuf, rc)
	if err != nil {
		return ExecResult{
			Err:      err,
			ExitCode: exitCode,
			Stdout:   nil,
			Stderr:   nil,
		}
	}
	_ = rc.Close()

	err = c.Stop(10 * time.Second)
	if err != nil {
		c.log.Error("Failed to stop and remove container", zap.Error(err), zap.String("container_id", c.containerID))
	}

	if exitCode != 0 {
		out := strings.Join([]string{stdoutBuf.String(), stderrBuf.String()}, " ")
		return ExecResult{
			Err:      fmt.Errorf("exit code %d: %s", exitCode, out),
			ExitCode: exitCode,
			Stdout:   nil,
			Stderr:   nil,
		}
	}

	return ExecResult{
		Err:      nil,
		ExitCode: exitCode,
		Stdout:   stdoutBuf.Bytes(),
		Stderr:   stderrBuf.Bytes(),
	}
}

// Stop gives the container up to timeout to stop and remove itself from the network.
func (c *Container) Stop(timeout time.Duration) error {
	// Use timeout*2 to give both stop and remove container operations a chance to complete.
	ctx, cancel := context.WithTimeout(context.Background(), timeout*2)
	defer cancel()

	var stopOptions container.StopOptions
	timeoutRound := int(timeout.Round(time.Second))
	stopOptions.Timeout = &timeoutRound
	err := c.job.client.ContainerStop(ctx, c.containerID, stopOptions)
	if err != nil {
		// Only return the error if it didn't match an already stopped, or a missing container.
		if !errdefs.IsNotModified(err) && !errdefs.IsNotFound(err) {
			return c.job.WrapErr(fmt.Errorf("stop container %s: %w", c.Name, err))
		}
	}

	// RemoveContainerOptions duplicates (*dockertest.Resource).Prune.
	err = c.job.client.ContainerRemove(ctx, c.containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if err != nil && !errdefs.IsNotFound(err) {
		return c.job.WrapErr(fmt.Errorf("remove container %s: %w", c.Name, err))
	}

	return nil
}
