package docker

import (
	"bytes"
	"context"
	"fmt"
	"github.com/celestiaorg/go-square/v2/share"
	squaretx "github.com/celestiaorg/go-square/v2/tx"
	dockerinternal "github.com/celestiaorg/tastora/framework/docker/internal"
	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/testutil/wait"
	"github.com/celestiaorg/tastora/framework/types"
	sdktx "github.com/cosmos/cosmos-sdk/client/tx"
	"go.uber.org/zap"
	"path"
	"reflect"
	"time"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
)

// broadcaster is responsible for broadcasting trasactions to docker chains.
type broadcaster struct {
	// buf stores the output sdk.TxResponse when broadcast.Tx is invoked.
	buf *bytes.Buffer
	// keyrings is a mapping of keyrings which point to a temporary test directory. The contents
	// of this directory are copied from the node container for the specific wallet.
	keyrings map[types.Wallet]keyring.Keyring

	// chain is a reference to the Chain instance which will be the target of the messages.
	chain *Chain
	// node is the specific node to broadcast through (defaults to chain.GetNode() if nil)
	node *ChainNode

	// factoryOptions is a slice of broadcast.FactoryOpt which enables arbitrary configuration of the tx.Factory.
	factoryOptions []types.FactoryOpt
	// clientContextOptions is a slice of broadcast.ClientContextOpt which enables arbitrary configuration of the client.Context.
	clientContextOptions []types.ClientContextOpt
}

// NewBroadcaster returns an instance of Broadcaster which can be used with broadcast.Tx to
// broadcast messages sdk messages.
func NewBroadcaster(chain *Chain) types.Broadcaster {
	return newBroadcasterForNode(chain, nil)
}

// newBroadcasterForNode returns an instance of Broadcaster that broadcasts through a specific node.
func newBroadcasterForNode(chain *Chain, node *ChainNode) types.Broadcaster {
	return &broadcaster{
		chain:    chain,
		node:     node,
		buf:      &bytes.Buffer{},
		keyrings: map[types.Wallet]keyring.Keyring{},
	}
}

// ConfigureFactoryOptions ensure the given configuration functions are run when calling GetFactory
// after all default options have been applied.
func (b *broadcaster) ConfigureFactoryOptions(opts ...types.FactoryOpt) {
	b.factoryOptions = append(b.factoryOptions, opts...)
}

// ConfigureClientContextOptions ensure the given configuration functions are run when calling GetClientContext
// after all default options have been applied.
func (b *broadcaster) ConfigureClientContextOptions(opts ...types.ClientContextOpt) {
	b.clientContextOptions = append(b.clientContextOptions, opts...)
}

// GetFactory returns an instance of tx.Factory that is configured with this Broadcaster's Chain
// and the provided wallet. ConfigureFactoryOptions can be used to specify arbitrary options to configure the returned
// factory.
func (b *broadcaster) GetFactory(ctx context.Context, wallet types.Wallet) (sdktx.Factory, error) {
	clientContext, err := b.GetClientContext(ctx, wallet)
	if err != nil {
		return sdktx.Factory{}, err
	}

	sdkAdd, err := sdkacc.AddressFromWallet(wallet)
	if err != nil {
		return sdktx.Factory{}, err
	}

	account, err := clientContext.AccountRetriever.GetAccount(clientContext, sdkAdd)
	if err != nil {
		return sdktx.Factory{}, err
	}

	f := b.defaultTxFactory(clientContext, account)
	for _, opt := range b.factoryOptions {
		f = opt(f)
	}
	return f, nil
}

