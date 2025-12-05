package hyperlane

import (
	"context"

	"github.com/celestiaorg/tastora/framework/docker/container"
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
}

// NewAgent creates a new Hyperlane agent (for future implementation)
func NewAgent(cfg Config, testName string, agentType AgentType, config *RelayerConfig) *Agent {
	return nil
}

// Start starts the agent container (for future implementation)
func (a *Agent) Start(ctx context.Context) error {
	return nil
}
