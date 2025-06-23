package config

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/BurntSushi/toml"
	"path/filepath"
	"strings"
)

// ReadWriter is an interface which ensures a type can both read and write configuration files.
type ReadWriter interface {
	// ReadFile reads the contents of the specified file from the given file path and returns it as a byte slice.
	// An error is returned if the file cannot be accessed or read.
	ReadFile(ctx context.Context, filePath string) ([]byte, error)
	// WriteFile writes the given data to the specified file path. It returns an error if the write operation fails.
	WriteFile(ctx context.Context, filePath string, data []byte) error
}

// Modify reads, modifies, then overwrites a config file, useful for config.toml, app.toml, etc.
func Modify[T any](
	ctx context.Context,
	readWriter ReadWriter,
	filePath string,
	modification func(*T),
) error {
	configFileBz, err := readWriter.ReadFile(ctx, filePath)
	if err != nil {
		return fmt.Errorf("failed to retrieve %s: %w", filePath, err)
	}

	var cfg T
	if err := unmarshalByExtension(configFileBz, &cfg, filePath); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", filePath, err)
	}

	modification(&cfg)

	bz, err := marshalByExtension(&cfg, filePath)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", filePath, err)
	}

	if err := readWriter.WriteFile(ctx, filePath, bz); err != nil {
		return fmt.Errorf("failed to overwrite %s: %w", filePath, err)
	}

	return nil
}

// unmarshalByExtension unmarshals data into the given config based on the file extension of configPath.
func unmarshalByExtension(data []byte, config interface{}, configPath string) error {
	switch filepath.Ext(configPath) {
	case ".toml":
		return toml.Unmarshal(data, config)
	case ".json":
		return json.Unmarshal(data, config)
	default:
		return fmt.Errorf("unsupported config file format: %s", configPath)
	}
}

// marshalByExtension serializes the given data `v` into a byte slice based on the file extension of `filePath`.
// It supports `.toml` and `.json` formats. If raw bytes are passed, it returns them directly.
// returns an error if the file extension is unsupported or the serialization fails.
func marshalByExtension(v interface{}, filePath string) ([]byte, error) {
	// if we are dealing with raw bytes, we just return them as is.
	// this use case is for if arbitrary modifications are required.
	if byteSlice, ok := v.(*[]byte); ok {
		return *byteSlice, nil
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".toml":
		return toml.Marshal(v)
	case ".json":
		return json.MarshalIndent(v, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported config file format: %s", ext)
	}
}
