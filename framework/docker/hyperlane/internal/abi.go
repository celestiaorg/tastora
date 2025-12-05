package internal

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"strings"
)

var HyperlaneRouterABI abi.ABI

func init() {
	a, err := abi.JSON(strings.NewReader(rawHyperlaneRouterABI))
	if err != nil {
		panic(err)
	}
	HyperlaneRouterABI = a
}

const rawHyperlaneRouterABI = `[
    {
        "inputs": [
            {"internalType": "uint32",  "name": "domain", "type": "uint32"},
            {"internalType": "bytes32", "name": "router", "type": "bytes32"}
        ],
        "name": "enrollRemoteRouter",
        "outputs": [],
        "stateMutability": "nonpayable",
        "type": "function"
    }
]`
