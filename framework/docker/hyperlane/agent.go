package hyperlane

import (
	"context"
	"fmt"
	"path"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
)

// AgentType defines the type of Hyperlane agent
type AgentType string

const (
	AgentTypeRelayer   AgentType = "relayer"
	AgentTypeValidator AgentType = "validator"
)

// Agent represents a running Hyperlane agent (relayer or validator)
// This is separate from the Hyperlane deployer - agents are long-lived containers
type Agent struct {
	*container.Node

	cfg       Config
	agentType AgentType
}

// Name returns the hostname/container name for the agent container
func (a *Agent) Name() string {
	base := fmt.Sprintf("hyperlane-agent-%s-%d-%s", a.agentType, a.Index, internal.SanitizeDockerResourceName(a.TestName))
	return internal.CondenseHostName(base)
}

// NewAgent creates a new Hyperlane agent that will run with the provided config.
// The config should be a relayer config JSON (as produced by BuildRelayerConfig).
func NewAgent(ctx context.Context, cfg Config, testName string, agentType AgentType, relayerCfg *RelayerConfig) (*Agent, error) {
	if relayerCfg == nil {
		return nil, fmt.Errorf("relayer config is required")
	}

	image := cfg.HyperlaneImage
	if image.UIDGID == "" {
		image.UIDGID = hyperlaneDefaultUIDGID
	}

	node := container.NewNode(
		cfg.DockerNetworkID,
		cfg.DockerClient,
		testName,
		image,
		hyperlaneHomeDir,
		0,
		AgentNodeType,
		cfg.Logger,
	)

	a := &Agent{
		Node:      node,
		cfg:       cfg,
		agentType: agentType,
	}

	lifecycle := container.NewLifecycle(cfg.Logger, cfg.DockerClient, a.Name())
	a.SetContainerLifecycle(lifecycle)

	if err := a.CreateAndSetupVolume(ctx, a.Name()); err != nil {
		return nil, err
	}

	// write relayer-config.json to the agent volume
	b, err := serializeRelayerConfig(relayerCfg)
	if err != nil {
		return nil, fmt.Errorf("serialize relayer config: %w", err)
	}
	if err := a.WriteFile(ctx, "relayer-config.json", b); err != nil {
		return nil, fmt.Errorf("write relayer-config.json: %w", err)
	}

	// also copy registry and configs if they exist on disk relative to this home
	// not strictly required for the agent to start if only relayer-config is needed.
	// callers can populate additional files before Start if desired.

	return a, nil
}

// Start starts the agent container with the relayer config mounted at /workspace/relayer-config.json
func (a *Agent) Start(ctx context.Context) error {
	// Use the agent binary entrypoint with CONFIG_FILES env to point at the config,
	// matching docker-compose pattern for hyperlane-agent images.
	// Some images expect /app/config/config.json; bind our file path via CONFIG_FILES.
	cfgPath := path.Join(hyperlaneHomeDir, "relayer-config.json")
	cmd := []string{"/app/relayer"}
	env := []string{
		fmt.Sprintf("CONFIG_FILES=%s", cfgPath),
		// Provide other common env flags as no-ops to reduce surprises
		"RUST_LOG=info",
	}

	if err := a.CreateContainer(
		ctx,
		a.TestName,
		a.NetworkID,
		a.Image,
		nil,
		"",
		a.Bind(),
		nil,
		a.Name(),
		cmd,
		env,
		nil,
	); err != nil {
		return fmt.Errorf("create agent container: %w", err)
	}

	return a.StartContainer(ctx)
}
