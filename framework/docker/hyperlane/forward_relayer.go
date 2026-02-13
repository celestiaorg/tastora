package hyperlane

import (
	"context"
	"fmt"
	"strings"

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
	port := s.Port
	if port == "" {
		port = DefaultBackendPort
	}

	env := []string{
		fmt.Sprintf("CHAIN_ID=%s", s.ChainID),
		fmt.Sprintf("CELESTIA_RPC=%s", ensureHTTPScheme(s.CelestiaRPC)),
		fmt.Sprintf("CELESTIA_GRPC=%s", s.CelestiaGRPC),
		fmt.Sprintf("BACKEND_URL=%s", ensureHTTPScheme(s.BackendURL)),
		fmt.Sprintf("PRIVATE_KEY_HEX=%s", s.PrivateKeyHex),
		fmt.Sprintf("PORT=%s", port),
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

	settings := rly.cfg.Settings
	if err := settings.validate(rly.mode); err != nil {
		return fmt.Errorf("invalid forward relayer settings: %w", err)
	}
	env := settings.ToEnv()

	switch rly.mode {
	case BackendMode:
		cmd = append(cmd, "backend")

		ports = nat.PortMap{
			nat.Port(settings.PortValue() + "/tcp"): {},
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

func (s *ForwardRelayerSettings) validate(mode Mode) error {
	switch mode {
	case BackendMode:
		if s.PortValue() == "" {
			return fmt.Errorf("port is required")
		}
	case RelayerMode:
		missing := make([]string, 0, 5)
		if s.ChainID == "" {
			missing = append(missing, "ChainID")
		}
		if s.CelestiaRPC == "" {
			missing = append(missing, "CelestiaRPC")
		}
		if s.CelestiaGRPC == "" {
			missing = append(missing, "CelestiaGRPC")
		}
		if s.BackendURL == "" {
			missing = append(missing, "BackendURL")
		}
		if s.PrivateKeyHex == "" {
			missing = append(missing, "PrivateKeyHex")
		}

		if len(missing) > 0 {
			return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
		}
	default:
		return fmt.Errorf("unsupported mode: %q", mode)
	}

	return nil
}

func (s *ForwardRelayerSettings) PortValue() string {
	if s.Port != "" {
		return s.Port
	}
	return DefaultBackendPort
}

func ensureHTTPScheme(address string) string {
	if address == "" {
		return address
	}

	if strings.Contains(address, "://") {
		return address
	}

	return "http://" + address
}
