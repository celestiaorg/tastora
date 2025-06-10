package types

import "context"

type RollkitChain interface {
	GetNodes() []RollkitNode
}

type RollkitNode interface {
	Start(ctx context.Context, startArguments ...string) error
	Init(ctx context.Context, initArguments ...string) error
}
