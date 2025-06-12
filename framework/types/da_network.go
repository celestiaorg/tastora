package types

import "strings"

// DataAvailabilityNetwork represents a network of DA nodes, categorized as bridge or light nodes.
type DataAvailabilityNetwork interface {
	// GetBridgeNodes retrieves a list of bridge nodes in the network.
	GetBridgeNodes() []DANode
	// GetLightNodes retrieves a list of light nodes in the network.
	GetLightNodes() []DANode
}

// BuildCelestiaCustomEnvVar constructs a custom environment variable for Celestia using chain ID, genesis hash, and P2P address.
func BuildCelestiaCustomEnvVar(chainID, genesisBlockHash, p2pAddress string) string {
	var sb strings.Builder
	sb.WriteString(chainID)
	if genesisBlockHash != "" {
		sb.WriteString(":")
		sb.WriteString(genesisBlockHash)
	}
	if p2pAddress != "" {
		sb.WriteString(":")
		sb.WriteString(p2pAddress)
	}
	return sb.String()
}
