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
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var portSuiteContainerSequence atomic.Uint64

func TestParallelDockerPortAllocationSuites(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker")
	}
	parallelLimit, err := strconv.Atoi(flag.Lookup("test.parallel").Value.String())
	require.NoError(t, err)
	if parallelLimit < 2 {
		t.Skip("requires go test -parallel=2 or greater")
	}

	t.Run("shared client", func(t *testing.T) {
		cli, networkID := Setup(t)
		runParallelPortSuites(t, 2, func(*testing.T) (types.TastoraDockerClient, string) {
			return cli, networkID
		})
	})

	t.Run("independent clients", func(t *testing.T) {
		runParallelPortSuites(t, 2, func(t *testing.T) (types.TastoraDockerClient, string) {
			return Setup(t)
		})
	})
}

func runParallelPortSuites(t *testing.T, suiteCount int, clientForSuite func(*testing.T) (types.TastoraDockerClient, string)) {
	t.Helper()
	const (
		containersPerSuite = 4
		portsPerContainer  = 4
	)

	var mu sync.Mutex
	seen := make(map[string]struct{}, suiteCount*containersPerSuite*portsPerContainer)
	ready := make(chan struct{}, suiteCount)
	release := make(chan struct{})
	finished := make(chan struct{}, suiteCount)
	allFinished := make(chan struct{})
	go func() {
		for range suiteCount {
			<-ready
		}
		close(release)
	}()
	go func() {
		for range suiteCount {
			<-finished
		}
		close(allFinished)
	}()

	for i := range suiteCount {
		t.Run(fmt.Sprintf("suite-%d", i), func(t *testing.T) {
			t.Parallel()
			ready <- struct{}{}
			<-release
			// Keep every suite's containers running until the other suites have
			// finished checking their ports, so reused ports cannot mask a collision.
			defer func() {
				finished <- struct{}{}
				<-allFinished
			}()

			cli, networkID := clientForSuite(t)
			addresses := startPortSuite(t, cli, networkID, containersPerSuite, portsPerContainer)
			for _, address := range addresses {
				mu.Lock()
				_, duplicate := seen[address]
				seen[address] = struct{}{}
				mu.Unlock()
				require.False(t, duplicate, "host port %s was assigned to multiple suites", address)
			}
		})
	}
	t.Cleanup(func() {
		require.Len(t, seen, suiteCount*containersPerSuite*portsPerContainer)
	})
}

func startPortSuite(t *testing.T, cli types.TastoraDockerClient, networkID string, containerCount, portCount int) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	image := container.Image{Repository: "busybox", Version: "stable"}
	require.NoError(t, image.PullImage(ctx, cli))

	ports := make(network.PortMap, portCount)
	portIDs := make([]string, portCount)
	for i := range portIDs {
		portIDs[i] = fmt.Sprintf("%d/tcp", 8080+i)
		ports[network.MustParsePort(portIDs[i])] = []network.PortBinding{}
	}

	lifecycles := make([]*container.Lifecycle, containerCount)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		var wg sync.WaitGroup
		for _, lifecycle := range lifecycles {
			if lifecycle == nil || lifecycle.ContainerID() == "" {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := lifecycle.RemoveContainer(cleanupCtx); err != nil {
					t.Errorf("remove port suite container: %v", err)
				}
			}()
		}
		wg.Wait()
	})

	group, groupCtx := errgroup.WithContext(ctx)
	for i := range lifecycles {
		i := i
		group.Go(func() error {
			lifecycle := container.NewLifecycle(zap.NewNop(), cli, fmt.Sprintf("port-suite-%d-%d", time.Now().UnixNano(), portSuiteContainerSequence.Add(1)))
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

	addresses := make([]string, 0, containerCount*portCount)
	for i, lifecycle := range lifecycles {
		mapped, err := lifecycle.GetHostPorts(ctx, portIDs...)
		require.NoError(t, err)
		require.Len(t, mapped, portCount)
		for _, address := range mapped {
			host, port, err := net.SplitHostPort(address)
			require.NoError(t, err)
			require.Equal(t, "127.0.0.1", host)
			require.NotEmpty(t, port)
			addresses = append(addresses, address)
		}
		conn, err := net.DialTimeout("tcp", mapped[0], 3*time.Second)
		require.NoError(t, err, "container %d service port is unreachable", i)
		require.NoError(t, conn.Close())
	}
	return addresses
}
