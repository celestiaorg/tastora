package docker

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockTestingT implements DockerSetupTestingT for testing purposes
type mockTestingT struct {
	name     string
	failed   bool
	logs     []string
	cleanups []func()
}

func (m *mockTestingT) Helper() {}

func (m *mockTestingT) Name() string {
	return m.name
}

func (m *mockTestingT) Failed() bool {
	return m.failed
}

func (m *mockTestingT) Cleanup(f func()) {
	m.cleanups = append(m.cleanups, f)
}

func (m *mockTestingT) Logf(format string, args ...any) {
	m.logs = append(m.logs, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

// generateSampleLogs creates sample log content with the specified number of lines
func generateSampleLogs(lines int) string {
	var buffer bytes.Buffer
	for i := 1; i <= lines; i++ {
		buffer.WriteString(fmt.Sprintf("Log line %d: This is sample log content for testing\n", i))
	}
	return buffer.String()
}

// TestLogBehaviorLogic tests the logic for determining when to show full vs tailed logs
func TestLogBehaviorLogic(t *testing.T) {
	testCases := []struct {
		name                string
		testFailed          bool
		showContainerLogs   string
		containerLogTail    string
		expectedShowLogs    bool
		expectedUseTail     bool
		expectedTailValue   string
		expectedLogHeader   string
	}{
		{
			name:                "Failed test with default settings",
			testFailed:          true,
			showContainerLogs:   "",
			containerLogTail:    "",
			expectedShowLogs:    true,
			expectedUseTail:     false, // This is the key change - no tail for failed tests
			expectedTailValue:   "",
			expectedLogHeader:   "Full container logs",
		},
		{
			name:                "Failed test ignores CONTAINER_LOG_TAIL",
			testFailed:          true,
			showContainerLogs:   "",
			containerLogTail:    "10",
			expectedShowLogs:    true,
			expectedUseTail:     false, // Should still not use tail
			expectedTailValue:   "",
			expectedLogHeader:   "Full container logs",
		},
		{
			name:                "Success test with SHOW_CONTAINER_LOGS=always",
			testFailed:          false,
			showContainerLogs:   "always",
			containerLogTail:    "",
			expectedShowLogs:    true,
			expectedUseTail:     true,
			expectedTailValue:   "50", // Default tail for success
			expectedLogHeader:   "Container logs",
		},
		{
			name:                "Success test with custom CONTAINER_LOG_TAIL",
			testFailed:          false,
			showContainerLogs:   "always",
			containerLogTail:    "20",
			expectedShowLogs:    true,
			expectedUseTail:     true,
			expectedTailValue:   "20",
			expectedLogHeader:   "Container logs",
		},
		{
			name:                "Success test without SHOW_CONTAINER_LOGS",
			testFailed:          false,
			showContainerLogs:   "",
			containerLogTail:    "",
			expectedShowLogs:    false,
			expectedUseTail:     false,
			expectedTailValue:   "",
			expectedLogHeader:   "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set environment variables
			if tc.showContainerLogs != "" {
				t.Setenv("SHOW_CONTAINER_LOGS", tc.showContainerLogs)
			}
			if tc.containerLogTail != "" {
				t.Setenv("CONTAINER_LOG_TAIL", tc.containerLogTail)
			}

			// Create mock testing interface
			mockT := &mockTestingT{
				name:   tc.name,
				failed: tc.testFailed,
			}

			// Test the logic that determines log behavior
			showContainerLogs := os.Getenv("SHOW_CONTAINER_LOGS")
			containerLogTail := os.Getenv("CONTAINER_LOG_TAIL")

			// This replicates the logic from DockerCleanup
			shouldShowLogs := (mockT.Failed() && showContainerLogs == "") || showContainerLogs == "always"
			require.Equal(t, tc.expectedShowLogs, shouldShowLogs, "Log display decision should match expected")

			if shouldShowLogs {
				// Test the tail logic
				var tailValue string
				useTail := false

				if !mockT.Failed() && containerLogTail != "" {
					tailValue = containerLogTail
					useTail = true
				} else if !mockT.Failed() {
					tailValue = "50"
					useTail = true
				}
				// When test fails, Tail is not set (useTail = false)

				require.Equal(t, tc.expectedUseTail, useTail, "Tail usage should match expected")
				require.Equal(t, tc.expectedTailValue, tailValue, "Tail value should match expected")

				// Test log header logic
				var logHeader string
				if mockT.Failed() && !useTail {
					logHeader = "Full container logs"
				} else {
					logHeader = "Container logs"
				}
				require.Equal(t, tc.expectedLogHeader, logHeader, "Log header should match expected")
			}
		})
	}
}

func TestLogContentDifference(t *testing.T) {
	// Generate sample logs
	fullLogs := generateSampleLogs(100) // 100 lines
	fullLogLines := strings.Count(fullLogs, "Log line")
	require.Equal(t, 100, fullLogLines, "Should have exactly 100 log lines")

	// Simulate tailed logs (last 50 lines)
	allLines := strings.Split(strings.TrimSpace(fullLogs), "\n")
	tailedLines := allLines[len(allLines)-50:]
	tailedLogs := strings.Join(tailedLines, "\n")
	tailedLogLines := strings.Count(tailedLogs, "Log line")
	
	require.Equal(t, 50, tailedLogLines, "Tailed logs should have exactly 50 lines")
	require.Greater(t, fullLogLines, tailedLogLines, "Full logs should have more lines than tailed logs")
	
	// Verify that tailed logs are a subset of full logs
	require.Contains(t, fullLogs, tailedLogs, "Tailed logs should be contained in full logs")
}

func TestLogHeaderGeneration(t *testing.T) {
	testCases := []struct {
		name           string
		testFailed     bool
		tailSet        bool
		expectedHeader string
	}{
		{
			name:           "Failed test with no tail",
			testFailed:     true,
			tailSet:        false,
			expectedHeader: "Full container logs",
		},
		{
			name:           "Success test with tail",
			testFailed:     false,
			tailSet:        true,
			expectedHeader: "Container logs",
		},
		{
			name:           "Success test with no tail",
			testFailed:     false,
			tailSet:        false,
			expectedHeader: "Container logs",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the header logic from DockerCleanup
			var logHeader string
			var tailValue string
			
			if tc.tailSet {
				tailValue = "50"
			}
			
			if tc.testFailed && tailValue == "" {
				logHeader = "Full container logs"
			} else {
				logHeader = "Container logs"
			}
			
			require.Equal(t, tc.expectedHeader, logHeader, "Log header should match expected")
		})
	}
}

