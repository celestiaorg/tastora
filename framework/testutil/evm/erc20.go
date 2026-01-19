package evm

import (
	"context"
	"github.com/celestiaorg/tastora/framework/testutil/evm/internal"
	"github.com/ethereum/go-ethereum"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"math/big"
)

// GetERC20Balance queries the ERC20 balance of an account for a given token contract
func GetERC20Balance(ctx context.Context, client *ethclient.Client, tokenAddress, account gethcommon.Address) (*big.Int, error) {
	data, err := internal.ERC20QueryABI.Pack("balanceOf", account)
	if err != nil {
		return nil, err
	}

	msg := ethereum.CallMsg{
		To:   &tokenAddress,
		Data: data,
	}

	result, err := client.CallContract(ctx, msg, nil)
	if err != nil {
		return nil, err
	}

	var balance *big.Int
	err = internal.ERC20QueryABI.UnpackIntoInterface(&balance, "balanceOf", result)
	if err != nil {
		return nil, err
	}

	return balance, nil
}
