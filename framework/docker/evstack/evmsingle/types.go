package evmsingle

import "github.com/celestiaorg/tastora/framework/types"

// defaultPorts returns the default internal container ports for an ev-node-evm-single node.
func defaultPorts() types.Ports {
    return types.Ports{
        RPC:    "7331",
        P2P:    "7676",
        // No explicit HTTP/GRPC separation for this image; RPC is the primary port.
    }
}

