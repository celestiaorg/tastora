package types

import (
	"fmt"
	"regexp"
	"strings"
)

var p2pAddressPattern *regexp.Regexp

func init() {
	// match a pattern like the following
	// /ip4/172.91.0.3/tcp/2121
	// can be used to extract
	p2pAddressPattern = regexp.MustCompile(`^/ip4/\d+\.\d+\.\d+\.\d+/tcp/\d+$`)
}

// P2PInfo represents peer-to-peer network information including a unique PeerID and a list of network addresses.
// Note: the json tags are explicitly upper case as the actual type returned from the api does not have json
// tags, and the tags on this struct map to the field names (default in go if no json tags are provided).
type P2PInfo struct {
	PeerID    string   `json:"ID"`
	Addresses []string `json:"Addrs"`
}

// GetP2PAddress generates a P2P address by combining the first valid TCPv4 address and the PeerID.
// Returns the constructed P2P address as a string or an error if no valid address is found.
// This can be used to pass to the light node via the CELESTIA_CUSTOM environment variable.
func (p P2PInfo) GetP2PAddress() (string, error) {
	addr, err := extractFirstTCPv4Addr(p.Addresses)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/p2p/%s", addr, p.PeerID), nil
}

// extractFirstTCPv4Addr returns the first /ip4/.../tcp/... address from a list of multiaddrs.
func extractFirstTCPv4Addr(addrs []string) (string, error) {
	for _, addr := range addrs {
		if p2pAddressPattern.MatchString(addr) {
			return addr, nil
		}
	}
	return "", fmt.Errorf("no /ip4/.../tcp/... address found")
}

type Header struct {
	Height uint64 `json:"height"`
}

type Blob struct {
	Namespace    string `json:"namespace"`
	Data         string `json:"data"`
	ShareVersion int    `json:"share_version"`
	Commitment   string `json:"commitment"`
	Index        int    `json:"index"`
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