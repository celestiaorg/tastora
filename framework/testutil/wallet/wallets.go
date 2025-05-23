package wallet

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/testutil/random"
	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// CreateAndFund creates a new test wallet, funds it using the faucet wallet, and returns the created wallet.
func CreateAndFund(
	ctx context.Context,
	keyNamePrefix string,
	coins sdk.Coins,
	chain types.Chain,
) (types.Wallet, error) {
	keyName := fmt.Sprintf("%s-%s", keyNamePrefix, random.LowerCaseLetterString(6))
	wallet, err := chain.CreateWallet(ctx, keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to get source user wallet: %w", err)
	}

	fromAddr, err := sdkacc.AddressFromWallet(chain.GetFaucetWallet())
	if err != nil {
		return nil, fmt.Errorf("invalid from address: %w", err)
	}

	toAddr, err := sdkacc.AddressFromWallet(wallet)
	if err != nil {
		return nil, fmt.Errorf("invalid to address: %w", err)
	}

	bankSend := banktypes.NewMsgSend(fromAddr, toAddr, coins)
	resp, err := chain.BroadcastMessages(ctx, chain.GetFaucetWallet(), bankSend)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("error in bank send response: %s", resp.RawLog)
	}

	return wallet, nil
}
