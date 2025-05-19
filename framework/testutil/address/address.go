package address

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/types"
	"strings"
)

// BuildInternalPeerAddressList constructs a comma-separated list of internal peer addresses from the given nodes.
// It returns an error if any node fails to provide a peer address.
func BuildInternalPeerAddressList(ctx context.Context, nodes []types.ChainNode) (string, error) {
	addrs := make([]string, 0, len(nodes))

	for _, node := range nodes {
		addr, err := node.GetInternalPeerAddress(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to get peer address from node of type %s: %w", node.GetType(), err)
		}
		addrs = append(addrs, addr)
	}

	return strings.Join(addrs, ","), nil
}

// BuildInternalRPCAddressList constructs a comma-separated list of internal rpc addresses from the given nodes.
// It returns an error if any node fails to provide a peer address.
func BuildInternalRPCAddressList(ctx context.Context, nodes []types.ChainNode) (string, error) {
	addrs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		addr, err := node.GetInternalRPCAddress(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to get rpc address from node of type %s: %w", node.GetType(), err)
		}
		addrs = append(addrs, addr)
	}
	return strings.Join(addrs, ","), nil
}
