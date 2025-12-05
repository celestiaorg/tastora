package evm

import (
    "bytes"
    "context"
    "fmt"

    "github.com/ethereum/go-ethereum"
    "github.com/ethereum/go-ethereum/accounts/abi"
    "github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/ethclient"
)

// CallFunction performs a read-only eth_call to the given contract, packing the
// method and args using the provided minimal ABI JSON. It returns the unpacked
// outputs as a slice of interface{} in ABI order.
func CallFunction(ctx context.Context, rpcURL, contractAddr string, abiJSON []byte, method string, args ...interface{}) ([]interface{}, error) {
    c, err := ethclient.DialContext(ctx, rpcURL)
    if err != nil {
        return nil, fmt.Errorf("dial eth rpc: %w", err)
    }
    defer c.Close()

    a, err := abi.JSON(bytes.NewReader(abiJSON))
    if err != nil {
        return nil, fmt.Errorf("parse abi: %w", err)
    }
    data, err := a.Pack(method, args...)
    if err != nil {
        return nil, fmt.Errorf("pack %s: %w", method, err)
    }
    to := common.HexToAddress(contractAddr)
    // From can be zero for eth_call
    call := ethereum.CallMsg{To: &to, Data: data}
    out, err := c.CallContract(ctx, call, nil)
    if err != nil {
        return nil, fmt.Errorf("call contract: %w", err)
    }
    // Unpack returns into a []interface{}
    outputs, err := a.Methods[method].Outputs.Unpack(out)
    if err != nil {
        return nil, fmt.Errorf("unpack %s: %w", method, err)
    }
    return outputs, nil
}

