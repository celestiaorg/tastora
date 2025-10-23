package internal

import (
	"context"
	"fmt"
	dockerclient "github.com/moby/moby/client"
	"net"
)

// GetContainerInternalIP returns the internal IP address of a container within the docker network.
// Returns empty string if container is not yet networked (no error).
func GetContainerInternalIP(ctx context.Context, client dockerclient.CommonAPIClient, containerID string) (string, error) {
	inspect, err := client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspecting container: %w", err)
	}
	if inspect.NetworkSettings == nil {
		return "", nil
	}
	networks := inspect.NetworkSettings.Networks
	if networks == nil {
		return "", nil
	}

	for _, network := range networks {
		if network.IPAddress != "" {
			return network.IPAddress, nil
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
