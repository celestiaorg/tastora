package types

import "context"

// RollkitChain is an interface defining the lifecycle operations for a Rollkit chain.
type RollkitChain interface {
	// GetNodes retrieves a list of RollkitNode instances associated with the chain.
	GetNodes() []RollkitNode
}

// RollkitNode is an interface defining the lifecycle operations for a Rollkit node.
type RollkitNode interface {
	// Init starts the RollkitNode with optional start arguments.
	Init(ctx context.Context, initArguments ...string) error
	// Start starts the RollkitNode with optional start arguments.
	Start(ctx context.Context, startArguments ...string) error
	// GetHostName returns the hostname of the RollkitNode.
	GetHostName() string
	// GetHostRPCPort returns the host RPC port.
	GetHostRPCPort() string
	// GetHostAPIPort returns the host API port.
	GetHostAPIPort() string
	// GetHostGRPCPort returns the host GRPC port.
	GetHostGRPCPort() string
	// GetHostP2PPort returns the host P2P port.
	GetHostP2PPort() string
	// GetHostHTTPPort returns the host HTTP port.
	GetHostHTTPPort() string
}
