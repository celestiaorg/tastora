package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/celestiaorg/go-square/v2/share"
	"github.com/celestiaorg/tastora/framework/types"
	"io"
	"net/http"
	"strconv"
)

type HeaderResult struct {
	Header Header `json:"header"`
}

type Header struct {
	Height string `json:"height"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// GetHeader fetches a header for the given block height from the DANode via an RPC call and returns it.
func (n *DANode) GetHeader(ctx context.Context, height uint64) (types.Header, error) {
	url := fmt.Sprintf("http://%s", n.hostRPCPort)

	result, err := callRPC[HeaderResult](ctx, url, "header.GetByHeight", []uint64{height})
	if err != nil {
		return types.Header{}, err
	}

	h, err := strconv.Atoi(result.Header.Height)
	if err != nil {
		return types.Header{}, fmt.Errorf("failed to parse header height: %w", err)
	}

	return types.Header{Height: uint64(h)}, nil
}

// GetAllBlobs retrieves all blobs from the node for the specified height and namespaces via an RPC call.
func (n *DANode) GetAllBlobs(ctx context.Context, height uint64, namespaces []share.Namespace) ([]types.Blob, error) {
	url := fmt.Sprintf("http://%s", n.hostRPCPort)
	result, err := callRPC[[]types.Blob](ctx, url, "blob.GetAll", []any{height, namespaces})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch blobs: %w", err)
	}
	return result, nil
}

// GetP2PInfo retrieves the p2p information of the node, such as PeerID and Addresses, via an RPC call.
func (n *DANode) GetP2PInfo(ctx context.Context) (types.P2PInfo, error) {
	url := fmt.Sprintf("http://%s", n.hostRPCPort)
	p2pInfo, err := callRPC[types.P2PInfo](ctx, url, "p2p.Info", []any{})
	if err != nil {
		return types.P2PInfo{}, fmt.Errorf("failed to fetch p2p info: %w", err)
	}
	return p2pInfo, nil
}

// RPCResponse is a Generic RPC response.
type RPCResponse[T any] struct {
	Result T         `json:"result"`
	Error  *RPCError `json:"error"`
}

// callRPC sends a JSON-RPC POST request and unmarshals the result into a typed response.
func callRPC[T any](ctx context.Context, url, method string, params interface{}) (T, error) {
	var zero T

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return zero, fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return zero, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return zero, fmt.Errorf("unexpected status code: %s, body: %s", resp.Status, data)
	}

	var rpcResp RPCResponse[T]
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return zero, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	if rpcResp.Error != nil {
		return zero, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}