// GetClientContext returns a client context that is configured with this Broadcaster's Chain and
// the provided wallet. ConfigureClientContextOptions can be used to configure arbitrary options to configure the returned
// client.Context.
func (b *broadcaster) GetClientContext(ctx context.Context, wallet types.Wallet) (client.Context, error) {
	cn := b.getNode()

	_, ok := b.keyrings[wallet]
	if !ok {
		containerKeyringDir := path.Join(cn.HomeDir(), "keyring-test")
		kr := dockerinternal.NewDockerKeyring(cn.DockerClient, cn.ContainerLifecycle.ContainerID(), containerKeyringDir, cn.EncodingConfig.Codec)
		b.keyrings[wallet] = kr
	}

	sdkAdd, err := sdkacc.AddressFromWallet(wallet)
	if err != nil {
		return client.Context{}, err
	}

	clientContext := b.defaultClientContext(wallet, sdkAdd)
	for _, opt := range b.clientContextOptions {
		clientContext = opt(clientContext)
	}
	return clientContext, nil
}

// GetTxResponseBytes returns the sdk.TxResponse bytes which returned from broadcast.Tx.
func (b *broadcaster) GetTxResponseBytes(ctx context.Context, wallet types.Wallet) ([]byte, error) {
	if b.buf == nil || b.buf.Len() == 0 {
		return nil, fmt.Errorf("empty buffer, transaction has not been executed yet")
	}
	return b.buf.Bytes(), nil
}

// UnmarshalTxResponseBytes accepts the sdk.TxResponse bytes and unmarshalls them into an
// instance of sdk.TxResponse.
func (b *broadcaster) UnmarshalTxResponseBytes(ctx context.Context, bytes []byte) (sdk.TxResponse, error) {
	resp := sdk.TxResponse{}
	if err := b.chain.cfg.ChainConfig.EncodingConfig.Codec.UnmarshalJSON(bytes, &resp); err != nil {
		return sdk.TxResponse{}, err
	}

	// persist nested errors such as ValidateBasic checks.
	code := resp.Code
	rawLog := resp.RawLog

	if code != 0 {
		return resp, fmt.Errorf("error in transaction (code: %d): raw_log: %s", code, rawLog)
	}

	return resp, nil
}

// getNode returns the node to use for broadcasting (specific node or chain default).
func (b *broadcaster) getNode() *ChainNode {
	if b.node != nil {
		return b.node
	}
	return b.chain.GetNode()
}

// defaultClientContext returns a default client context configured with the wallet as the sender.
func (b *broadcaster) defaultClientContext(fromWallet types.Wallet, sdkAdd sdk.AccAddress) client.Context {
	// initialize a clean buffer each time
	b.buf.Reset()
	kr := b.keyrings[fromWallet]
	cn := b.getNode()
	return cn.CliContext().
		WithOutput(b.buf).
		WithFrom(fromWallet.GetFormattedAddress()).
		WithFromAddress(sdkAdd).
		WithFromName(fromWallet.GetKeyName()).
		WithSkipConfirmation(true).
		WithAccountRetriever(AccountRetriever{chain: b.chain, prefix: b.chain.cfg.ChainConfig.Bech32Prefix}).
		WithKeyring(kr).
		WithBroadcastMode(flags.BroadcastSync).
		WithCodec(b.chain.cfg.ChainConfig.EncodingConfig.Codec)
}

// defaultTxFactory creates a new Factory with default configuration.
func (b *broadcaster) defaultTxFactory(clientCtx client.Context, account client.Account) sdktx.Factory {
	chainConfig := b.chain.cfg.ChainConfig
	return sdktx.Factory{}.
		WithAccountNumber(account.GetAccountNumber()).
		WithSequence(account.GetSequence()).
		WithSignMode(signing.SignMode_SIGN_MODE_DIRECT).
		WithGasAdjustment(chainConfig.GasAdjustment).
		WithGas(flags.DefaultGasLimit).
		WithGasPrices(chainConfig.GasPrices).
		WithMemo("celestia-test").
		WithTxConfig(clientCtx.TxConfig).
		WithAccountRetriever(clientCtx.AccountRetriever).
		WithKeybase(clientCtx.Keyring).
		WithChainID(clientCtx.ChainID).
		WithSimulateAndExecute(false)
}

