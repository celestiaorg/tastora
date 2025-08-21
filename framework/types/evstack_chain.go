package types

import "context"

// EVStackChain is an interface defining the lifecycle operations for an evstack chain.
type EVStackChain interface {
	// GetNodes retrieves a list of EVStackNode instances associated with the chain.
	GetNodes() []EVStackNode
}

// EVStackNode is an interface defining the lifecycle operations for an evstack node.
type EVStackNode interface {
	// Init starts the EVStackNode with optional start arguments.
	Init(ctx context.Context, initArguments ...string) error
	// Start starts the EVStackNode with optional start arguments.
	Start(ctx context.Context, startArguments ...string) error
	// GetHostName returns the hostname of the EVStackNode.
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