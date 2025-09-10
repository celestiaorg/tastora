package address

import (
	"context"
	"fmt"
	"strings"

	"github.com/celestiaorg/tastora/framework/types"
)

type PeerAddresser interface {
	types.NetworkInfoProvider
	GetType() types.ConsensusNodeType
	NodeID(ctx context.Context) (string, error)
}

// BuildInternalPeerAddressList constructs a comma-separated list of internal peer addresses from the given nodes.
// It returns an error if any node fails to provide a peer address.
func BuildInternalPeerAddressList[T PeerAddresser](ctx context.Context, peers []T) (string, error) {
	addrs := make([]string, 0, len(peers))

	for _, p := range peers {
		networkInfo, err := p.GetNetworkInfo(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to get network info from node of type %s: %w", p.GetType().String(), err)
		}
		
		nodeID, err := p.NodeID(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to get node ID from node of type %s: %w", p.GetType().String(), err)
		}
		
		peerAddr := fmt.Sprintf("%s@%s", nodeID, networkInfo.Internal.P2PAddress())
		addrs = append(addrs, peerAddr)
	}

	return strings.Join(addrs, ","), nil
}

// BuildInternalRPCAddressList constructs a comma-separated list of internal rpc addresses from the given nodes.
// It returns an error if any node fails to provide a peer address.
func BuildInternalRPCAddressList(ctx context.Context, nodes []types.ChainNode) (string, error) {
	addrs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		networkInfo, err := node.GetNetworkInfo(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to get network info from node of type %s: %w", node.GetType().String(), err)
		}
		addrs = append(addrs, networkInfo.Internal.RPCAddress())
	}
	return strings.Join(addrs, ","), nil
}
