package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/testutil/random"

	"github.com/avast/retry-go/v4"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/moby/moby/client"
	"github.com/moby/moby/errdefs"
)

// DockerSetupTestingT is a subset of testing.T required for DockerSetup.
type DockerSetupTestingT interface {
	Helper()

	Name() string

	Failed() bool
	Cleanup(func())

	Logf(format string, args ...any)
}

// KeepVolumesOnFailure determines whether volumes associated with a test
// using DockerSetup are retained or deleted following a test failure.
//
// The value is false by default, but can be initialized to true by setting the
// environment variable ICTEST_SKIP_FAILURE_CLEANUP to a non-empty value.
// Alternatively, importers of the dockerutil package may set the variable to true.
// Because dockerutil is an internal package, the public API for setting this value
// is interchaintest.KeepDockerVolumesOnFailure(bool).
var KeepVolumesOnFailure = os.Getenv("ICTEST_SKIP_FAILURE_CLEANUP") != ""

// DockerSetup returns a new Docker Client and the ID of a configured network, associated with t.
//
// If any part of the setup fails, DockerSetup panics because the test cannot continue.
func DockerSetup(t DockerSetupTestingT) (*LabeledClient, string) {
	t.Helper()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(fmt.Errorf("failed to create docker client: %v", err))
	}

	cleanupLabel := fmt.Sprintf("%s-%s", t.Name(), random.LowerCaseLetterString(8))
	labeledClient := NewLabeledClient(cli, cleanupLabel)

	name := fmt.Sprintf("%s-%s", consts.CelestiaDockerPrefix, random.LowerCaseLetterString(8))
	network, err := labeledClient.NetworkCreate(context.TODO(), name, network.CreateOptions{
		Driver: "bridge",
		IPAM:   &network.IPAM{},
		Labels: map[string]string{consts.CleanupLabel: cleanupLabel},
	})
	if err != nil {
		panic(fmt.Errorf("failed to create docker network: %v", err))
	}

	// Register cleanup using the LabeledClient's label to ensure consistency
	t.Cleanup(DockerCleanup(t, labeledClient))

	return labeledClient, network.ID
}

// DockerCleanup will clean up Docker containers, networks, and the other various config files generated in testing.
func DockerCleanup(t DockerSetupTestingT, cli client.CommonAPIClient) func() {
	if labeledClient, ok := cli.(*LabeledClient); ok {
		return DockerCleanupWithTestName(t, cli, labeledClient.CleanupLabel())
	}
	return DockerCleanupWithTestName(t, cli, t.Name())
}

// DockerCleanupWithTestName will clean up Docker containers, networks, and other config files using a custom test name.
func DockerCleanupWithTestName(t DockerSetupTestingT, cli client.CommonAPIClient, testName string) func() {
	return func() {
		showContainerLogs := os.Getenv("SHOW_CONTAINER_LOGS")
		containerLogTail := os.Getenv("CONTAINER_LOG_TAIL")
		keepContainers := os.Getenv("KEEP_CONTAINERS") != ""

		ctx := context.TODO()
		cli.NegotiateAPIVersion(ctx)
		cs, err := cli.ContainerList(ctx, container.ListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.Arg("label", consts.CleanupLabel+"="+testName),
			),
		})
		if err != nil {
			t.Logf("Failed to list containers during docker cleanup: %v", err)
			return
		}

		for _, c := range cs {
			if shouldShowContainerLogs(t.Failed(), showContainerLogs) {
				logOptions := configureLogOptions(t.Failed(), containerLogTail)
				displayContainerLogs(ctx, t, cli, c.ID, c.Names, logOptions)
			}
			if !keepContainers {
				var stopTimeout container.StopOptions
				timeout := 10
				timeoutDur := time.Duration(timeout * int(time.Second))
				deadline := time.Now().Add(timeoutDur)
				stopTimeout.Timeout = &timeout
				if err := cli.ContainerStop(ctx, c.ID, stopTimeout); IsLoggableStopError(err) {
					t.Logf("Failed to stop container %s during docker cleanup: %v", c.ID, err)
				}

				waitCtx, cancel := context.WithDeadline(ctx, deadline.Add(500*time.Millisecond))
				waitCh, errCh := cli.ContainerWait(waitCtx, c.ID, container.WaitConditionNotRunning)
				select {
				case <-waitCtx.Done():
					t.Logf("Timed out waiting for container %s", c.ID)
				case err := <-errCh:
					t.Logf("Failed to wait for container %s during docker cleanup: %v", c.ID, err)
				case res := <-waitCh:
					if res.Error != nil {
						t.Logf("Error while waiting for container %s during docker cleanup: %s", c.ID, res.Error.Message)
					}
					// Ignoring statuscode for now.
				}
				cancel()

				if err := cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{
					// Not removing volumes with the container, because we separately handle them conditionally.
					Force: true,
				}); err != nil {
					t.Logf("Failed to remove container %s during docker cleanup: %v", c.ID, err)
				}
			}
		}

		if !keepContainers {
			PruneVolumesWithRetryAndTestName(ctx, t, cli, testName)
			PruneNetworksWithRetryAndTestName(ctx, t, cli, testName)
		} else {
			t.Logf("Keeping containers - Docker cleanup skipped")
		}
	}
}

