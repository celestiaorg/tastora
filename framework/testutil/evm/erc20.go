package evm

import (
	"context"
	"github.com/ethereum/go-ethereum"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	gethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const erc20ABI = `[{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`

// GetERC20Balance queries the ERC20 balance of an account for a given token contract
func GetERC20Balance(ctx context.Context, client *ethclient.Client, tokenAddress, account gethcommon.Address) (*big.Int, error) {
	parsedABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, err
	}

	data, err := parsedABI.Pack("balanceOf", account)
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
	err = parsedABI.UnpackIntoInterface(&balance, "balanceOf", result)
	if err != nil {
		return nil, err
	}

	return balance, nil
}
