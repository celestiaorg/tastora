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

// WalletCreator defines the interface for creating wallets and accessing the faucet wallet.
type WalletCreator interface {
	CreateWallet(ctx context.Context, keyName string, bech32Prefix string) (*types.Wallet, error)
	GetFaucetWallet() *types.Wallet
}

// MessageBroadcaster defines the interface for broadcasting messages to the blockchain.
type MessageBroadcaster interface {
	BroadcastMessages(ctx context.Context, signingWallet *types.Wallet, msgs ...sdk.Msg) (sdk.TxResponse, error)
}

// CreateAndFund creates a new test wallet, funds it using the faucet wallet, and returns the created wallet.
func CreateAndFund(
	ctx context.Context,
	keyNamePrefix string,
	coins sdk.Coins,
	bech32Prefix string,
	walletCreator WalletCreator,
	broadcaster MessageBroadcaster,
) (*types.Wallet, error) {
	keyName := fmt.Sprintf("%s-%s", keyNamePrefix, random.LowerCaseLetterString(6))
	wallet, err := walletCreator.CreateWallet(ctx, keyName, bech32Prefix)
	if err != nil {
		return nil, fmt.Errorf("failed to get source user wallet: %w", err)
	}

	fromAddr, err := sdkacc.AddressFromWallet(walletCreator.GetFaucetWallet())
	if err != nil {
		return nil, fmt.Errorf("invalid from address: %w", err)
	}

	toAddr, err := sdkacc.AddressFromWallet(wallet)
	if err != nil {
		return nil, fmt.Errorf("invalid to address: %w", err)
	}

	bankSend := banktypes.NewMsgSend(fromAddr, toAddr, coins)
	resp, err := broadcaster.BroadcastMessages(ctx, walletCreator.GetFaucetWallet(), bankSend)
	if err != nil {
		return nil, fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	if resp.Code != 0 {
		return nil, fmt.Errorf("error in bank send response: %s", resp.RawLog)
	}

	return wallet, nil
}