// BroadcastBlobMessage uses the provided Broadcaster to broadcast all the provided message which will be signed
// by the Wallet provided. The transaction will be wrapped with MarshalBlobTx before being broadcast.
// The sdk.TxResponse and an error are returned.
func (b *broadcaster) BroadcastBlobMessage(ctx context.Context, signingWallet types.Wallet, msg sdk.Msg, blobs ...*share.Blob) (sdk.TxResponse, error) {
	cc, err := b.GetClientContext(ctx, signingWallet)
	if err != nil {
		return sdk.TxResponse{}, err
	}

	txf, err := b.GetFactory(ctx, signingWallet)
	if err != nil {
		return sdk.TxResponse{}, err
	}

	txf, err = txf.Prepare(cc)
	if err != nil {
		return sdk.TxResponse{}, err
	}

	txBuilder, err := txf.BuildUnsignedTx(msg)
	if err != nil {
		return sdk.TxResponse{}, err
	}

	if err = sdktx.Sign(ctx, txf, signingWallet.GetKeyName(), txBuilder, true); err != nil {
		return sdk.TxResponse{}, err
	}

	txBytes, err := cc.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return sdk.TxResponse{}, err
	}

	blobTx, err := squaretx.MarshalBlobTx(txBytes, blobs...)
	if err != nil {
		return sdk.TxResponse{}, err
	}

	res, err := cc.BroadcastTx(blobTx)
	if err != nil {
		return sdk.TxResponse{}, err
	}

	return getFullyPopulatedResponse(ctx, cc, res.TxHash)
}

// BroadcastMessages uses the provided Broadcaster to broadcast all the provided messages which will be signed
// by the Wallet provided. The sdk.TxResponse and an error are returned.
func (b *broadcaster) BroadcastMessages(ctx context.Context, signingWallet types.Wallet, msgs ...sdk.Msg) (sdk.TxResponse, error) {
	f, err := b.GetFactory(ctx, signingWallet)
	if err != nil {
		return sdk.TxResponse{}, err
	}

	cc, err := b.GetClientContext(ctx, signingWallet)
	if err != nil {
		return sdk.TxResponse{}, err
	}

	if err := sdktx.BroadcastTx(cc, f, msgs...); err != nil {
		return sdk.TxResponse{}, err
	}

	txBytes, err := b.GetTxResponseBytes(ctx, signingWallet)
	if err != nil {
		return sdk.TxResponse{}, err
	}

	err = wait.ForCondition(ctx, time.Second*30, time.Second*5, func() (bool, error) {
		var err error
		txBytes, err = b.GetTxResponseBytes(ctx, signingWallet)
		if err != nil {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return sdk.TxResponse{}, err
	}

	respWithTxHash, err := b.UnmarshalTxResponseBytes(ctx, txBytes)
	if err != nil {
		return sdk.TxResponse{}, err
	}
	var msgTypes []string
	for _, msg := range msgs {
		msgType := reflect.TypeOf(msg).Elem().Name()
		msgTypes = append(msgTypes, msgType)
	}

	b.chain.log.Info("broadcasted message",
		zap.String("wallet_address", signingWallet.GetFormattedAddress()),
		zap.Strings("message_types", msgTypes),
		zap.String("tx_hash", respWithTxHash.TxHash))

	return getFullyPopulatedResponse(ctx, cc, respWithTxHash.TxHash)
}

// getFullyPopulatedResponse returns a fully populated sdk.TxResponse.
// the QueryTx function is periodically called until a tx with the given hash
// has been included in a block.
func getFullyPopulatedResponse(ctx context.Context, cc client.Context, txHash string) (sdk.TxResponse, error) {
	var resp sdk.TxResponse
	err := wait.ForCondition(ctx, time.Second*60, time.Second*5, func() (bool, error) {
		fullyPopulatedTxResp, err := authtx.QueryTx(cc, txHash)
		if err != nil {
			return false, nil
		}

		resp = *fullyPopulatedTxResp
		return true, nil
	})
	return resp, err
}
