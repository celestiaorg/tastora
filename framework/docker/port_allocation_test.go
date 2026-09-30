package docker

import (
	"context"
	"flag"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var portTestSequence atomic.Uint64

func TestConcurrentDockerPortAllocation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker")
	}
	cli, networkID := Setup(t)
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = occupied.Close() }()

	addresses := startPortContainers(t, cli, networkID, 16, 8)
	t.Cleanup(func() { removePortContainers(t, addresses.containers) })
	checkPortAddresses(t, addresses.ports, 128, occupied.Addr().String())
}

func TestParallelDockerPortAllocationSuites(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker")
	}
	parallel, err := strconv.Atoi(flag.Lookup("test.parallel").Value.String())
	require.NoError(t, err)
	if parallel < 2 {
		t.Skip("requires go test -parallel=2 or greater")
	}

	for _, independent := range []bool{false, true} {
		name := "shared client"
		if independent {
			name = "independent clients"
		}
		t.Run(name, func(t *testing.T) {
			clients := make([]types.TastoraDockerClient, 2)
			networks := make([]string, 2)
			clients[0], networks[0] = Setup(t)
			if independent {
				clients[1], networks[1] = Setup(t)
			} else {
				clients[1], networks[1] = clients[0], networks[0]
			}

			results := make(chan portRun, 2)
			t.Cleanup(func() {
				var addresses []string
				var containers []*container.Lifecycle
				for range 2 {
					run := <-results
					addresses = append(addresses, run.ports...)
					containers = append(containers, run.containers...)
				}
				defer removePortContainers(t, containers)
				checkPortAddresses(t, addresses, 32, "")
			})
			for i := range clients {
				t.Run(fmt.Sprintf("suite-%d", i), func(t *testing.T) {
					t.Parallel()
					var run portRun
					defer func() { results <- run }()
					run = startPortContainers(t, clients[i], networks[i], 4, 4)
				})
			}
		})
	}
}

type portRun struct {
	ports      []string
	containers []*container.Lifecycle
}

func startPortContainers(t *testing.T, cli types.TastoraDockerClient, networkID string, count, portCount int) portRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	image := container.Image{Repository: "busybox", Version: "stable"}
	require.NoError(t, image.PullImage(ctx, cli))

	ports := make(network.PortMap, portCount)
	portIDs := make([]string, portCount)
	for i := range portIDs {
		portIDs[i] = fmt.Sprintf("%d/tcp", 8080+i)
		ports[network.MustParsePort(portIDs[i])] = nil
	}

	lifecycles := make([]*container.Lifecycle, count)
	group, groupCtx := errgroup.WithContext(ctx)
	for i := range lifecycles {
		i := i
		group.Go(func() error {
			lifecycle := container.NewLifecycle(zap.NewNop(), cli, fmt.Sprintf("port-test-%d-%d", time.Now().UnixNano(), portTestSequence.Add(1)))
			lifecycles[i] = lifecycle
			if err := lifecycle.CreateContainer(groupCtx, t.Name(), networkID, image, ports, "", nil, nil, "", []string{"httpd", "-f", "-p", "8080"}, nil, nil); err != nil {
				return fmt.Errorf("container %d create: %w", i, err)
			}
			if err := lifecycle.StartContainer(groupCtx); err != nil {
				return fmt.Errorf("container %d start: %w", i, err)
			}
			return nil
		})
	}
	require.NoError(t, group.Wait())

	addresses := make([]string, 0, count*portCount)
	for _, lifecycle := range lifecycles {
		mapped, err := lifecycle.GetHostPorts(ctx, portIDs...)
		require.NoError(t, err)
		require.Len(t, mapped, portCount)
		inspect, err := cli.ContainerInspect(ctx, lifecycle.ContainerID(), client.ContainerInspectOptions{})
		require.NoError(t, err)
		require.Len(t, inspect.Container.NetworkSettings.Ports, portCount)
		for _, port := range portIDs {
			binding := inspect.Container.NetworkSettings.Ports[network.MustParsePort(port)]
			require.Len(t, binding, 1)
			require.Equal(t, "127.0.0.1", binding[0].HostIP.String())
		}
		conn, err := net.DialTimeout("tcp", mapped[0], 3*time.Second)
		require.NoError(t, err)
		require.NoError(t, conn.Close())
		addresses = append(addresses, mapped...)
	}
	return portRun{addresses, lifecycles}
}

func removePortContainers(t *testing.T, containers []*container.Lifecycle) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, lifecycle := range containers {
		if lifecycle == nil || lifecycle.ContainerID() == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := lifecycle.RemoveContainer(ctx); err != nil {
				t.Errorf("remove port test container: %v", err)
			}
		}()
	}
	wg.Wait()
}

func checkPortAddresses(t *testing.T, addresses []string, want int, occupied string) {
	t.Helper()
	require.Len(t, addresses, want)
	seen := make(map[string]bool, want)
	for _, address := range addresses {
		host, port, err := net.SplitHostPort(address)
		require.NoError(t, err)
		require.Equal(t, "127.0.0.1", host)
		require.NotEmpty(t, port)
		require.NotEqual(t, occupied, address)
		require.False(t, seen[address], "duplicate host binding %s", address)
		seen[address] = true
	}
}

func TestDockerPortsStableAcrossRestart(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker")
	}
	cli, networkID := Setup(t)
	run := startPortContainers(t, cli, networkID, 1, 2)
	t.Cleanup(func() { removePortContainers(t, run.containers) })
	lifecycle := run.containers[0]

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for i := range 2 {
		require.NoError(t, lifecycle.StopContainer(ctx))
		require.NoError(t, lifecycle.StartContainer(ctx), "restart %d", i)
		mapped, err := lifecycle.GetHostPorts(ctx, "8080/tcp", "8081/tcp")
		require.NoError(t, err)
		require.Equal(t, run.ports, mapped, "host ports changed after restart %d", i)
		conn, err := net.DialTimeout("tcp", mapped[0], 3*time.Second)
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	}
}
