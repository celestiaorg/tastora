package docker

import (
	"context"
	"cosmossdk.io/math"
	"fmt"
	"github.com/celestiaorg/tastora/framework/testutil/random"
	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"testing"
)

// CreateAndFundTestWallet creates a new test wallet, funds it using the faucet wallet, and returns the created wallet.
func CreateAndFundTestWallet(
	t *testing.T,
	ctx context.Context,
	keyNamePrefix string,
	amount math.Int,
	chain *Chain,
) (types.Wallet, error) {
	t.Helper()
	chainCfg := chain.cfg.ChainConfig
	keyName := fmt.Sprintf("%s-%s-%s", keyNamePrefix, chainCfg.ChainID, random.LowerCaseLetterString(3))
	wallet, err := chain.CreateWallet(ctx, keyName)
	if err != nil {
		return nil, fmt.Errorf("failed to get source user wallet: %w", err)
	}

	fromAddr, err := sdkacc.AddressFromBech32(chain.faucetWallet.GetFormattedAddress(), chainCfg.Bech32Prefix)
	if err != nil {
		return nil, fmt.Errorf("invalid from address: %w", err)
	}

	toAddr, err := sdkacc.AddressFromBech32(wallet.GetFormattedAddress(), chainCfg.Bech32Prefix)
	if err != nil {
		return nil, fmt.Errorf("invalid to address: %w", err)
	}

	bankSend := banktypes.NewMsgSend(fromAddr, toAddr, sdk.NewCoins(sdk.NewCoin(chainCfg.Denom, amount)))
	resp, err := chain.BroadcastMessages(ctx, chain.faucetWallet, bankSend)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("error in bank send response: %s", resp.RawLog)
	}

	return wallet, nil
}
