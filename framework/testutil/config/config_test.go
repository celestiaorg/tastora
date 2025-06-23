package config

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// mock implementation of ReadWriter for testing
type mockReadWriter struct {
	files map[string][]byte
	mutex sync.RWMutex

	// error injection for testing
	readError  error
	writeError error
}

func newMockReadWriter() *mockReadWriter {
	return &mockReadWriter{
		files: make(map[string][]byte),
	}
}

func (m *mockReadWriter) ReadFile(ctx context.Context, filePath string) ([]byte, error) {
	if m.readError != nil {
		return nil, m.readError
	}

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	data, exists := m.files[filePath]
	if !exists {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}
	return data, nil
}

func (m *mockReadWriter) WriteFile(ctx context.Context, filePath string, data []byte) error {
	if m.writeError != nil {
		return m.writeError
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.files[filePath] = data
	return nil
}

func (m *mockReadWriter) setFile(filePath string, content []byte) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.files[filePath] = content
}

func (m *mockReadWriter) getFile(filePath string) []byte {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.files[filePath]
}

// test config structs
type SimpleConfig struct {
	Name    string `json:"name" toml:"name"`
	Port    int    `json:"port" toml:"port"`
	Enabled bool   `json:"enabled" toml:"enabled"`
}

type AppConfig struct {
	Name    string `json:"name" toml:"name"`
	Version string `json:"version" toml:"version"`
}

type DatabaseConfig struct {
	Host string `json:"host" toml:"host"`
	Port int    `json:"port" toml:"port"`
}

type NestedConfig struct {
	App      AppConfig      `json:"app" toml:"app"`
	Database DatabaseConfig `json:"database" toml:"database"`
}

func TestModifySimpleJSON(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	initialContent := `{
  "name": "test-app",
  "port": 8080,
  "enabled": true
}`
	mockRW.setFile("config.json", []byte(initialContent))

	err := Modify(ctx, mockRW, "config.json", func(cfg *SimpleConfig) {
		cfg.Name = "modified-app"
		cfg.Port = 9090
		cfg.Enabled = false
	})

	require.NoError(t, err)

	var result SimpleConfig
	require.NoError(t, unmarshalByExtension(mockRW.getFile("config.json"), &result, "config.json"))

	require.Equal(t, "modified-app", result.Name)
	require.Equal(t, 9090, result.Port)
	require.Equal(t, false, result.Enabled)
}

func TestModifySimpleTOML(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	initialContent := `name = "test-app"
port = 8080
enabled = true
`
	mockRW.setFile("config.toml", []byte(initialContent))

	err := Modify(ctx, mockRW, "config.toml", func(cfg *SimpleConfig) {
		cfg.Name = "modified-toml-app"
		cfg.Port = 3000
	})

	require.NoError(t, err)

	var result SimpleConfig
	require.NoError(t, unmarshalByExtension(mockRW.getFile("config.toml"), &result, "config.toml"))

	require.Equal(t, "modified-toml-app", result.Name)
	require.Equal(t, 3000, result.Port)
	require.Equal(t, true, result.Enabled)
}

func TestModifyNestedJSON(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	initialContent := `{
  "app": {
    "name": "my-app",
    "version": "1.0.0"
  },
  "database": {
    "host": "localhost",
    "port": 5432
  }
}`
	mockRW.setFile("nested.json", []byte(initialContent))

	err := Modify(ctx, mockRW, "nested.json", func(cfg *NestedConfig) {
		cfg.App.Version = "2.0.0"
		cfg.Database.Host = "remote-db"
	})

	require.NoError(t, err)

	var result NestedConfig
	require.NoError(t, unmarshalByExtension(mockRW.getFile("nested.json"), &result, "nested.json"))

	require.Equal(t, "my-app", result.App.Name)
	require.Equal(t, "2.0.0", result.App.Version)
	require.Equal(t, "remote-db", result.Database.Host)
	require.Equal(t, 5432, result.Database.Port)
}

func TestModifyNestedTOML(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	initialContent := `[app]
name = "my-app"
version = "1.0.0"

[database]
host = "localhost"
port = 5432
`
	mockRW.setFile("nested.toml", []byte(initialContent))

	err := Modify(ctx, mockRW, "nested.toml", func(cfg *NestedConfig) {
		cfg.App.Name = "updated-app"
		cfg.Database.Port = 3306
	})

	require.NoError(t, err)

	var result NestedConfig
	require.NoError(t, unmarshalByExtension(mockRW.getFile("nested.toml"), &result, "nested.toml"))

	require.Equal(t, "updated-app", result.App.Name)
	require.Equal(t, "1.0.0", result.App.Version)
	require.Equal(t, "localhost", result.Database.Host)
	require.Equal(t, 3306, result.Database.Port)
}

func TestModifyUnsupportedExtension(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	mockRW.setFile("config.xml", []byte(`<config></config>`))

	err := Modify(ctx, mockRW, "config.xml", func(cfg *SimpleConfig) {
		cfg.Name = "modified"
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported config file format")
}

func TestModifyInvalidJSON(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	invalidJSON := `{
  "name": "test"
  "invalid": json
}`
	mockRW.setFile("invalid.json", []byte(invalidJSON))

	err := Modify(ctx, mockRW, "invalid.json", func(cfg *SimpleConfig) {
		cfg.Name = "modified"
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to unmarshal")
}

func TestModifyReadError(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()
	mockRW.readError = errors.New("read failed")

	err := Modify(ctx, mockRW, "test.json", func(cfg *SimpleConfig) {
		cfg.Name = "modified"
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to retrieve")
}

func TestModifyWriteError(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()
	mockRW.setFile("test.json", []byte(`{"name": "test", "port": 8080, "enabled": true}`))
	mockRW.writeError = errors.New("write failed")

	err := Modify(ctx, mockRW, "test.json", func(cfg *SimpleConfig) {
		cfg.Name = "modified"
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "overwrite")
}

func TestModifyFileNotFound(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	err := Modify(ctx, mockRW, "nonexistent.json", func(cfg *SimpleConfig) {
		cfg.Name = "modified"
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to retrieve")
}

func TestModifyRawBytes(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	initialContent := []byte("original content that needs modification")
	mockRW.setFile("raw.toml", initialContent)

	err := Modify(ctx, mockRW, "raw.toml", func(data *[]byte) {
		*data = []byte("modified raw content")
	})

	require.NoError(t, err)

	result := mockRW.getFile("raw.toml")
	require.Equal(t, []byte("modified raw content"), result)
}
