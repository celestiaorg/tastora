package hyperlane

import (
	"context"
	"fmt"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

type Mode string

const (
	RelayerMode Mode = "relayer"
	BackendMode Mode = "backend"
)

const (
	DefaultBackendPort = "8080"
)

// ForwardRelayerConfig configures both docker runtime settings and mode-specific env vars.
type ForwardRelayerConfig struct {
	Logger          *zap.Logger
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string

	Image container.Image

	Settings ForwardRelayerSettings
}

// ForwardRelayerSettings contains settings used for the forward relayer container in both backend
// and relayer mode.
type ForwardRelayerSettings struct {
	ChainID       string
	CelestiaRPC   string
	CelestiaGRPC  string
	BackendURL    string
	PrivateKeyHex string

	Port string

	Env []string
}

func (s *ForwardRelayerSettings) ToEnv() []string {
	env := []string{
		fmt.Sprintf("CHAIN_ID=%s", s.ChainID),
		fmt.Sprintf("CELESTIA_RPC=%s", s.CelestiaRPC),
		fmt.Sprintf("CELESTIA_GRPC=%s", s.CelestiaGRPC),
		fmt.Sprintf("BACKEND_URL=%s", s.BackendURL),
		fmt.Sprintf("PRIVATE_KEY_HEX=%s", s.PrivateKeyHex),
		fmt.Sprintf("PORT=%s", s.Port),
	}

	return append(env, s.Env...)
}

// docker pull ghcr.io/celestiaorg/forwarding-relayer:sha-d43e7c6
type ForwardRelayer struct {
	*container.Node
	cfg  ForwardRelayerConfig
	mode Mode
}

// Name returns the hostname/container name for the agent container
func (rly *ForwardRelayer) Name() string {
	base := fmt.Sprintf("hyperlane-forward-%s-%d-%s", rly.mode, rly.Index, internal.SanitizeDockerResourceName(rly.TestName))
	return internal.CondenseHostName(base)
}

// NewForwardRelayer creates a new Hyperlane forward relayer with mode-specific runtime config.
func NewForwardRelayer(ctx context.Context, cfg ForwardRelayerConfig, testName string, mode Mode) (*ForwardRelayer, error) {
	image := cfg.Image
	if image.UIDGID == "" {
		image.UIDGID = hyperlaneDefaultUIDGID
	}

	node := container.NewNode(
		cfg.DockerNetworkID,
		cfg.DockerClient,
		testName,
		image,
		"/app",
		0,
		ForwardRelayerNodeType,
		cfg.Logger,
	)

	rly := &ForwardRelayer{
		Node: node,
		cfg:  cfg,
		mode: mode,
	}

	lifecycle := container.NewLifecycle(cfg.Logger, cfg.DockerClient, rly.Name())
	rly.SetContainerLifecycle(lifecycle)

	if err := rly.CreateAndSetupVolume(ctx, rly.Name()); err != nil {
		return nil, err
	}

	return rly, nil
}

// Start creates and starts the forward relayer container in the configured mode.
func (rly *ForwardRelayer) Start(ctx context.Context) error {
	var (
		cmd   []string
		ports nat.PortMap
	)

	cfg := rly.cfg
	env := cfg.Settings.ToEnv()

	switch rly.mode {
	case BackendMode:
		cmd = append(cmd, "backend")

		ports = nat.PortMap{
			nat.Port(cfg.Settings.Port + "/tcp"): {},
		}

	case RelayerMode:
		cmd = append(cmd, "relayer")
	default:
		return fmt.Errorf("unsupported forward relayer mode: %q", rly.mode)
	}

	if err := rly.CreateContainer(
		ctx,
		rly.TestName,
		rly.NetworkID,
		rly.Image,
		ports,
		"",
		rly.Bind(),
		nil,
		rly.Name(),
		cmd,
		env,
		nil,
	); err != nil {
		return fmt.Errorf("create agent container: %w", err)
	}

	return rly.StartContainer(ctx)
}
