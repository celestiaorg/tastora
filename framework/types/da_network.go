package types

// DANetwork represents a network of DA nodes, categorized as bridge, full, or light nodes.
type DANetwork interface {
	// GetBridgeNodes retrieves a list of bridge nodes in the network.
	GetBridgeNodes() []DANode
	// GetFullNodes retrieves a list of full nodes in the network.
	GetFullNodes() []DANode
	// GetLightNodes retrieves a list of light nodes in the network.
	GetLightNodes() []DANode
}