func PruneVolumesWithRetry(ctx context.Context, t DockerSetupTestingT, cli client.CommonAPIClient) {
	if labeledClient, ok := cli.(*LabeledClient); ok {
		PruneVolumesWithRetryAndTestName(ctx, t, cli, labeledClient.CleanupLabel())
		return
	}
	PruneVolumesWithRetryAndTestName(ctx, t, cli, t.Name())
}

func PruneVolumesWithRetryAndTestName(ctx context.Context, t DockerSetupTestingT, cli client.CommonAPIClient, testName string) {
	if KeepVolumesOnFailure && t.Failed() {
		return
	}

	var deletedCount int
	var spaceReclaimed uint64
	err := retry.Do(
		func() error {
			// List volumes with the cleanup label
			filterArgs := filters.NewArgs(filters.Arg("label", consts.CleanupLabel+"="+testName))
			volumeList, err := cli.VolumeList(ctx, volume.ListOptions{Filters: filterArgs})
			if err != nil {
				return retry.Unrecoverable(fmt.Errorf("listing volumes: %w", err))
			}

			// explicitly remove each volume.
			for _, vol := range volumeList.Volumes {
				// try to get volume size before removal
				if vol.UsageData != nil {
					spaceReclaimed += uint64(vol.UsageData.Size)
				}

				err := cli.VolumeRemove(ctx, vol.Name, true)
				if err != nil {
					if errdefs.IsConflict(err) {
						return err
					}
					// Log but continue with other volumes
					t.Logf("Failed to remove volume %s: %v", vol.Name, err)
				} else {
					deletedCount++
				}
			}

			return nil
		},
		retry.Context(ctx),
		retry.DelayType(retry.FixedDelay),
	)
	if err != nil {
		t.Logf("Failed to remove volumes during docker cleanup: %v", err)
		return
	}

	if deletedCount > 0 {
		msg := fmt.Sprintf("Removed %d volumes, reclaiming approximately %.1f MB", deletedCount, float64(spaceReclaimed)/(1024*1024))
		t.Logf("%s", msg)
	}
}

func PruneNetworksWithRetry(ctx context.Context, t DockerSetupTestingT, cli client.CommonAPIClient) {
	if labeledClient, ok := cli.(*LabeledClient); ok {
		PruneNetworksWithRetryAndTestName(ctx, t, cli, labeledClient.CleanupLabel())
		return
	}
	PruneNetworksWithRetryAndTestName(ctx, t, cli, t.Name())
}

func PruneNetworksWithRetryAndTestName(ctx context.Context, t DockerSetupTestingT, cli client.CommonAPIClient, testName string) {
	var deleted []string
	err := retry.Do(
		func() error {
			res, err := cli.NetworksPrune(ctx, filters.NewArgs(filters.Arg("label", consts.CleanupLabel+"="+testName)))
			if err != nil {
				if errdefs.IsConflict(err) {
					// Prune is already in progress; try again.
					return err
				}

				// Give up on any other error.
				return retry.Unrecoverable(err)
			}

			deleted = res.NetworksDeleted
			return nil
		},
		retry.Context(ctx),
		retry.DelayType(retry.FixedDelay),
	)
	if err != nil {
		t.Logf("Failed to prune networks during docker cleanup: %v", err)
		return
	}

	if len(deleted) > 0 {
		t.Logf("Pruned unused networks: %v", deleted)
	}
}

func IsLoggableStopError(err error) bool {
	if err == nil {
		return false
	}
	return !errdefs.IsNotModified(err) && !errdefs.IsNotFound(err)
}

// shouldShowContainerLogs determines if container logs should be displayed based on test status and environment variables
func shouldShowContainerLogs(testFailed bool, showContainerLogs string) bool {
	return (testFailed && showContainerLogs == "") || showContainerLogs == "always"
}

// configureLogOptions creates container log options based on test status and environment variables
func configureLogOptions(testFailed bool, containerLogTail string) container.LogsOptions {
	logOptions := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	}

	// Only apply tail limit if test hasn't failed
	if !testFailed {
		if containerLogTail != "" {
			logOptions.Tail = containerLogTail
		} else {
			logOptions.Tail = consts.DefaultLogTail
		}
	}
	// When test fails, Tail is not set, so full logs are returned

	return logOptions
}

// displayContainerLogs fetches and displays container logs
func displayContainerLogs(
	ctx context.Context,
	t DockerSetupTestingT,
	cli client.CommonAPIClient,
	containerID string,
	containerNames []string,
	logOptions container.LogsOptions,
) {
	rc, err := cli.ContainerLogs(ctx, containerID, logOptions)
	if err != nil {
		return
	}
	defer func() { _ = rc.Close() }()

	b := new(bytes.Buffer)
	_, err = b.ReadFrom(rc)
	if err != nil {
		return
	}

	logHeader := generateLogHeader(t.Failed(), logOptions.Tail)
	t.Logf("\n\n%s - {%s}\n%s", logHeader, strings.Join(containerNames, " "), b.String())
}

// generateLogHeader returns the appropriate log header based on test failure and tail settings
func generateLogHeader(testFailed bool, tail string) string {
	if testFailed && tail == "" {
		return "Full container logs"
	}
	return "Container logs"
}
