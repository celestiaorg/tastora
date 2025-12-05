package evm

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// Sender is a minimal EVM contract call helper that can pack ABI calls and send
// transactions to an RPC endpoint using a provided private key.
type Sender struct {
	client  *ethclient.Client
	chainID *big.Int
}

// NewSender dials the given RPC URL and resolves the chain ID.
func NewSender(ctx context.Context, rpcURL string) (*Sender, error) {
	c, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial eth rpc: %w", err)
	}
	chainID, err := c.ChainID(ctx)
	if err != nil {
		return nil, fmt.Errorf("get chain id: %w", err)
	}
	return &Sender{client: c, chainID: chainID}, nil
}

// Close closes the underlying client.
func (s *Sender) Close() {
	s.client.Close()
}

// SendFunctionTx packs the given ABI method and args and sends a transaction to the
// provided contract address using the supplied private key (hex, with or without 0x).
func (s *Sender) SendFunctionTx(ctx context.Context, privKeyHex, contractAddress string, abiJSON []byte, method string, args ...interface{}) (common.Hash, error) {
	data, err := packFunctionCall(abiJSON, method, args...)
	if err != nil {
		return common.Hash{}, err
	}
	return s.SendCalldataTx(ctx, privKeyHex, contractAddress, data)
}

// SendCalldataTx sends arbitrary calldata to a contract address using the supplied private key.
func (s *Sender) SendCalldataTx(ctx context.Context, privKeyHex, contractAddress string, data []byte) (common.Hash, error) {
	pk, err := parseHexPrivKey(privKeyHex)
	if err != nil {
		return common.Hash{}, err
	}
	fromAddr := crypto.PubkeyToAddress(pk.PublicKey)

	nonce, err := s.client.PendingNonceAt(ctx, fromAddr)
	if err != nil {
		return common.Hash{}, fmt.Errorf("pending nonce: %w", err)
	}

	to := common.HexToAddress(contractAddress)
	gasPrice, err := s.client.SuggestGasPrice(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("suggest gas price: %w", err)
	}

	msg := ethereum.CallMsg{From: fromAddr, To: &to, Data: data, GasPrice: gasPrice}
	gasLimit, err := s.client.EstimateGas(ctx, msg)
	if err != nil {
		// fallback to a sane default if estimation fails
		gasLimit = 300_000
	}

	// build legacy tx suitable for local/test networks
	tx := types.NewTransaction(nonce, to, big.NewInt(0), gasLimit, gasPrice, data)
	signer := types.LatestSignerForChainID(s.chainID)
	signed, err := types.SignTx(tx, signer, pk)
	if err != nil {
		return common.Hash{}, fmt.Errorf("sign tx: %w", err)
	}
	if err := s.client.SendTransaction(ctx, signed); err != nil {
		return common.Hash{}, fmt.Errorf("send tx: %w", err)
	}

	return signed.Hash(), nil
}

// packFunctionCall encodes an ABI method call with the given args.
func packFunctionCall(abiJSON []byte, method string, args ...interface{}) ([]byte, error) {
	a, err := abi.JSON(bytes.NewReader(abiJSON))
	if err != nil {
		return nil, fmt.Errorf("parse abi: %w", err)
	}
	data, err := a.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack %s: %w", method, err)
	}
	return data, nil
}

// parseHexPrivKey parses a hex private key string into an ECDSA key.
func parseHexPrivKey(h string) (*ecdsa.PrivateKey, error) {
	if len(h) > 1 && (h[:2] == "0x" || h[:2] == "0X") {
		h = h[2:]
	}
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, fmt.Errorf("decode hex key: %w", err)
	}
	pk, err := crypto.ToECDSA(b)
	if err != nil {
		return nil, fmt.Errorf("to ecdsa: %w", err)
	}
	return pk, nil
}
