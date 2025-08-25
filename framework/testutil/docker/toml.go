package docker

import (
	"bytes"
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/celestiaorg/tastora/framework/docker/file"
	tomlutil "github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/moby/moby/client"
	"go.uber.org/zap"
)

// ModifyConfigFile reads, modifies, then overwrites a toml config file, useful for config.toml, app.toml, etc.
func ModifyConfigFile(
	ctx context.Context,
	logger *zap.Logger,
	dockerClient *client.Client,
	testName string,
	volumeName string,
	filePath string,
	modifications tomlutil.Toml,
) error {
	fr := file.NewRetriever(logger, dockerClient, testName)
	config, err := fr.SingleFileContent(ctx, volumeName, filePath)
	if err != nil {
		return fmt.Errorf("failed to retrieve %s: %w", filePath, err)
	}

	var c tomlutil.Toml
	if err := toml.Unmarshal(config, &c); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", filePath, err)
	}

	if err := tomlutil.RecursiveModify(c, modifications); err != nil {
		return fmt.Errorf("failed to modify %s: %w", filePath, err)
	}

	buf := new(bytes.Buffer)
	if err := toml.NewEncoder(buf).Encode(c); err != nil {
		return fmt.Errorf("failed to encode %s: %w", filePath, err)
	}

	fw := file.NewWriter(logger, dockerClient, testName)
	if err := fw.WriteFile(ctx, volumeName, filePath, buf.Bytes()); err != nil {
		return fmt.Errorf("overwriting %s: %w", filePath, err)
	}

	return nil
}