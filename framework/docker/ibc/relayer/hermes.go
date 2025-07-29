package relayer

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	sdkmath "cosmossdk.io/math"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/ibc"
	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	dockerclient "github.com/moby/moby/client"
	"go.uber.org/zap"
)

const (
	hermesDefaultImage   = "ghcr.io/informalsystems/hermes"
	hermesDefaultVersion = "1.13.1"
	hermesDefaultUIDGID  = "2000:2000"
	hermesHomeDir        = "/home/hermes"
)

// ChannelCreationResponse represents the response from hermes create channel command
type ChannelCreationResponse struct {
	Result CreateChannelResult `json:"result"`
}

// CreateChannelResult holds channel information for both sides
type CreateChannelResult struct {
	ASide ChannelSide `json:"a_side"`
	BSide ChannelSide `json:"b_side"`
}

// ChannelSide captures the channel ID for each side
type ChannelSide struct {
	ChannelID string `json:"channel_id"`
}

// Hermes implements the IBC relayer interface using Hermes.
type Hermes struct {
	*container.Node

	// Configuration
	chains  map[string]types.Chain
	wallets map[string]types.Wallet

	// Runtime state
	started bool
}

// NewHermes creates a new Hermes relayer instance.
func NewHermes(ctx context.Context, dockerClient *dockerclient.Client, testName, networkID string, logger *zap.Logger) (*Hermes, error) {
	image := container.Image{
		Repository: hermesDefaultImage,
		Version:    hermesDefaultVersion,
		UIDGID:     hermesDefaultUIDGID,
	}

	node := container.NewNode(
		networkID,
		dockerClient,
		testName,
		image,
		hermesHomeDir,
		0, // index
		"hermes-relayer",
		logger,
	)

	// Create container lifecycle
	containerName := fmt.Sprintf("%s-hermes", testName)
	lifecycle := container.NewLifecycle(logger, dockerClient, containerName)
	node.SetContainerLifecycle(lifecycle)

	hermes := &Hermes{
		Node:    node,
		chains:  make(map[string]types.Chain),
		wallets: make(map[string]types.Wallet),
	}

	// Set the user for Hermes execution
	//hermes.SetUser(hermesDefaultUIDGID)

	// Create and setup volume for Hermes
	if err := hermes.CreateAndSetupVolume(ctx, "hermes"); err != nil {
		return nil, err
	}

	return hermes, nil
}

// Start starts the Hermes relayer.
func (h *Hermes) Start(ctx context.Context) error {
	if h.started {
		return nil
	}

	// TODO: Start Hermes Docker container as daemon
	h.started = true
	return nil
}

// Stop stops the Hermes relayer.
func (h *Hermes) Stop(ctx context.Context) error {
	if !h.started {
		return nil
	}

	if err := h.StopContainer(ctx); err != nil {
		return err
	}

	h.started = false
	return nil
}

// Init initializes the relayer configuration
func (h *Hermes) Init(ctx context.Context) error {
	return h.generateConfig(ctx)
}

// SetupWallets creates and funds relayer wallets on both chains
func (h *Hermes) SetupWallets(ctx context.Context, chainA, chainB types.Chain) error {
	// Create relayer wallet on chain A
	walletNameA := fmt.Sprintf("relayer-%s", chainA.GetChainID())
	relayerWalletA, err := chainA.CreateWallet(ctx, walletNameA)
	if err != nil {
		return fmt.Errorf("failed to create relayer wallet on chain A: %w", err)
	}

	// Create relayer wallet on chain B
	walletNameB := fmt.Sprintf("relayer-%s", chainB.GetChainID())
	relayerWalletB, err := chainB.CreateWallet(ctx, walletNameB)
	if err != nil {
		return fmt.Errorf("failed to create relayer wallet on chain B: %w", err)
	}

	// Fund both wallets from faucets
	err = h.fundRelayerWallet(ctx, chainA, relayerWalletA)
	if err != nil {
		return fmt.Errorf("failed to fund relayer wallet on chain A: %w", err)
	}

	err = h.fundRelayerWallet(ctx, chainB, relayerWalletB)
	if err != nil {
		return fmt.Errorf("failed to fund relayer wallet on chain B: %w", err)
	}

	// Configure relayer with wallets and chains
	if err := h.AddChain(chainA); err != nil {
		return fmt.Errorf("failed to add chain A to relayer: %w", err)
	}

	if err := h.AddChain(chainB); err != nil {
		return fmt.Errorf("failed to add chain B to relayer: %w", err)
	}

	if err := h.AddWallet(chainA.GetChainID(), relayerWalletA); err != nil {
		return fmt.Errorf("failed to add chain A wallet to relayer: %w", err)
	}

	if err := h.AddWallet(chainB.GetChainID(), relayerWalletB); err != nil {
		return fmt.Errorf("failed to add chain B wallet to relayer: %w", err)
	}

	return nil
}

