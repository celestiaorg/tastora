package types

import "fmt"

// NodeType represents any node type that can be converted to a string
type NodeType interface {
	String() string
}

// Consensus Node Types (Celestia blockchain consensus)

// ConsensusNodeType represents nodes that participate in Celestia consensus
type ConsensusNodeType int

const (
	// Unspecified consensus node type (zero value)
	ConsensusUnspecified ConsensusNodeType = iota // Unspecified consensus node type
	// Celestia Chain Node Types (CometBFT consensus)
	NodeTypeValidator     // Validator node in blockchain
	NodeTypeConsensusFull // Full node in blockchain
)

// String returns the string representation of the ConsensusNodeType
func (n ConsensusNodeType) String() string {
	if int(n) >= 0 && int(n) < len(consensusNodeTypeStrings) {
		return consensusNodeTypeStrings[n]
	}
	return fmt.Sprintf("ConsensusNodeType(%d)", int(n))
}

var consensusNodeTypeStrings = [...]string{
	"unspec", // ConsensusUnspecified
	"val",    // NodeTypeValidator
	"cfull",  // NodeTypeConsensusFull
}

// DA Node Types (Data Availability layer)

// DANodeType represents Data Availability layer node types
type DANodeType int

const (
	// Unspecified DA node type (zero value)
	DAUnspecified DANodeType = iota // Unspecified DA node type
	BridgeNode                      // Bridge node in DA network
	LightNode                       // Light node in DA network
	FullNode                        // Full node in DA network
)

// String returns the string representation of the DANodeType
func (n DANodeType) String() string {
	if int(n) >= 0 && int(n) < len(daNodeTypeStrings) {
		return daNodeTypeStrings[n]
	}
	return fmt.Sprintf("DANodeType(%d)", int(n))
}

var daNodeTypeStrings = [...]string{
	"unspec", // DAUnspecified
	"bridge", // BridgeNode
	"light",  // LightNode
	"full",   // FullNode
}

// Interface Compliance Checks
var (
	_ NodeType = ConsensusUnspecified
	_ NodeType = DAUnspecified
)
