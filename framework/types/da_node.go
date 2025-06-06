package types

import (
	"context"
	"fmt"
	"regexp"

	"github.com/celestiaorg/go-square/v2/share"
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

type DANodeType int

const (
	BridgeNode DANodeType = iota
	LightNode
	FullNode
)

func (n DANodeType) String() string {
	return nodeStrings[n]
}

var nodeStrings = [...]string{
	"bridge",
	"light",
	"full",
}

type DANode interface {
	// Start starts the node.
	Start(ctx context.Context, opts ...DANodeStartOption) error
	// Stop stops the node.
	Stop(ctx context.Context) error
	// GetType returns the type of node. E.g. "bridge" / "light" / "full"
	GetType() DANodeType
	// GetHeader returns a header at a specified height.
	GetHeader(ctx context.Context, height uint64) (Header, error)
	GetAllBlobs(ctx context.Context, height uint64, namespaces []share.Namespace) ([]Blob, error)
	// GetHostRPCAddress returns the externally resolvable RPC address of the node.
	GetHostRPCAddress() string
	GetP2PInfo(ctx context.Context) (P2PInfo, error)
}

type Blob struct {
	Namespace    string `json:"namespace"`
	Data         string `json:"data"`
	ShareVersion int    `json:"share_version"`
	Commitment   string `json:"commitment"`
	Index        int    `json:"index"`
}

type DANodeStartOption func(*DANodeStartOptions)

// DANodeStartOptions represents the configuration options required for starting a DA node.
type DANodeStartOptions struct {
	// ChainID is the chain ID.
	ChainID string
	// StartArguments specifies any additional start arguments after "celestia start <type>"
	StartArguments []string
	// EnvironmentVariables specifies any environment variables that should be passed to the DANode
	// e.g. the CELESTIA_CUSTOM environment variable.
	EnvironmentVariables map[string]string
}

// WithAdditionalStartArguments sets the additional start arguments to be used.
func WithAdditionalStartArguments(startArgs ...string) DANodeStartOption {
	return func(o *DANodeStartOptions) {
		o.StartArguments = startArgs
	}
}

// WithEnvironmentVariables sets the environment variables to be used.
func WithEnvironmentVariables(envVars map[string]string) DANodeStartOption {
	return func(o *DANodeStartOptions) {
		o.EnvironmentVariables = envVars
	}
}

// WithChainID sets the chainID.
func WithChainID(chainID string) DANodeStartOption {
	return func(o *DANodeStartOptions) {
		o.ChainID = chainID
	}
}

// WithCoreIP sets the core IP address for the DA node to connect to.
func WithCoreIP(coreIP string) DANodeStartOption {
	return func(o *DANodeStartOptions) {
		if o.EnvironmentVariables == nil {
			o.EnvironmentVariables = make(map[string]string)
		}
		// Store core IP for later use in building CELESTIA_CUSTOM
		o.EnvironmentVariables["CELESTIA_CORE_IP"] = coreIP
	}
}

// WithGenesisBlockHash sets the genesis block hash for the DA node.
func WithGenesisBlockHash(genesisHash string) DANodeStartOption {
	return func(o *DANodeStartOptions) {
		if o.EnvironmentVariables == nil {
			o.EnvironmentVariables = make(map[string]string)
		}
		// Store genesis hash for later use in building CELESTIA_CUSTOM
		o.EnvironmentVariables["CELESTIA_GENESIS_HASH"] = genesisHash
	}
}
