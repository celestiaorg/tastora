package broadcast

import (
	"context"
	"github.com/celestiaorg/go-square/v2/share"
	"github.com/celestiaorg/tastora/framework/docker"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Messages broadcasts the provided messages to the first node signed on behalf of the given signer.
func Messages(ctx context.Context, signer types.Wallet, chain *docker.Chain, msgs ...sdk.Msg) (sdk.TxResponse, error) {
	return MessagesForNode(ctx, signer, chain, nil, msgs...)
}

// MessagesForNode broadcasts the provided messages to the target node signed on behalf of the given signer.
func MessagesForNode(ctx context.Context, signer types.Wallet, chain *docker.Chain, chainNode *docker.ChainNode, msgs ...sdk.Msg) (sdk.TxResponse, error) {
	b := docker.NewBroadcasterForNode(chain, chainNode)
	return b.BroadcastMessages(ctx, signer, msgs...)
}

// BlobMessage broadcasts the provided message to the first node signed on behalf of the given signer. The transaction bytes are wrapped
// using the MarshalBlobTx function before broadcasting.
func BlobMessage(ctx context.Context, signer types.Wallet, chain *docker.Chain, msg sdk.Msg, blobs ...*share.Blob) (sdk.TxResponse, error) {
	return BlobMessageForNode(ctx, signer, chain, nil, msg, blobs...)
}

// BlobMessageForNode broadcasts the provided message to the target node signed on behalf of the given signer. The transaction bytes are wrapped
// using the MarshalBlobTx function before broadcasting.
func BlobMessageForNode(ctx context.Context, signer types.Wallet, chain *docker.Chain, chainNode *docker.ChainNode, msg sdk.Msg, blobs ...*share.Blob) (sdk.TxResponse, error) {
	b := docker.NewBroadcasterForNode(chain, chainNode)
	return b.BroadcastBlobMessage(ctx, signer, msg, blobs...)
}
