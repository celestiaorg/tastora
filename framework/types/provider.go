package types

import "context"

// Provider is an interface to implement chain and node provisioning functionality.
// It defines methods to retrieve instances of Chain and DataAvailabilityNetwork.
//
// different Providers can be implemented to enable different infrastructure / backends for the Chains and
// DataAvailabilityNetwork to run on.
type Provider interface {
	// GetChain returns an implement of the Chain interface.
	GetChain(ctx context.Context) (Chain, error)
	// GetDataAvailabilityNetwork retrieves an implementation of the DataAvailabilityNetwork.
	GetDataAvailabilityNetwork(ctx context.Context) (DataAvailabilityNetwork, error)
}
