package docker

import (
	"testing"

	"github.com/celestiaorg/tastora/framework/testutil/random"
	"github.com/docker/docker/api/types/container"

	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"
)

func TestGetHostPort(t *testing.T) {
	for _, tt := range []struct {
		Container container.InspectResponse
		PortID    string
		Want      string
	}{
		{
			container.InspectResponse{
				NetworkSettings: &container.NetworkSettings{
					NetworkSettingsBase: container.NetworkSettingsBase{
						Ports: nat.PortMap{
							nat.Port("test"): []nat.PortBinding{
								{HostIP: "1.2.3.4", HostPort: "8080"},
								{HostIP: "0.0.0.0", HostPort: "9999"},
							},
						},
					},
				},
			}, "test", "1.2.3.4:8080",
		},
		{
			container.InspectResponse{
				NetworkSettings: &container.NetworkSettings{
					NetworkSettingsBase: container.NetworkSettingsBase{
						Ports: nat.PortMap{
							nat.Port("test"): []nat.PortBinding{
								{HostIP: "0.0.0.0", HostPort: "3000"},
							},
						},
					},
				},
			}, "test", "0.0.0.0:3000",
		},

		{container.InspectResponse{}, "", ""},
		{container.InspectResponse{NetworkSettings: &container.NetworkSettings{}}, "does-not-matter", ""},
	} {
		require.Equal(t, tt.Want, GetHostPort(tt.Container, tt.PortID), tt)
	}
}

func TestRandLowerCaseLetterString(t *testing.T) {
	require.Empty(t, random.LowerCaseLetterString(0))

	// Test that the function produces strings of correct length
	result1 := random.LowerCaseLetterString(12)
	require.Len(t, result1, 12, "Result should have correct length")

	result2 := random.LowerCaseLetterString(30)
	require.Len(t, result2, 30, "Result should have correct length")

	// Verify all characters are lowercase letters
	for _, char := range result1 {
		require.True(t, char >= 'a' && char <= 'z', "All characters should be lowercase letters")
	}

	for _, char := range result2 {
		require.True(t, char >= 'a' && char <= 'z', "All characters should be lowercase letters")
	}

	// Function is working correctly if we reach here
}

func TestCondenseHostName(t *testing.T) {
	for _, tt := range []struct {
		HostName, Want string
	}{
		{"", ""},
		{"test", "test"},
		{"some-really-very-incredibly-long-hostname-that-is-greater-than-64-characters", "some-really-very-incredibly-lo_._-is-greater-than-64-characters"},
	} {
		require.Equal(t, tt.Want, CondenseHostName(tt.HostName), tt)
	}
}

func TestSanitizeContainerName(t *testing.T) {
	for _, tt := range []struct {
		Name, Want string
	}{
		{"hello-there", "hello-there"},
		{"hello@there", "hello_there"},
		{"hello@/there", "hello__there"},
		// edge cases
		{"?", "_"},
		{"", ""},
	} {
		require.Equal(t, tt.Want, SanitizeContainerName(tt.Name), tt)
	}
}
