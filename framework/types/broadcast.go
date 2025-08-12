package types

import (
	"context"
	"github.com/celestiaorg/go-square/v2/share"
	"github.com/cosmos/cosmos-sdk/client"
	sdktx "github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type ClientContextOpt func(clientContext client.Context) client.Context

type FactoryOpt func(factory sdktx.Factory) sdktx.Factory

// Broadcaster specifies the functionality for a type being able to broadcast sdk messages
// and sign them on behalf of arbitrary users.
type Broadcaster interface {
	// BroadcastMessages broadcasts the given messages signed on behalf of the provided user.
	BroadcastMessages(ctx context.Context, signingWallet Wallet, msgs ...sdk.Msg) (sdk.TxResponse, error)
	// BroadcastBlobMessage broadcasts the given messages signed on behalf of the provided user. The transaction bytes are wrapped
	// using the MarshalBlobTx function before broadcasting.
	BroadcastBlobMessage(ctx context.Context, signingWallet Wallet, msg sdk.Msg, blobs ...*share.Blob) (sdk.TxResponse, error)
	// ConfigureFactoryOptions applies a list of functional options to modify the transaction factory configuration.
	ConfigureFactoryOptions(opts ...FactoryOpt)
	// ConfigureClientContextOptions applies a list of functional options to modify the client context configuration.
	ConfigureClientContextOptions(opts ...ClientContextOpt)
}
