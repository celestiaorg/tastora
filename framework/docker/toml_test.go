package docker_test

import (
	"context"
	"testing"

	"github.com/celestiaorg/tastora/framework/docker"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/docker/file"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// TestConfig represents a dummy configuration struct with TOML tags for testing
type TestConfig struct {
	Name     string            `toml:"name"`
	Port     int               `toml:"port"`
	Enabled  bool              `toml:"enabled"`
	Database DatabaseConfig    `toml:"database"`
	Features map[string]string `toml:"features"`
}

type DatabaseConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Username string `toml:"username"`
	SSL      bool   `toml:"ssl"`
}

func TestModifyTypedConfigFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping due to short mode")
	}

	cli, network := docker.DockerSetup(t)
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Create a volume for testing
	v, err := cli.VolumeCreate(ctx, volumetypes.CreateOptions{
		Labels: map[string]string{consts.CleanupLabel: t.Name()},
	})
	require.NoError(t, err)

	img := docker.NewImage(
		logger,
		cli,
		network,
		t.Name(),
		"busybox", "stable",
	)

	t.Run("successful modification", func(t *testing.T) {
		// Initial TOML content
		initialToml := `name = "test-service"
port = 8080
enabled = true

[database]
host = "localhost"
port = 5432
username = "admin"
ssl = false

[features]
auth = "enabled"
logging = "debug"
`

		// Write initial config to volume
		fw := file.NewWriter(logger, cli, t.Name())
		require.NoError(t, fw.WriteFile(ctx, v.Name, "config.toml", []byte(initialToml)))

		// Modify the config using ModifyTypedConfigFile
		err := docker.ModifyTypedConfigFile[TestConfig](
			ctx,
			logger,
			cli,
			t.Name(),
			v.Name,
			"config.toml",
			func(cfg *TestConfig) {
				cfg.Name = "modified-service"
				cfg.Port = 9090
				cfg.Enabled = false
				cfg.Database.Host = "remote-host"
				cfg.Database.Port = 3306
				cfg.Database.SSL = true
				cfg.Features["auth"] = "disabled"
				cfg.Features["cache"] = "redis"
			},
		)
		require.NoError(t, err)

		// Read the modified file and verify changes
		res := img.Run(
			ctx,
			[]string{"sh", "-c", "cat /mnt/test/config.toml"},
			docker.ContainerOptions{
				Binds: []string{v.Name + ":/mnt/test"},
				User:  consts.UserRootString,
			},
		)
		require.NoError(t, res.Err)

		modifiedContent := string(res.Stdout)

		// Verify specific modifications
		require.Contains(t, modifiedContent, `name = "modified-service"`)
		require.Contains(t, modifiedContent, `port = 9090`)
		require.Contains(t, modifiedContent, `enabled = false`)
		require.Contains(t, modifiedContent, `host = "remote-host"`)
		require.Contains(t, modifiedContent, `port = 3306`)
		require.Contains(t, modifiedContent, `ssl = true`)
		require.Contains(t, modifiedContent, `auth = "disabled"`)
		require.Contains(t, modifiedContent, `cache = "redis"`)
	})

	t.Run("modification with empty config", func(t *testing.T) {
		// Write empty config file
		fw := file.NewWriter(logger, cli, t.Name())
		require.NoError(t, fw.WriteFile(ctx, v.Name, "empty.toml", []byte("")))

		// Modify the empty config
		err := docker.ModifyTypedConfigFile[TestConfig](
			ctx,
			logger,
			cli,
			t.Name(),
			v.Name,
			"empty.toml",
			func(cfg *TestConfig) {
				cfg.Name = "new-service"
				cfg.Port = 3000
				cfg.Enabled = true
				cfg.Database = DatabaseConfig{
					Host:     "newdb",
					Port:     5432,
					Username: "user",
					SSL:      true,
				}
				cfg.Features = map[string]string{
					"feature1": "value1",
					"feature2": "value2",
				}
			},
		)
		require.NoError(t, err)

		// Verify the file was created correctly
		res := img.Run(
			ctx,
			[]string{"sh", "-c", "cat /mnt/test/empty.toml"},
			docker.ContainerOptions{
				Binds: []string{v.Name + ":/mnt/test"},
				User:  consts.UserRootString,
			},
		)
		require.NoError(t, res.Err)

		content := string(res.Stdout)
		require.Contains(t, content, `name = "new-service"`)
		require.Contains(t, content, `port = 3000`)
		require.Contains(t, content, `enabled = true`)
	})

	t.Run("error cases", func(t *testing.T) {
		t.Run("non-existent file", func(t *testing.T) {
			err := docker.ModifyTypedConfigFile[TestConfig](
				ctx,
				logger,
				cli,
				t.Name(),
				v.Name,
				"non-existent.toml",
				func(cfg *TestConfig) {
					cfg.Name = "test"
				},
			)
			require.Error(t, err)
			require.Contains(t, err.Error(), "failed to retrieve non-existent.toml")
		})

		t.Run("invalid TOML content", func(t *testing.T) {
			// Write invalid TOML content
			invalidToml := `name = "test"
[invalid
section without closing bracket
`
			fw := file.NewWriter(logger, cli, t.Name())
			require.NoError(t, fw.WriteFile(ctx, v.Name, "invalid.toml", []byte(invalidToml)))

			err := docker.ModifyTypedConfigFile[TestConfig](
				ctx,
				logger,
				cli,
				t.Name(),
				v.Name,
				"invalid.toml",
				func(cfg *TestConfig) {
					cfg.Name = "test"
				},
			)
			require.Error(t, err)
			require.Contains(t, err.Error(), "failed to unmarshal invalid.toml")
		})

		t.Run("non-existent volume", func(t *testing.T) {
			err := docker.ModifyTypedConfigFile[TestConfig](
				ctx,
				logger,
				cli,
				t.Name(),
				"non-existent-volume",
				"config.toml",
				func(cfg *TestConfig) {
					cfg.Name = "test"
				},
			)
			require.Error(t, err)
			require.Contains(t, err.Error(), "failed to retrieve config.toml")
		})
	})
}