// Connect establishes IBC connection between the two chains
func (h *Hermes) Connect(ctx context.Context, chainA, chainB types.Chain) error {
	// Create clients
	if err := h.CreateClients(ctx, chainA, chainB); err != nil {
		return fmt.Errorf("failed to create IBC clients: %w", err)
	}

	// Create connections
	if err := h.CreateConnections(ctx, chainA, chainB); err != nil {
		return fmt.Errorf("failed to create IBC connections: %w", err)
	}

	return nil
}

// CreateClients creates IBC clients on both chains.
func (h *Hermes) CreateClients(ctx context.Context, chainA, chainB types.Chain) error {
	cmd := []string{"hermes", "--json", "create", "client", "--host-chain", chainA.GetChainID(), "--reference-chain", chainB.GetChainID()}
	_, _, err := h.Exec(ctx, h.Logger, cmd, nil)
	if err != nil {
		return err
	}

	cmd = []string{"hermes", "--json", "create", "client", "--host-chain", chainB.GetChainID(), "--reference-chain", chainA.GetChainID()}
	_, _, err = h.Exec(ctx, h.Logger, cmd, nil)
	return err
}

// CreateConnections creates IBC connections between the chains.
func (h *Hermes) CreateConnections(ctx context.Context, chainA, chainB types.Chain) error {
	cmd := []string{"hermes", "create", "connection", "--a-chain", chainA.GetChainID(), "--b-chain", chainB.GetChainID()}
	_, _, err := h.Exec(ctx, h.Logger, cmd, nil)
	return err
}

// CreateChannel creates an IBC channel between the chains.
func (h *Hermes) CreateChannel(ctx context.Context, chainA, chainB types.Chain, opts ibc.CreateChannelOptions) (*ibc.Channel, error) {
	// Execute hermes create channel command
	cmd := []string{
		"hermes", "create", "channel",
		"--order", string(opts.Order),
		"--a-chain", chainA.GetChainID(),
		"--a-port", opts.SourcePortName,
		"--b-chain", chainB.GetChainID(),
		"--b-port", opts.DestPortName,
		"--channel-version", opts.Version,
	}
	stdout, _, err := h.Exec(ctx, h.Logger, cmd, nil)
	if err != nil {
		return nil, err
	}

	// Parse channel information from hermes output
	channel, err := h.parseCreateChannelOutput(string(stdout), opts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hermes create channel output: %w", err)
	}

	return channel, nil
}

// parseCreateChannelOutput parses the output from hermes create channel command
func (h *Hermes) parseCreateChannelOutput(output string, opts ibc.CreateChannelOptions) (*ibc.Channel, error) {
	// Extract channel IDs using the same approach as interchaintest
	channelA, channelB, err := h.getChannelIDsFromStdout([]byte(output))
	if err != nil {
		return nil, fmt.Errorf("failed to parse channel IDs: %w", err)
	}

	channel := &ibc.Channel{
		ChannelID:        channelA,
		CounterpartyID:   channelB,
		PortID:           opts.SourcePortName,
		CounterpartyPort: opts.DestPortName,
		Order:            opts.Order,
		Version:          opts.Version,
		State:            "OPEN",
	}

	return channel, nil
}

