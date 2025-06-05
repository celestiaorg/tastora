package types

import "context"

// Provider is an interface to implement chain and node provisioning functionality.
// It defines methods to retrieve instances of Chain and Node.
//
// different Providers can be implement to enable different infrastructure / bankends for the Chains and Nodes to run
// on.
type Provider interface {
	// GetChain returns an implement of the Chain interface.
	GetChain(ctx context.Context) (Chain, error)
	// GetDANode returns an implementation of the Node interface.
	GetDANode(ctx context.Context, nodeType DANodeType) (DANode, error)
	// GetDANetwork retrieves an implementation of the DataAvailabilityNetwork.
	GetDANetwork(ctx context.Context) (DataAvailabilityNetwork, error)
}
