package container

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/celestiaorg/tastora/framework/types"
	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type createClient struct {
	types.TastoraDockerClient
	create       func(client.ContainerCreateOptions) (client.ContainerCreateResult, error)
	exposedPorts []string
}

func (c createClient) CleanupLabel() string { return "port-test" }

func (c createClient) ImageInspect(context.Context, string, ...client.ImageInspectOption) (client.ImageInspectResult, error) {
	result := client.ImageInspectResult{}
	if len(c.exposedPorts) > 0 {
		result.Config = &dockerspec.DockerOCIImageConfig{}
		result.Config.ExposedPorts = make(map[string]struct{}, len(c.exposedPorts))
		for _, port := range c.exposedPorts {
			result.Config.ExposedPorts[port] = struct{}{}
		}
	}
	return result, nil
}

func (c createClient) ContainerCreate(_ context.Context, opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
	return c.create(opts)
}

func TestLifecycleCreateContainerDynamicPorts(t *testing.T) {
	rpc := network.MustParsePort("26657/tcp")
	grpc := network.MustParsePort("9090/tcp")
	imagePort := network.MustParsePort("8888/tcp")
	ports := network.PortMap{
		rpc:  {{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: "40000"}},
		grpc: {},
	}
	called := false
	cli := createClient{exposedPorts: []string{"8888/tcp"}, create: func(opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
		called = true
		require.Equal(t, "validator", opts.Name)
		require.Len(t, opts.Config.ExposedPorts, len(ports)+1)
		require.Len(t, opts.HostConfig.PortBindings, len(ports)+1)
		require.False(t, opts.HostConfig.PublishAllPorts)
		for _, port := range []network.Port{rpc, grpc, imagePort} {
			_, exposed := opts.Config.ExposedPorts[port]
			require.True(t, exposed)
			bindings := opts.HostConfig.PortBindings[port]
			require.Len(t, bindings, 1)
			require.Equal(t, netip.MustParseAddr("127.0.0.1"), bindings[0].HostIP)
			require.Empty(t, bindings[0].HostPort)
		}
		return client.ContainerCreateResult{ID: "validator-id"}, nil
	}}
	lifecycle := NewLifecycle(zap.NewNop(), cli, "validator")
	err := lifecycle.CreateContainer(context.Background(), "test", "network-id", Image{Repository: "busybox", Version: "stable"}, ports, "", nil, nil, "", nil, nil, nil)
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "validator-id", lifecycle.ContainerID())
	require.Equal(t, "40000", ports[rpc][0].HostPort, "caller port map must remain unchanged")
}

func TestLifecycleCreateContainerHostNetwork(t *testing.T) {
	port := network.MustParsePort("8080/tcp")
	cli := createClient{create: func(opts client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
		require.Equal(t, "host", string(opts.HostConfig.NetworkMode))
		require.Empty(t, opts.HostConfig.PortBindings)
		require.Empty(t, opts.Config.ExposedPorts)
		return client.ContainerCreateResult{ID: "host-id"}, nil
	}}
	lifecycle := NewLifecycle(zap.NewNop(), cli, "host-container")
	lifecycle.SetHostNetwork(true)
	err := lifecycle.CreateContainer(context.Background(), "test", "", Image{Repository: "busybox", Version: "stable"}, network.PortMap{port: {}}, "", nil, nil, "", nil, nil, nil)
	require.NoError(t, err)
}

func TestLifecycleCreateContainerError(t *testing.T) {
	wantErr := errors.New("docker create failed")
	cli := createClient{create: func(client.ContainerCreateOptions) (client.ContainerCreateResult, error) {
		return client.ContainerCreateResult{}, wantErr
	}}
	lifecycle := NewLifecycle(zap.NewNop(), cli, "validator")
	err := lifecycle.CreateContainer(context.Background(), "test", "network-id", Image{Repository: "busybox", Version: "stable"}, network.PortMap{}, "", nil, nil, "", nil, nil, nil)
	require.ErrorIs(t, err, wantErr)
}
