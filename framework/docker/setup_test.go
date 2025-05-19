package docker

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteToFile(t *testing.T) {
	testCases := []struct {
		name       string
		content    string
		subDir     string
		filename   string
		tempPrefix string
	}{
		{
			name:       "flat directory",
			content:    "This is test content for writeToFile function",
			filename:   "test-file.txt",
			tempPrefix: "writetofile-test",
		},
		{
			name:       "nested directory",
			content:    "This is test content for nested directory",
			subDir:     filepath.Join("nested", "directory", "structure"),
			filename:   "nested-file.txt",
			tempPrefix: "writetofile-nested-test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// create a temporary directory for testing.
			tempDir, err := os.MkdirTemp("", tc.tempPrefix)
			require.NoError(t, err)
			defer os.RemoveAll(tempDir)

			// create a mock io.ReadCloser with the test content
			r := io.NopCloser(strings.NewReader(tc.content))

			// determine target directory
			targetDir := tempDir
			if tc.subDir != "" {
				targetDir = filepath.Join(tempDir, tc.subDir)
			}

			err = writeToFile(r, targetDir, tc.filename)
			require.NoError(t, err)

			// verify the file was created at the correct path
			expectedPath := filepath.Join(targetDir, tc.filename)
			_, err = os.Stat(expectedPath)
			require.NoError(t, err, "file should exist at the expected path")

			// read the file content and verify it matches the expected content
			content, err := os.ReadFile(expectedPath)
			require.NoError(t, err)
			require.Equal(t, tc.content, string(content), "file content should match the input content")
		})
	}
}
