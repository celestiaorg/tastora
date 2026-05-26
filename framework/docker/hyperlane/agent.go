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
func agentNodeName(testName string, agentType AgentType) string {
	base := fmt.Sprintf("hyperlane-agent-%s-0-%s", agentType, internal.SanitizeDockerResourceName(testName))
	return internal.CondenseHostName(base)
}

func (a *Agent) Name() string {
	return agentNodeName(a.TestName, a.agentType)
}

// NewAgent creates a new Hyperlane agent that will run with the provided config.
// The config should be a relayer config JSON (as produced by BuildRelayerConfig).
func NewAgent(ctx context.Context, cfg Config, testName string, agentType AgentType, d *Deployer) (*Agent, error) {

	image := cfg.HyperlaneImage
	if image.UIDGID == "" {
		image.UIDGID = hyperlaneDefaultUIDGID
	}

	name := agentNodeName(testName, agentType)
	node, err := container.NewNodeBuilder(cfg.DockerClient, testName, image, cfg.Logger).
		WithNetworkID(cfg.DockerNetworkID).
		WithHomeDir(hyperlaneHomeDir).
		WithNodeType(AgentNodeType).
		WithVolumeName(d.Name()).
		Build(ctx, name)
	if err != nil {
		return nil, err
	}

	return &Agent{
		Node:      node,
		cfg:       cfg,
		agentType: agentType,
	}, nil
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
		"HYP_LOG_LEVEL=debug",
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
