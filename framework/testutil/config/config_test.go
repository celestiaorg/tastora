package config

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/BurntSushi/toml"
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

// mapstructure-tagged config structs for testing real-world cosmos SDK config scenarios
type MapstructureSimpleConfig struct {
	Name    string `json:"name" mapstructure:"name"`
	Port    int    `json:"port" mapstructure:"port"`
	Enabled bool   `json:"enabled" mapstructure:"enabled"`
}

type MapstructureAppConfig struct {
	Name    string `json:"name" mapstructure:"name"`
	Version string `json:"version" mapstructure:"version"`
}

type MapstructureDatabaseConfig struct {
	Host string `json:"host" mapstructure:"host"`
	Port int    `json:"port" mapstructure:"port"`
}

type MapstructureNestedConfig struct {
	App      MapstructureAppConfig      `json:"app" mapstructure:"app"`
	Database MapstructureDatabaseConfig `json:"database" mapstructure:"database"`
}

// mixed tags config for testing precedence
type MixedTagsConfig struct {
	Name    string `json:"name" toml:"name_toml" mapstructure:"name"`
	Port    int    `json:"port" toml:"port_toml" mapstructure:"port"`
	Enabled bool   `json:"enabled" toml:"enabled_toml" mapstructure:"enabled"`
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

// tests for mapstructure-tagged structs (real-world cosmos SDK config scenario)
func TestModifySimpleTOMLWithMapstructureTags(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	initialContent := `name = "cosmos-app"
port = 1317
enabled = true
`
	mockRW.setFile("app.toml", []byte(initialContent))

	err := Modify(ctx, mockRW, "app.toml", func(cfg *MapstructureSimpleConfig) {
		cfg.Name = "modified-cosmos-app"
		cfg.Port = 2317
		cfg.Enabled = false
	})

	require.NoError(t, err)

	var result MapstructureSimpleConfig
	require.NoError(t, unmarshalByExtension(mockRW.getFile("app.toml"), &result, "app.toml"))

	require.Equal(t, "modified-cosmos-app", result.Name)
	require.Equal(t, 2317, result.Port)
	require.Equal(t, false, result.Enabled)
}

func TestModifyNestedTOMLWithMapstructureTags(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	initialContent := `[app]
name = "celestia-node"
version = "1.0.0"

[database]
host = "localhost"
port = 5432
`
	mockRW.setFile("config.toml", []byte(initialContent))

	err := Modify(ctx, mockRW, "config.toml", func(cfg *MapstructureNestedConfig) {
		cfg.App.Name = "updated-celestia-node"
		cfg.App.Version = "2.0.0"
		cfg.Database.Host = "remote-cosmos-db"
		cfg.Database.Port = 3306
	})

	require.NoError(t, err)

	var result MapstructureNestedConfig
	require.NoError(t, unmarshalByExtension(mockRW.getFile("config.toml"), &result, "config.toml"))

	require.Equal(t, "updated-celestia-node", result.App.Name)
	require.Equal(t, "2.0.0", result.App.Version)
	require.Equal(t, "remote-cosmos-db", result.Database.Host)
	require.Equal(t, 3306, result.Database.Port)
}

func TestModifyJSONWithMapstructureTags(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	initialContent := `{
  "name": "cosmos-json-app",
  "port": 8080,
  "enabled": true
}`
	mockRW.setFile("config.json", []byte(initialContent))

	err := Modify(ctx, mockRW, "config.json", func(cfg *MapstructureSimpleConfig) {
		cfg.Name = "modified-json-app"
		cfg.Port = 9090
		cfg.Enabled = false
	})

	require.NoError(t, err)

	var result MapstructureSimpleConfig
	require.NoError(t, unmarshalByExtension(mockRW.getFile("config.json"), &result, "config.json"))

	require.Equal(t, "modified-json-app", result.Name)
	require.Equal(t, 9090, result.Port)
	require.Equal(t, false, result.Enabled)
}

// edge case tests
func TestModifyMixedTags(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	// test that mapstructure tags take precedence in TOML files
	initialContent := `name = "test-mixed"
port = 8080
enabled = true
`
	mockRW.setFile("mixed.toml", []byte(initialContent))

	err := Modify(ctx, mockRW, "mixed.toml", func(cfg *MixedTagsConfig) {
		cfg.Name = "modified-mixed"
		cfg.Port = 9999
		cfg.Enabled = false
	})

	require.NoError(t, err)

	var result MixedTagsConfig
	require.NoError(t, unmarshalByExtension(mockRW.getFile("mixed.toml"), &result, "mixed.toml"))

	require.Equal(t, "modified-mixed", result.Name)
	require.Equal(t, 9999, result.Port)
	require.Equal(t, false, result.Enabled)
}

func TestModifyStringConversionHookWithMapstructure(t *testing.T) {
	ctx := context.Background()
	mockRW := newMockReadWriter()

	// test the string conversion hook with quoted values in TOML
	initialContent := `name = "string-conversion-test"
port = "1317"
enabled = "true"
`
	mockRW.setFile("conversion.toml", []byte(initialContent))

	err := Modify(ctx, mockRW, "conversion.toml", func(cfg *MapstructureSimpleConfig) {
		cfg.Port = 2317
		cfg.Enabled = false
	})

	require.NoError(t, err)

	var result MapstructureSimpleConfig
	require.NoError(t, unmarshalByExtension(mockRW.getFile("conversion.toml"), &result, "conversion.toml"))

	require.Equal(t, "string-conversion-test", result.Name)
	require.Equal(t, 2317, result.Port)
	require.Equal(t, false, result.Enabled)
}

// direct TOML marshalling/unmarshalling tests (bypassing the config system)
func TestDirectTOMLMarshalUnmarshalWithTomlTags(t *testing.T) {
	original := SimpleConfig{
		Name:    "direct-toml-test",
		Port:    8080,
		Enabled: true,
	}

	// marshal using TOML library directly
	tomlData, err := toml.Marshal(original)
	require.NoError(t, err)

	// unmarshal using TOML library directly
	var result SimpleConfig
	err = toml.Unmarshal(tomlData, &result)
	require.NoError(t, err)

	// verify round-trip works with toml tags
	require.Equal(t, original.Name, result.Name)
	require.Equal(t, original.Port, result.Port)
	require.Equal(t, original.Enabled, result.Enabled)
}

func TestDirectTOMLMarshalUnmarshalWithMapstructureTags(t *testing.T) {
	original := MapstructureSimpleConfig{
		Name:    "direct-mapstructure-test",
		Port:    9090,
		Enabled: false,
	}

	// marshal using TOML library directly
	tomlData, err := toml.Marshal(original)
	require.NoError(t, err)

	// unmarshal using TOML library directly
	var result MapstructureSimpleConfig
	err = toml.Unmarshal(tomlData, &result)
	require.NoError(t, err)

	// verify round-trip works (TOML library ignores mapstructure tags and uses field names)
	require.Equal(t, original.Name, result.Name)
	require.Equal(t, original.Port, result.Port)
	require.Equal(t, original.Enabled, result.Enabled)
}