// getChannelIDsFromStdout extracts channel IDs from hermes stdout
func (h *Hermes) getChannelIDsFromStdout(stdout []byte) (string, string, error) {
	var channelResponse ChannelCreationResponse
	if err := json.Unmarshal(h.extractJSONResult(stdout), &channelResponse); err != nil {
		return "", "", fmt.Errorf("failed to unmarshal channel creation response: %w", err)
	}
	return channelResponse.Result.ASide.ChannelID, channelResponse.Result.BSide.ChannelID, nil
}

// extractJSONResult extracts the JSON result line from hermes output
func (h *Hermes) extractJSONResult(stdout []byte) []byte {
	stdoutLines := strings.Split(string(stdout), "\n")
	var jsonOutput string
	for _, line := range stdoutLines {
		if strings.Contains(line, "result") {
			jsonOutput = line
			break
		}
	}
	return []byte(jsonOutput)
}

// AddWallet adds a wallet for the specified chain ID to the relayer configuration.
func (h *Hermes) AddWallet(chainID string, wallet types.Wallet) error {
	h.wallets[chainID] = wallet
	return nil
}

// AddChain adds a chain to the relayer configuration.
func (h *Hermes) AddChain(chain types.Chain) error {
	h.chains[chain.GetChainID()] = chain
	return nil
}

// generateConfig creates the Hermes configuration file and writes it to the container
func (h *Hermes) generateConfig(ctx context.Context) error {
	// Collect chain configs from all added chains
	chainConfigs := make([]types.ChainConfig, 0, len(h.chains))
	for _, chain := range h.chains {
		chainConfigs = append(chainConfigs, chain.GetChainConfig())
	}

	// Generate Hermes config
	hermesConfig, err := NewHermesConfig(chainConfigs)
	if err != nil {
		return fmt.Errorf("failed to create hermes config: %w", err)
	}

	// Convert to TOML
	configTOML, err := hermesConfig.ToTOML()
	if err != nil {
		return fmt.Errorf("failed to marshal hermes config: %w", err)
	}

	// Write config to the container volume
	configPath := ".hermes/config.toml"
	err = h.WriteFile(ctx, configPath, configTOML)
	if err != nil {
		return fmt.Errorf("failed to write hermes config: %w", err)
	}

	h.Logger.Info("Hermes config written",
		zap.Int("config_size", len(configTOML)),
		zap.Int("chains_count", len(h.chains)),
		zap.String("file_path", path.Join(h.HomeDir(), configPath)),
		zap.String("config_content", string(configTOML)),
	)
	for chainID := range h.chains {
		h.Logger.Info("Chain configured", zap.String("chain_id", chainID))
	}

	return nil
}

// fundRelayerWallet funds a relayer wallet from a faucet wallet.
func (h *Hermes) fundRelayerWallet(ctx context.Context, chain types.Chain, relayerWallet types.Wallet) error {
	// Get the chain's faucet wallet and config
	faucet := chain.GetFaucetWallet()
	chainConfig := chain.GetChainConfig()

	// Get addresses from wallets
	fromAddr, err := sdkacc.AddressFromWallet(faucet)
	if err != nil {
		return fmt.Errorf("failed to get faucet address: %w", err)
	}

	toAddr, err := sdkacc.AddressFromWallet(relayerWallet)
	if err != nil {
		return fmt.Errorf("failed to get relayer wallet address: %w", err)
	}

	// Define amount to fund the relayer wallet (enough for relayer operations)
	// Use the chain's native denom from the config
	fundAmount := sdk.NewCoins(sdk.NewCoin(chainConfig.Denom, sdkmath.NewInt(10000000))) // 10 tokens

	// Create bank send message
	bankSend := banktypes.NewMsgSend(fromAddr, toAddr, fundAmount)

	// Broadcast the funding transaction
	resp, err := chain.BroadcastMessages(ctx, faucet, bankSend)
	if err != nil {
		return fmt.Errorf("failed to broadcast funding transaction: %w", err)
	}

	if resp.Code != 0 {
		return fmt.Errorf("funding transaction failed: %s", resp.RawLog)
	}

	return nil
}
