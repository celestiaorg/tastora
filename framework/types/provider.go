package types

import "context"

// Provider is an interface to implement chain and node provisioning functionality.
// It defines methods to retrieve instances of DataAvailabilityNetwork.
//
// different Providers can be implemented to enable different infrastructure / backends for the
// DataAvailabilityNetwork to run on.
type Provider interface {
	// GetDataAvailabilityNetwork retrieves an implementation of the DataAvailabilityNetwork.
	GetDataAvailabilityNetwork(ctx context.Context) (DataAvailabilityNetwork, error)
}
