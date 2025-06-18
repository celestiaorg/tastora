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
}
