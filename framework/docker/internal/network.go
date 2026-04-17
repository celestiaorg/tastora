package internal

import (
	"context"
	"fmt"
	"net"

	"github.com/celestiaorg/tastora/framework/types"
	dockerclient "github.com/moby/moby/client"
)

// GetContainerInternalIP returns the internal IP address of a container within the docker network.
// Returns empty string if container is not yet networked (no error).
func GetContainerInternalIP(ctx context.Context, client types.TastoraDockerClient, containerID string) (string, error) {
	inspectResult, err := client.ContainerInspect(ctx, containerID, dockerclient.ContainerInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("inspecting container: %w", err)
	}
	inspect := inspectResult.Container
	if inspect.NetworkSettings == nil {
		return "", nil
	}
	networks := inspect.NetworkSettings.Networks
	if networks == nil {
		return "", nil
	}

	for _, network := range networks {
		if network.IPAddress.IsValid() {
			return network.IPAddress.String(), nil
		}
	}
	return "", nil
}

// ExtractPort extracts the port number from a "host:port" address
func ExtractPort(address string) (string, error) {
	_, port, err := net.SplitHostPort(address)
	return port, err
}

// MustExtractPort extracts the port number from a "host:port" address.
// It panics if the address is not in the correct format.
func MustExtractPort(address string) string {
	port, err := ExtractPort(address)
	if err != nil {
		panic(err)
	}
	return port
}
