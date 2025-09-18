package types

import (
	"context"
	"fmt"
)

// NetworkInfo provides standardized access to network configuration
// with clear separation between internal and external connectivity.
type NetworkInfo struct {
	Internal Network
	External Network
}

// Network contains network information for connectivity
type Network struct {
	Hostname string
	IP       string
	Ports    Ports
}

// RPCAddress returns the full RPC address (hostname:port)
func (n Network) RPCAddress() string {
	return fmt.Sprintf("%s:%s", n.Hostname, n.Ports.RPC)
}

// GRPCAddress returns the full GRPC address (hostname:port)
func (n Network) GRPCAddress() string {
	return fmt.Sprintf("%s:%s", n.Hostname, n.Ports.GRPC)
}

// APIAddress returns the full API address (hostname:port)
func (n Network) APIAddress() string {
	return fmt.Sprintf("%s:%s", n.Hostname, n.Ports.API)
}

// P2PAddress returns the full P2P address (hostname:port)
func (n Network) P2PAddress() string {
	return fmt.Sprintf("%s:%s", n.Hostname, n.Ports.P2P)
}

// HTTPAddress returns the full HTTP address (hostname:port)
func (n Network) HTTPAddress() string {
	return fmt.Sprintf("%s:%s", n.Hostname, n.Ports.HTTP)
}

// Ports contains port information for various services
type Ports struct {
    RPC      string
    API      string
    P2P      string
    HTTP     string
    CoreRPC  string // Only needed for DA nodes - port to connect to celestia-app RPC
    CoreGRPC string // Only needed for DA nodes - port to connect to celestia-app GRPC
    GRPC     string // General purpose GRPC port (used by Cosmos); keep for backward compatibility
    Engine   string // Only used by Reth - Engine/AuthRPC port (8551)
    Metrics  string // Only used by Reth - Prometheus metrics port (9001)
}

// NetworkInfoProvider is an interface for types that can provide network information
type NetworkInfoProvider interface {
	GetNetworkInfo(ctx context.Context) (NetworkInfo, error)
}
