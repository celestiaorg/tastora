package config

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/mitchellh/mapstructure"
	"path/filepath"
	"reflect"
	"strconv"
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
//
// NOTE: when using this function, however the type is serialized will be what is written to disk.
// if the struct is not specified in its entirety, some fields may be lost. In order to manipulate raw bytes
// use *[]byte as the generic type.
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
	// if we are dealing with raw bytes, we just return them as is.
	// this use case is for if arbitrary modifications are required.
	if _, ok := config.(*[]byte); ok {
		return nil
	}

	switch filepath.Ext(configPath) {
	case ".toml":
		// the toml files like app.toml and config.toml use mapstructure annotations, not toml annotations.
		// so we need to decode with the mapstructure library.
		var tomlMap map[string]interface{}
		if err := toml.Unmarshal(data, &tomlMap); err != nil {
			return err
		}
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
				stringConversionHook(),
			),
			Result: config,
		})
		if err != nil {
			return err
		}
		return decoder.Decode(tomlMap)
	case ".json":
		return json.Unmarshal(data, config)
	default:
		return fmt.Errorf("unsupported config file format: %s", configPath)
	}
}

// marshalByExtension serializes the given data `v` into a byte slice based on the file extension of `filePath`.
// It supports `.toml` and `.json` formats. If raw bytes are passed, it returns them directly.
// returns an error if the file extension is unsupported or the serialization fails.
func marshalByExtension(config interface{}, filePath string) ([]byte, error) {
	// if we are dealing with raw bytes, we just return them as is.
	// this use case is for if arbitrary modifications are required.
	if byteSlice, ok := config.(*[]byte); ok {
		return *byteSlice, nil
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".toml":
		var tomlMap map[string]interface{}
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result: &tomlMap,
		})
		if err != nil {
			return nil, err
		}
		if err := decoder.Decode(config); err != nil {
			return nil, err
		}
		return toml.Marshal(tomlMap)
	case ".json":
		return json.MarshalIndent(config, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported config file format: %s", ext)
	}
}

// stringConversionHook converts string values to int types for mapstructure
// this is required as some values in the toml files are "2" and "true" and are not
// parsable by default by mapstructure. This hook handles the conversion logic.
func stringConversionHook() mapstructure.DecodeHookFunc {
	return func(f, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}

		str := data.(string)

		switch t.Kind() {
		case reflect.Int:
			return strconv.Atoi(str)
		case reflect.Int8:
			val, err := strconv.ParseInt(str, 10, 8)
			return int8(val), err
		case reflect.Int16:
			val, err := strconv.ParseInt(str, 10, 16)
			return int16(val), err
		case reflect.Int32:
			val, err := strconv.ParseInt(str, 10, 32)
			return int32(val), err
		case reflect.Int64:
			return strconv.ParseInt(str, 10, 64)
		case reflect.Uint:
			val, err := strconv.ParseUint(str, 10, 0)
			return uint(val), err
		case reflect.Uint8:
			val, err := strconv.ParseUint(str, 10, 8)
			return uint8(val), err
		case reflect.Uint16:
			val, err := strconv.ParseUint(str, 10, 16)
			return uint16(val), err
		case reflect.Uint32:
			val, err := strconv.ParseUint(str, 10, 32)
			return uint32(val), err
		case reflect.Uint64:
			return strconv.ParseUint(str, 10, 64)
		case reflect.Bool:
			return strconv.ParseBool(str)
		default:
			return data, nil
		}
	}
}