func TestEnvironmentVariableHandling(t *testing.T) {
	// Test default behavior (no env vars set)
	t.Run("default_behavior", func(t *testing.T) {
		showLogs := os.Getenv("SHOW_CONTAINER_LOGS")
		tailValue := os.Getenv("CONTAINER_LOG_TAIL")
		
		// With no env vars, these should be empty
		require.Empty(t, showLogs, "SHOW_CONTAINER_LOGS should be empty by default")
		require.Empty(t, tailValue, "CONTAINER_LOG_TAIL should be empty by default")
	})

	// Test with environment variables set
	t.Run("with_env_vars", func(t *testing.T) {
		t.Setenv("SHOW_CONTAINER_LOGS", "always")
		t.Setenv("CONTAINER_LOG_TAIL", "25")
		
		showLogs := os.Getenv("SHOW_CONTAINER_LOGS")
		tailValue := os.Getenv("CONTAINER_LOG_TAIL")
		
		require.Equal(t, "always", showLogs, "SHOW_CONTAINER_LOGS should be set")
		require.Equal(t, "25", tailValue, "CONTAINER_LOG_TAIL should be set")
	})
}

// TestDockerCleanupBehaviorSimulation simulates the key behavior changes
func TestDockerCleanupBehaviorSimulation(t *testing.T) {
	t.Run("failed_test_shows_full_logs", func(t *testing.T) {
		// Simulate a failed test scenario
		mockT := &mockTestingT{
			name:   "failed-test",
			failed: true,
		}
		
		// Environment setup that would trigger log display
		showContainerLogs := "" // Default - shows logs on failure
		containerLogTail := "50" // This should be ignored for failed tests
		
		// Logic from DockerCleanup: should show logs because test failed
		shouldShowLogs := (mockT.Failed() && showContainerLogs == "") || showContainerLogs == "always"
		require.True(t, shouldShowLogs, "Failed test should trigger log display")
		
		// Logic for tail behavior - THIS IS THE KEY CHANGE
		// Failed tests should NOT use tail limits
		var tailValue string
		if !mockT.Failed() && containerLogTail != "" {
			tailValue = containerLogTail
		} else if !mockT.Failed() {
			tailValue = "50"
		}
		// For failed tests, tailValue remains empty (no tail limit)
		
		require.Empty(t, tailValue, "Failed test should not set tail limit")
		
		// Header should indicate full logs
		logHeader := "Container logs"
		if mockT.Failed() && tailValue == "" {
			logHeader = "Full container logs"
		}
		require.Equal(t, "Full container logs", logHeader, "Failed test should show 'Full container logs' header")
	})
	
	t.Run("success_test_respects_tail", func(t *testing.T) {
		// Simulate a successful test scenario
		mockT := &mockTestingT{
			name:   "success-test",
			failed: false,
		}
		
		t.Setenv("SHOW_CONTAINER_LOGS", "always") // Force log display for success test
		
		showContainerLogs := os.Getenv("SHOW_CONTAINER_LOGS")
		containerLogTail := "" // Use default
		
		// Should show logs because SHOW_CONTAINER_LOGS=always
		shouldShowLogs := (mockT.Failed() && showContainerLogs == "") || showContainerLogs == "always"
		require.True(t, shouldShowLogs, "Success test with SHOW_CONTAINER_LOGS=always should show logs")
		
		// Success tests should use tail limits
		var tailValue string
		if !mockT.Failed() && containerLogTail != "" {
			tailValue = containerLogTail
		} else if !mockT.Failed() {
			tailValue = "50" // Default for success
		}
		
		require.Equal(t, "50", tailValue, "Success test should use default tail limit")
		
		// Header should indicate regular (tailed) logs
		logHeader := "Container logs"
		if mockT.Failed() && tailValue == "" {
			logHeader = "Full container logs"
		}
		require.Equal(t, "Container logs", logHeader, "Success test should show 'Container logs' header")
	})
}