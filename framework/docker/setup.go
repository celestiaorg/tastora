package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/testutil/random"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/moby/moby/client"
	"github.com/moby/moby/errdefs"
)

// SetupTestingT is a subset of testing.T required for Setup.
type SetupTestingT interface {
	Helper()

	Name() string

	Failed() bool
	Cleanup(func())

	Logf(format string, args ...any)
}

// KeepVolumesOnFailure determines whether volumes associated with a test
// using Setup are retained or deleted following a test failure.
//
// The value is false by default, but can be initialized to true by setting the
// environment variable ICTEST_SKIP_FAILURE_CLEANUP to a non-empty value.
// Alternatively, importers of the dockerutil package may set the variable to true.
// Because dockerutil is an internal package, the public API for setting this value
// is interchaintest.KeepDockerVolumesOnFailure(bool).
var KeepVolumesOnFailure = os.Getenv("ICTEST_SKIP_FAILURE_CLEANUP") != ""

// Setup returns a new Docker Client and the ID of a configured network, associated with t.
//
// If any part of the setup fails, Setup panics because the test cannot continue.
func Setup(t SetupTestingT) (*client.Client, string) {
	t.Helper()

	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(fmt.Errorf("failed to create docker client: %v", err))
	}

	// Clean up docker resources at end of test.
	t.Cleanup(Cleanup(t, cli))

	// Also eagerly clean up any leftover resources from a previous test run,
	// e.g. if the test was interrupted.
	Cleanup(t, cli)()

	name := fmt.Sprintf("%s-%s", consts.CelestiaDockerPrefix, random.LowerCaseLetterString(8))
	octet := uint8(rand.Intn(256))
	baseSubnet := fmt.Sprintf("172.%d.0.0/16", octet)
	usedSubnets, err := getUsedSubnets(cli)
	if err != nil {
		panic(fmt.Errorf("failed to get used subnets: %v", err))
	}
	subnet, err := findAvailableSubnet(baseSubnet, usedSubnets)
	if err != nil {
		panic(fmt.Errorf("failed to find an available subnet: %v", err))
	}
	network, err := cli.NetworkCreate(context.TODO(), name, network.CreateOptions{
		Driver: "bridge",
		IPAM: &network.IPAM{
			Config: []network.IPAMConfig{
				{
					Subnet: subnet,
				},
			},
		},

		Labels: map[string]string{consts.CleanupLabel: t.Name()},
	})
	if err != nil {
		panic(fmt.Errorf("failed to create docker network: %v", err))
	}

	return cli, network.ID
}

