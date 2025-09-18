package reth

import "github.com/celestiaorg/tastora/framework/types"

// defaultPorts returns the default internal container ports for a Reth node.
func defaultPorts() types.Ports {
    return types.Ports{
        Metrics: "9001",
        P2P:     "30303",
        RPC:     "8545",
        Engine:  "8551",
        API:     "8546", // WS
    }
}
