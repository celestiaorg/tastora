package internal

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"strings"
)

var ERC20QueryABI abi.ABI

func init() {
	a, err := abi.JSON(strings.NewReader(rawErc20QueryABI))
	if err != nil {
		panic(err)
	}
	ERC20QueryABI = a
}

const rawErc20QueryABI = `[
  {
    "constant": true,
    "inputs": [
      {
        "name": "account",
        "type": "address"
      }
    ],
    "name": "balanceOf",
    "outputs": [
      {
        "name": "",
        "type": "uint256"
      }
    ],
    "type": "function"
  }
]`