func getUsedSubnets(cli *client.Client) (map[string]bool, error) {
	usedSubnets := make(map[string]bool)
	networks, err := cli.NetworkList(context.TODO(), network.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, net := range networks {
		for _, config := range net.IPAM.Config {
			if config.Subnet != "" {
				usedSubnets[config.Subnet] = true
			}
		}
	}
	return usedSubnets, nil
}

func findAvailableSubnet(baseSubnet string, usedSubnets map[string]bool) (string, error) {
	ip, ipNet, err := net.ParseCIDR(baseSubnet)
	if err != nil {
		return "", fmt.Errorf("invalid base subnet: %v", err)
	}

	for {
		if isSubnetUsed(ipNet.String(), usedSubnets) {
			incrementIP(ip, 2)
			ipNet.IP = ip
			continue
		}

		for subIP := ip.Mask(ipNet.Mask); ipNet.Contains(subIP); incrementIP(subIP, 1) {
			subnet := fmt.Sprintf("%s/24", subIP)

			if !isSubnetUsed(subnet, usedSubnets) {
				return subnet, nil
			}
		}

		incrementIP(ip, 2)
		ipNet.IP = ip
	}
}

func isSubnetUsed(subnet string, usedSubnets map[string]bool) bool {
	_, targetNet, err := net.ParseCIDR(subnet)
	if err != nil {
		return true
	}

	for usedSubnet := range usedSubnets {
		_, usedNet, err := net.ParseCIDR(usedSubnet)
		if err != nil {
			continue
		}

		if usedNet.Contains(targetNet.IP) || targetNet.Contains(usedNet.IP) {
			return true
		}
	}
	return false
}

func incrementIP(ip net.IP, incrementLevel int) {
	for j := len(ip) - incrementLevel; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// Cleanup will clean up Docker containers, networks, and the other various config files generated in testing.
func Cleanup(t SetupTestingT, cli *client.Client) func() {
	return func() {
		keepContainers := os.Getenv("KEEP_CONTAINERS") != ""
		logDir := os.Getenv("LOG_DIR")

		ctx := context.TODO()
		cli.NegotiateAPIVersion(ctx)
		cs, err := cli.ContainerList(ctx, container.ListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.Arg("label", consts.CleanupLabel+"="+t.Name()),
			),
		})
		if err != nil {
			t.Logf("Failed to list containers during docker cleanup: %v", err)
			return
		}

		for _, c := range cs {
			if t.Failed() {
				rc, err := cli.ContainerLogs(ctx, c.ID, container.LogsOptions{
					ShowStdout: true,
					ShowStderr: true,
					Tail:       "all",
				})
				if err == nil {
					containerName := strings.TrimPrefix(c.Names[0], "/")
					if err := writeToFile(rc, logDir, fmt.Sprintf("%s.log", containerName)); err != nil {
						t.Logf("Failed to write container logs to file during docker cleanup for container %s: %v", containerName, err)
					}
				}
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
			PruneVolumesWithRetry(ctx, t, cli)
			PruneNetworksWithRetry(ctx, t, cli)
		} else {
			t.Logf("Keeping containers - Docker cleanup skipped")
		}
	}
}

func PruneVolumesWithRetry(ctx context.Context, t SetupTestingT, cli *client.Client) {
	if KeepVolumesOnFailure && t.Failed() {
		return
	}

	var msg string
	err := retry.Do(
		func() error {
			res, err := cli.VolumesPrune(ctx, filters.NewArgs(filters.Arg("label", consts.CleanupLabel+"="+t.Name())))
			if err != nil {
				if errdefs.IsConflict(err) {
					// Prune is already in progress; try again.
					return err
				}

				// Give up on any other error.
				return retry.Unrecoverable(err)
			}

			if len(res.VolumesDeleted) > 0 {
				msg = fmt.Sprintf("Pruned %d volumes, reclaiming approximately %.1f MB", len(res.VolumesDeleted), float64(res.SpaceReclaimed)/(1024*1024))
			}

			return nil
		},
		retry.Context(ctx),
		retry.DelayType(retry.FixedDelay),
	)
	if err != nil {
		t.Logf("Failed to prune volumes during docker cleanup: %v", err)
		return
	}

	if msg != "" {
		// Odd to Logf %s, but this is a defensive way to keep the SetupTestingT interface
		// with only Logf and not need to add Log.
		t.Logf("%s", msg)
	}
}

func PruneNetworksWithRetry(ctx context.Context, t SetupTestingT, cli *client.Client) {
	var deleted []string
	err := retry.Do(
		func() error {
			res, err := cli.NetworksPrune(ctx, filters.NewArgs(filters.Arg("label", consts.CleanupLabel+"="+t.Name())))
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
	return !(errdefs.IsNotModified(err) || errdefs.IsNotFound(err))
}

// writeToFile writes the contents of an io.ReadCloser to a specified file in the given directory.
// It ensures the directory exists before creating and writing to the file.
// Returns an error if directory creation, file creation, or content copy fails.
func writeToFile(r io.ReadCloser, dir, filename string) error {
	defer r.Close()

	// ensure the directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// create the output file.
	outPath := filepath.Join(dir, filename)
	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// copy the contents
	_, err = io.Copy(outFile, r)
	return err
}
