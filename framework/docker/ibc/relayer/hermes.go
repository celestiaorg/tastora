package relayer

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	sdkmath "cosmossdk.io/math"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/ibc"
	"github.com/celestiaorg/tastora/framework/testutil/sdkacc"
	"github.com/celestiaorg/tastora/framework/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/go-bip39"
	dockerclient "github.com/moby/moby/client"
	"go.uber.org/zap"
)

const (
	hermesDefaultImage   = "ghcr.io/informalsystems/hermes"
	hermesDefaultVersion = "1.13.1"
	hermesDefaultUIDGID  = "2000:2000"
	hermesHomeDir        = "/home/hermes"
)


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
func (h *Hermes) Init(ctx context.Context, chainA, chainB types.Chain) error {
	// register chains with relayer
	if err := h.AddChain(chainA); err != nil {
		return fmt.Errorf("failed to add chain A to relayer: %w", err)
	}

	if err := h.AddChain(chainB); err != nil {
		return fmt.Errorf("failed to add chain B to relayer: %w", err)
	}

	if err := h.generateConfig(ctx); err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	// TODO: implement
	//if err := h.validateConfig(ctx); err != nil {
	//	return fmt.Errorf("failed to validate config: %w", err)
	//}

	return nil
}

// SetupWallets creates keys on Hermes relayer and funds them from chain faucets
func (h *Hermes) SetupWallets(ctx context.Context, chainA, chainB types.Chain) error {
	// Create key for chain A on Hermes relayer
	keyNameA := fmt.Sprintf("relayer-%s", chainA.GetChainID())
	addressA, err := h.createRelayerKey(ctx, chainA.GetChainID(), keyNameA)
	if err != nil {
		return fmt.Errorf("failed to create relayer key for chain A: %w", err)
	}

	// Create key for chain B on Hermes relayer
	keyNameB := fmt.Sprintf("relayer-%s", chainB.GetChainID())
	addressB, err := h.createRelayerKey(ctx, chainB.GetChainID(), keyNameB)
	if err != nil {
		return fmt.Errorf("failed to create relayer key for chain B: %w", err)
	}

	// Fund the relayer addresses from chain faucets
	err = h.fundRelayerAddress(ctx, chainA, addressA)
	if err != nil {
		return fmt.Errorf("failed to fund relayer address on chain A: %w", err)
	}

	err = h.fundRelayerAddress(ctx, chainB, addressB)
	if err != nil {
		return fmt.Errorf("failed to fund relayer address on chain B: %w", err)
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
func (h *Hermes) CreateConnections(ctx context.Context, chainA, chainB types.Chain) (ibc.Connection, error) {
	cmd := []string{"hermes", "--json", "create", "connection", "--a-chain", chainA.GetChainID(), "--b-chain", chainB.GetChainID()}
	stdout, _, err := h.Exec(ctx, h.Logger, cmd, nil)
	if err != nil {
		return ibc.Connection{}, err
	}

	// Parse connection information from hermes output
	connection, err := h.parseCreateConnectionOutput(string(stdout))
	if err != nil {
		return ibc.Connection{}, fmt.Errorf("failed to parse hermes create connection output: %w", err)
	}

	return connection, nil
}

// CreateChannel creates an IBC channel between the chains.
func (h *Hermes) CreateChannel(ctx context.Context, chainA, chainB types.Chain, connection ibc.Connection, opts ibc.CreateChannelOptions) (*ibc.Channel, error) {
	// Validate that connection has a valid ID
	if connection.ConnectionID == "" {
		return nil, fmt.Errorf("invalid connection: connection ID is empty")
	}

	// Execute hermes create channel command with JSON output using existing connection
	cmd := []string{
		"hermes", "--json", "create", "channel",
		"--order", string(opts.Order),
		"--a-chain", chainA.GetChainID(),
		"--a-connection", connection.ConnectionID,
		"--a-port", opts.SourcePortName,
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

// parseCreateConnectionOutput parses the output from hermes create connection command
func (h *Hermes) parseCreateConnectionOutput(output string) (ibc.Connection, error) {
	// Extract connection IDs using the same approach as channel creation
	connectionA, connectionB, err := h.getConnectionIDsFromStdout([]byte(output))
	if err != nil {
		return ibc.Connection{}, fmt.Errorf("failed to parse connection IDs: %w", err)
	}

	// Validate that we got valid connection IDs
	if connectionA == "" {
		return ibc.Connection{}, fmt.Errorf("no connection ID found in output")
	}

	connection := ibc.Connection{
		ConnectionID:   connectionA,
		CounterpartyID: connectionB,
		State:          "OPEN",
	}

	return connection, nil
}

// getConnectionIDsFromStdout extracts connection IDs from hermes stdout
func (h *Hermes) getConnectionIDsFromStdout(stdout []byte) (string, string, error) {
	var connectionResponse ConnectionCreationResponse
	if err := json.Unmarshal(h.extractJSONResult(stdout), &connectionResponse); err != nil {
		return "", "", fmt.Errorf("failed to unmarshal connection creation response: %w", err)
	}
	return connectionResponse.Result.ASide.ConnectionID, connectionResponse.Result.BSide.ConnectionID, nil
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

// createRelayerKey creates a new key on the Hermes relayer for the specified chain
func (h *Hermes) createRelayerKey(ctx context.Context, chainID, keyName string) (string, error) {
	// Generate a new mnemonic
	mnemonic, err := h.generateMnemonic()
	if err != nil {
		return "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	// Write mnemonic to a temporary file in the container
	mnemonicFile := fmt.Sprintf(".hermes/%s_mnemonic.txt", keyName)
	err = h.WriteFile(ctx, mnemonicFile, []byte(mnemonic))
	if err != nil {
		return "", fmt.Errorf("failed to write mnemonic file: %w", err)
	}

	// Use hermes keys add command with mnemonic file and JSON output
	mnemonicPath := path.Join(h.HomeDir(), mnemonicFile)
	cmd := []string{"hermes", "--json", "keys", "add", "--chain", chainID, "--key-name", keyName, "--mnemonic-file", mnemonicPath}
	stdout, stderr, err := h.Exec(ctx, h.Logger, cmd, nil)
	if err != nil {
		h.Logger.Error("Failed to create relayer key",
			zap.String("chain_id", chainID),
			zap.String("key_name", keyName),
			zap.String("mnemonic_file", mnemonicPath),
			zap.String("stdout", string(stdout)),
			zap.String("stderr", string(stderr)),
			zap.Error(err))
		return "", fmt.Errorf("failed to create key %s for chain %s: %w", keyName, chainID, err)
	}

	// Extract the address from the command output
	address, err := h.parseAddressFromKeyOutput(string(stdout))
	if err != nil {
		return "", fmt.Errorf("failed to parse address from key creation output: %w", err)
	}

	h.Logger.Info("Created relayer key",
		zap.String("chain_id", chainID),
		zap.String("key_name", keyName),
		zap.String("address", address),
		zap.String("mnemonic_file", mnemonicPath))

	return address, nil
}

// generateMnemonic generates a new BIP39 mnemonic
func (h *Hermes) generateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(256) // 24 words
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %w", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	return mnemonic, nil
}

// parseAddressFromKeyOutput extracts the address from hermes key creation output
func (h *Hermes) parseAddressFromKeyOutput(output string) (string, error) {
	// Try to parse as JSON first (when --json flag is used)
	var jsonResult map[string]interface{}
	if err := json.Unmarshal([]byte(output), &jsonResult); err == nil {
		// JSON format - extract address from the result
		if result, ok := jsonResult["result"].(string); ok {
			// Use regex to find bech32 address in the result string
			return h.extractAddressWithRegex(result)
		}
	}

	// Fallback to text parsing for non-JSON output
	// Use regex to find any bech32 address pattern (works for any chain prefix)
	return h.extractAddressWithRegex(output)
}

// extractAddressWithRegex uses regex to find bech32 addresses
func (h *Hermes) extractAddressWithRegex(text string) (string, error) {
	// Bech32 address regex pattern: prefix + 1 + base32 characters (at least 38 chars total)
	// This will match addresses like: cosmos1..., celestia1..., osmo1..., etc.
	bech32Pattern := `([a-z]+1[a-z0-9]{38,})`
	re := regexp.MustCompile(bech32Pattern)
	
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		address := matches[1]
		// Remove any trailing punctuation that might have been captured
		address = strings.TrimRight(address, ".,;:!?)")
		return address, nil
	}
	
	return "", fmt.Errorf("could not find bech32 address in output: %s", text)
}

// fundRelayerAddress funds a relayer address from a faucet wallet.
func (h *Hermes) fundRelayerAddress(ctx context.Context, chain types.Chain, relayerAddress string) error {
	// Get the chain's faucet wallet and config
	faucet := chain.GetFaucetWallet()
	chainConfig := chain.GetChainConfig()

	// Get faucet address
	fromAddr, err := sdkacc.AddressFromWallet(faucet)
	if err != nil {
		return fmt.Errorf("failed to get faucet address: %w", err)
	}

	// Parse the relayer address
	toAddr, err := sdk.AccAddressFromBech32(relayerAddress)
	if err != nil {
		return fmt.Errorf("failed to parse relayer address %s: %w", relayerAddress, err)
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
