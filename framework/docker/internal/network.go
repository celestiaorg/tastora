package internal

import (
	"context"
	"fmt"
	"strings"

	dockerclient "github.com/moby/moby/client"
)

// GetContainerInternalIP returns the internal IP address of a container within the docker network.
// Returns empty string if container is not yet networked (no error).
func GetContainerInternalIP(ctx context.Context, client dockerclient.APIClient, containerID string) (string, error) {
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
func ExtractPort(address string) string {
	if idx := strings.LastIndex(address, ":"); idx != -1 {
		return address[idx+1:]
	}
	return address
}