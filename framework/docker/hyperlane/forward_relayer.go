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

	Image    container.Image
	Settings ForwardRelayerSettings
}

// ForwardRelayerSettings contains settings used for the forward relayer container in both backend
// and relayer mode.
type ForwardRelayerSettings struct {
	// Relayer config settings
	CelestiaGRPC  string
	BackendURL    string
	PrivateKeyHex string

	// Backend config settings
	Port string

	// Additional overrides
	Env []string
}

// PortValue returns the configured backend port value or a sensible default.
func (s *ForwardRelayerSettings) PortValue() string {
	if s.Port != "" {
		return s.Port
	}

	return DefaultBackendPort
}

// ToEnv converts the settings to a comma separate list of environment variables
// which can be consumed within the container.
func (s *ForwardRelayerSettings) ToEnv() []string {
	port := s.Port
	if port == "" {
		port = DefaultBackendPort
	}

	env := []string{
		fmt.Sprintf("CELESTIA_GRPC=%s", ensureHTTPScheme(s.CelestiaGRPC)),
		fmt.Sprintf("BACKEND_URL=%s", ensureHTTPScheme(s.BackendURL)),
		fmt.Sprintf("PRIVATE_KEY_HEX=%s", s.PrivateKeyHex),
		fmt.Sprintf("PORT=%s", port),
	}

	return append(env, s.Env...)
}

// Validate performs basic sanity checks on the configuration settings.
func (s *ForwardRelayerSettings) Validate(mode Mode) error {
	switch mode {
	case BackendMode:
		if strings.TrimSpace(s.Port) == "" {
			return fmt.Errorf("port is required")
		}
	case RelayerMode:
		if strings.TrimSpace(s.PrivateKeyHex) == "" {
			return fmt.Errorf("private key is required")
		}
		if strings.TrimSpace(s.CelestiaGRPC) == "" {
			return fmt.Errorf("celestia gRPC address is required")
		}
		if strings.TrimSpace(s.BackendURL) == "" {
			return fmt.Errorf("backend URL is required")
		}
	default:
		return fmt.Errorf("unsupported mode: %q", mode)
	}

	return nil
}

// ForwardRelayer encapsulates the container runtime for the Hyperlane forwarding relayer service.
type ForwardRelayer struct {
	*container.Node

	Config    ForwardRelayerConfig
	Mode      Mode
	HostPorts types.Ports
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
		Node:   node,
		Config: cfg,
		Mode:   mode,
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
	settings := rly.Settings()
	if err := settings.Validate(rly.Mode); err != nil {
		return fmt.Errorf("invalid forward relayer settings: %w", err)
	}

	var (
		cmd   []string
		ports nat.PortMap
	)

	switch rly.Mode {
	case BackendMode:
		cmd = append(cmd, "backend")

		ports = nat.PortMap{
			nat.Port(settings.PortValue() + "/tcp"): {},
		}

	case RelayerMode:
		cmd = append(cmd, "relayer")
	default:
		return fmt.Errorf("unsupported forward relayer mode: %q", rly.Mode)
	}

	env := settings.ToEnv()

	if err := rly.CreateContainer(
		ctx,
		rly.TestName,
		rly.NetworkID,
		rly.Image,
		ports,
		"",
		rly.Bind(),
		nil,
		rly.HostName(),
		cmd,
		env,
		nil,
	); err != nil {
		return fmt.Errorf("create agent container: %w", err)
	}

	if err := rly.StartContainer(ctx); err != nil {
		return err
	}

	if rly.Mode != BackendMode {
		return nil
	}

	hostPorts, err := rly.ContainerLifecycle.GetHostPorts(ctx, settings.PortValue()+"/tcp")
	if err != nil {
		return fmt.Errorf("get backend host port: %w", err)
	}
	if len(hostPorts) == 0 {
		return fmt.Errorf("no host ports returned for backend port %s", settings.PortValue())
	}

	rly.HostPorts = types.Ports{
		HTTP: internal.MustExtractPort(hostPorts[0]),
	}

	return nil
}

// Name returns the hostname/container name for the agent container
func (rly *ForwardRelayer) Name() string {
	return fmt.Sprintf("hyperlane-forward-%s-%d-%s", rly.Mode, rly.Index, internal.SanitizeDockerResourceName(rly.TestName))
}

// HostName returns the condensed hostname used for in-network container communication.
func (rly *ForwardRelayer) HostName() string {
	return internal.CondenseHostName(rly.Name())
}

// GetNetworkInfo returns internal/external network address information.
func (rly *ForwardRelayer) GetNetworkInfo(ctx context.Context) (types.NetworkInfo, error) {
	internalIP, err := internal.GetContainerInternalIP(ctx, rly.DockerClient, rly.ContainerLifecycle.ContainerID())
	if err != nil {
		return types.NetworkInfo{}, err
	}

	internalPorts := types.Ports{}
	if rly.Mode == BackendMode {
		internalPorts.HTTP = rly.Config.Settings.PortValue()
	}

	return types.NetworkInfo{
		Internal: types.Network{
			Hostname: rly.HostName(),
			IP:       internalIP,
			Ports:    internalPorts,
		},
		External: types.Network{
			Hostname: "localhost",
			IP:       "127.0.0.1",
			Ports:    rly.HostPorts,
		},
	}, nil
}

// Settings returns the forward relayer service settings.
func (rly *ForwardRelayer) Settings() ForwardRelayerSettings {
	return rly.Config.Settings
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
