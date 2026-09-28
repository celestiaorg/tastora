package docker

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

func TestConcurrentDockerPortAllocation(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Docker")
	}

	const (
		containerCount    = 16
		portsPerContainer = 8
		servicePort       = 8080
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cli, networkID := Setup(t)
	image := container.Image{Repository: "busybox", Version: "stable"}
	require.NoError(t, image.PullImage(ctx, cli))

	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = occupied.Close() }()
	occupiedPort := occupied.Addr().(*net.TCPAddr).Port

	portMap := make(network.PortMap, portsPerContainer)
	portIDs := make([]string, portsPerContainer)
	for i := range portIDs {
		portIDs[i] = fmt.Sprintf("%d/tcp", servicePort+i)
		portMap[network.MustParsePort(portIDs[i])] = []network.PortBinding{}
	}

	lifecycles := make([]*container.Lifecycle, containerCount)
	addresses := make([][]string, containerCount)
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
					t.Errorf("remove port test container: %v", err)
				}
			}()
		}
		wg.Wait()
	})

	group, groupCtx := errgroup.WithContext(ctx)
	for i := range lifecycles {
		i := i
		group.Go(func() error {
			lifecycle := container.NewLifecycle(zap.NewNop(), cli, fmt.Sprintf("port-test-%d-%d", time.Now().UnixNano(), i))
			lifecycles[i] = lifecycle
			if err := lifecycle.CreateContainer(groupCtx, t.Name(), networkID, image, portMap, "", nil, nil, "", []string{"httpd", "-f", "-p", fmt.Sprint(servicePort)}, nil, nil); err != nil {
				return fmt.Errorf("container %d create: %w", i, err)
			}
			if err := lifecycle.StartContainer(groupCtx); err != nil {
				return fmt.Errorf("container %d start: %w", i, err)
			}
			mapped, err := lifecycle.GetHostPorts(groupCtx, portIDs...)
			if err != nil {
				return fmt.Errorf("container %d ports: %w", i, err)
			}
			addresses[i] = mapped
			return nil
		})
	}
	require.NoError(t, group.Wait())

	seen := make(map[string]struct{}, containerCount*portsPerContainer)
	for i, mapped := range addresses {
		require.Len(t, mapped, portsPerContainer)
		inspect, err := cli.ContainerInspect(ctx, lifecycles[i].ContainerID(), client.ContainerInspectOptions{})
		require.NoError(t, err)
		require.Len(t, inspect.Container.NetworkSettings.Ports, portsPerContainer, "container %d has unexpected published ports", i)
		for j, address := range mapped {
			host, port, err := net.SplitHostPort(address)
			require.NoError(t, err, "container %d port %d", i, j)
			require.Equal(t, "127.0.0.1", host)
			require.NotEmpty(t, port)
			require.NotEqual(t, fmt.Sprint(occupiedPort), port)
			_, exists := seen[address]
			require.False(t, exists, "duplicate host binding %s", address)
			seen[address] = struct{}{}

			bindings := inspect.Container.NetworkSettings.Ports[network.MustParsePort(portIDs[j])]
			require.Len(t, bindings, 1, "container %d port %s has unexpected IPv6 binding", i, portIDs[j])
			require.Equal(t, "127.0.0.1", bindings[0].HostIP.String())
		}
		conn, err := net.DialTimeout("tcp", mapped[0], 3*time.Second)
		require.NoError(t, err, "container %d service port is unreachable", i)
		require.NoError(t, conn.Close())
	}
	require.Len(t, seen, containerCount*portsPerContainer)
}
