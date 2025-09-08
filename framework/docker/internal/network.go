package internal

import (
	"context"
	"fmt"

	dockerclient "github.com/moby/moby/client"
)

// GetContainerInternalIP returns the internal IP address of a container within the docker network.
func GetContainerInternalIP(ctx context.Context, client dockerclient.APIClient, containerID string) (string, error) {
	inspect, err := client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("inspecting container: %w", err)
	}
	if inspect.NetworkSettings == nil {
		return "", fmt.Errorf("container network settings not available")
	}
	networks := inspect.NetworkSettings.Networks
	if networks == nil {
		return "", fmt.Errorf("container networks not available")
	}
	
	for _, network := range networks {
		if network.IPAddress != "" {
			return network.IPAddress, nil
		}
	}
	return "", fmt.Errorf("no IP address found for container")
}