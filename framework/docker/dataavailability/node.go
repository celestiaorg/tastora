package dataavailability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"sync"

	"github.com/celestiaorg/go-square/v2/share"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/internal"
	dockerutil "github.com/celestiaorg/tastora/framework/testutil/docker"
	tomlutil "github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

// Default port numbers (without /tcp suffix)
const (
	defaultDANodeRPCPort = "26658" // Default RPC port for DANode
	defaultDANodeP2PPort = "2121"  // Default P2P port for DANode
	defaultCoreRPCPort   = "26657" // Default RPC port for Core
	defaultCoreGRPCPort  = "9090"  // Default GRPC port for Core
)

// initializeDANodePorts initializes internal ports with defaults and applies custom overrides
func initializeDANodePorts(customPorts types.Ports) types.Ports {
	// Start with defaults
	ports := types.Ports{
		RPC:      defaultDANodeRPCPort,
		P2P:      defaultDANodeP2PPort,
		CoreRPC:  defaultCoreRPCPort,
		CoreGRPC: defaultCoreGRPCPort,
	}

	// Apply custom overrides if provided
	if customPorts.RPC != "" {
		ports.RPC = customPorts.RPC
	}
	if customPorts.P2P != "" {
		ports.P2P = customPorts.P2P
	}
	if customPorts.CoreRPC != "" {
		ports.CoreRPC = customPorts.CoreRPC
	}
	if customPorts.CoreGRPC != "" {
		ports.CoreGRPC = customPorts.CoreGRPC
	}

	return ports
}

// Node is a docker implementation of a celestia da node.
type Node struct {
	*container.Node
	// cfg is the config for the entire Network.
	cfg                 Config
	mu                  sync.Mutex
	hasBeenStarted      bool
	nodeType            types.DANodeType
	additionalStartArgs []string
	configModifications map[string]tomlutil.Toml
	// internalPorts contains any overrides for the internal ports used by the node.
	internalPorts types.Ports
	wallet        *types.Wallet
	// adminAuthToken is a token that has admin access, it should be generated after init.
	adminAuthToken string
	// External ports that are resolvable from the test runners themselves.
	externalPorts types.Ports
}

func NewNode(cfg Config, testName string, image container.Image, index int, nodeConfig NodeConfig) *Node {
	logger := cfg.Logger.With(
		zap.String("node_type", nodeConfig.NodeType.String()),
	)
	node := &Node{
		cfg:                 cfg,
		nodeType:            nodeConfig.NodeType,
		additionalStartArgs: nodeConfig.AdditionalStartArgs,
		configModifications: nodeConfig.ConfigModifications,
		internalPorts:       initializeDANodePorts(nodeConfig.InternalPorts),
		Node:                container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, image, "/home/celestia", index, nodeConfig.NodeType, logger),
	}

	node.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, node.Name()))
	return node
}

// Name returns the container name for the Node.
func (n *Node) Name() string {
	return fmt.Sprintf("da-%s-%d-%s", n.nodeType.String(), n.Index, internal.SanitizeContainerName(n.TestName))
}

// HostName returns the condensed hostname for the Node.
func (n *Node) HostName() string {
	return internal.CondenseHostName(n.Name())
}

// GetType returns the type of the Node.
func (n *Node) GetType() types.DANodeType {
	return n.nodeType
}

// GetWallet returns the wallet associated with the node.
func (n *Node) GetWallet() (*types.Wallet, error) {
	if n.wallet == nil {
		// Safely handle the case where the node might not be fully initialized
		nodeName := "uninitialized-node"
		if n.Node != nil {
			nodeName = n.Name()
		}
		return nil, fmt.Errorf("wallet not initialized for node %s", nodeName)
	}
	return n.wallet, nil
}

// GetAuthToken returns the auth token for the node.
func (n *Node) GetAuthToken() (string, error) {
	if n.adminAuthToken == "" {
		return "", fmt.Errorf("admin token has not yet been generated for da node: %s", n.Name())
	}
	return n.adminAuthToken, nil
}

// GetNetworkInfo returns the network information for the DA node.
func (n *Node) GetNetworkInfo(ctx context.Context) (types.NetworkInfo, error) {
	internalIP, err := internal.GetContainerInternalIP(ctx, n.DockerClient, n.ContainerLifecycle.ContainerID())
	if err != nil {
		return types.NetworkInfo{}, err
	}

	return types.NetworkInfo{
		Internal: types.Network{
			Hostname: n.HostName(),
			IP:       internalIP,
			Ports:    n.internalPorts,
		},
		External: types.Network{
			Hostname: "0.0.0.0",
			Ports:    n.externalPorts,
		},
	}, nil
}

// Stop terminates the Node by stopping its associated container gracefully using the provided context.
func (n *Node) Stop(ctx context.Context) error {
	if err := n.StopContainer(ctx); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	if err := n.RemoveContainer(ctx); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	return nil
}

// Start initializes and starts the Node with the provided options in the given context.
// It returns an error if the node initialization or startup fails.
func (n *Node) Start(ctx context.Context, opts ...StartOption) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// if the container has already been started, we just start the container with existing settings.
	if n.hasBeenStarted {
		return n.StartContainer(ctx)
	}
	return n.startAndInitialize(ctx, opts...)
}

func (n *Node) startAndInitialize(ctx context.Context, opts ...StartOption) error {
	startOpts := StartOptions{
		ChainID: n.cfg.ChainID,
		// by default disable RPC authentication, if any custom overrides are applied,
		// they will need to explicitly disable rpc auth also if that is required.
		ConfigModifications: disableRPCAuthModification(),
	}

	for _, fn := range opts {
		fn(&startOpts)
	}

	var env []string
	for k, v := range startOpts.EnvironmentVariables {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// merge with node-level env vars
	env = append(env, n.cfg.Env...)

	if err := n.initNode(ctx, startOpts.ChainID, env); err != nil {
		return fmt.Errorf("failed to initialize da node: %w", err)
	}

	if err := n.initAuthToken(ctx); err != nil {
		return fmt.Errorf("failed to initialize auth token: %w", err)
	}

	// merge config modifications
	allConfigMods := make(map[string]tomlutil.Toml)
	for k, v := range n.configModifications {
		allConfigMods[k] = v
	}
	for k, v := range startOpts.ConfigModifications {
		allConfigMods[k] = v
	}

	// merge start arguments
	allStartArgs := append(n.additionalStartArgs, startOpts.StartArguments...)

	if err := n.startNode(ctx, allStartArgs, allConfigMods, env); err != nil {
		return fmt.Errorf("failed to start da node: %w", err)
	}

	n.hasBeenStarted = true
	return nil
}

// initNode initializes the Node by running the "init" command for the specified Node type, network, and keyring settings.
func (n *Node) initNode(ctx context.Context, chainID string, env []string) error {
	if err := n.createWallet(ctx); err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}

	// note: my_celes_key is the default key name for the da node.
	cmd := []string{n.cfg.Bin, n.nodeType.String(), "init", "--p2p.network", chainID, "--keyring.keyname", "my-key", "--node.store", n.HomeDir()}
	_, _, err := n.Exec(ctx, n.Logger, cmd, env)
	return err
}

// createWallet creates a wallet for use on this node. Creating one explicitly
// gives us access to the address for use in tests.
func (n *Node) createWallet(ctx context.Context) error {
	cmd := []string{"cel-key", "add", "my-key", "--node.type", n.nodeType.String(), "--keyring-dir", path.Join(n.HomeDir(), "keys"), "--output", "json"}
	_, stderr, err := n.Exec(ctx, n.Logger, cmd, nil)
	if err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}

	address := extractAddressFromCreateWalletOutput(string(stderr))
	n.wallet = types.NewWallet(nil, address, "celestia", "my-key")
	return nil
}

// extractAddressFromCreateWalletOutput extracts the address from the output of the create wallet command.
func extractAddressFromCreateWalletOutput(output string) string {
	re := regexp.MustCompile(`"address"\s*:\s*"([^"]+)"`)
	matches := re.FindStringSubmatch(output)
	if len(matches) < 2 {
		panic("address not found")
	}
	return matches[1]
}

// startNode initializes and starts the Node container and updates its configuration based on the provided options.
func (n *Node) startNode(ctx context.Context, additionalStartArgs []string, configModifications map[string]tomlutil.Toml, env []string) error {
	if err := n.createNodeContainer(ctx, additionalStartArgs, env); err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// apply any config modifications
	if err := n.ModifyConfigFiles(ctx, configModifications); err != nil {
		return fmt.Errorf("failed to apply config modifications: %w", err)
	}

	if err := n.ContainerLifecycle.StartContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Set the host ports once since they will not change after the container has started.
	hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, n.internalPorts.RPC+"/tcp", n.internalPorts.P2P+"/tcp")
	if err != nil {
		return err
	}

	n.externalPorts = types.Ports{
		RPC: internal.MustExtractPort(hostPorts[0]),
		P2P: internal.MustExtractPort(hostPorts[1]),
	}
	return nil
}

// createNodeContainer creates and initializes a container for the Node with specified context, options, and environment variables.
func (n *Node) createNodeContainer(ctx context.Context, additionalStartArgs []string, env []string) error {
	cmd := []string{n.cfg.Bin, n.nodeType.String(), "start"}
	cmd = append(cmd, additionalStartArgs...)
	usingPorts := n.getPortMap()
	return n.ContainerLifecycle.CreateContainer(ctx, n.TestName, n.NetworkID, n.cfg.Image, usingPorts, "", n.Bind(), nil, n.HostName(), cmd, env, []string{})
}

// initAuthToken initialises an admin auth token.
func (n *Node) initAuthToken(ctx context.Context) error {
	// Command to generate admin token
	cmd := []string{n.cfg.Bin, n.nodeType.String(), "auth", "admin"}

	// Run the command inside the container
	stdout, stderr, err := n.Exec(ctx, n.Logger, cmd, nil)
	if err != nil {
		return fmt.Errorf("failed to generate auth token (stderr=%q): %w", stderr, err)
	}

	n.adminAuthToken = string(bytes.TrimSpace(stdout))
	return nil
}

// getPortMap returns the port mapping for the node type.
func (n *Node) getPortMap() nat.PortMap {
	return nat.PortMap{
		nat.Port(n.internalPorts.RPC + "/tcp"): {},
		nat.Port(n.internalPorts.P2P + "/tcp"): {},
	}
}

// ModifyConfigFiles modifies the specified config files with the provided TOML modifications.
func (n *Node) ModifyConfigFiles(ctx context.Context, configModifications map[string]tomlutil.Toml) error {
	for filePath, modifications := range configModifications {
		if err := dockerutil.ModifyConfigFile(
			ctx,
			n.Logger,
			n.DockerClient,
			n.TestName,
			n.VolumeName,
			filePath,
			modifications,
		); err != nil {
			return fmt.Errorf("failed to modify %s: %w", filePath, err)
		}
	}
	return nil
}

// disableRPCAuthModification returns TOML modifications to disable RPC authentication
func disableRPCAuthModification() map[string]tomlutil.Toml {
	return map[string]tomlutil.Toml{
		"config.toml": {
			"RPC": tomlutil.Toml{
				"SkipAuth": true,
			},
		},
	}
}

// JSON RPC method implementations

// GetHeader fetches a header for the given block height from the DANode via an RPC call and returns it.
func (n *Node) GetHeader(ctx context.Context, height uint64) (types.Header, error) {
	url := fmt.Sprintf("http://0.0.0.0:%s", n.externalPorts.RPC)

	result, err := callRPC[HeaderResult](ctx, url, "header.GetByHeight", []uint64{height})
	if err != nil {
		return types.Header{}, err
	}

	h, err := strconv.Atoi(result.Header.Height)
	if err != nil {
		return types.Header{}, fmt.Errorf("failed to parse header height: %w", err)
	}

	return types.Header{Height: uint64(h)}, nil
}

// GetAllBlobs retrieves all blobs from the node for the specified height and namespaces via an RPC call.
func (n *Node) GetAllBlobs(ctx context.Context, height uint64, namespaces []share.Namespace) ([]types.Blob, error) {
	url := fmt.Sprintf("http://0.0.0.0:%s", n.externalPorts.RPC)
	result, err := callRPC[[]types.Blob](ctx, url, "blob.GetAll", []any{height, namespaces})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch blobs: %w", err)
	}
	return result, nil
}

// GetP2PInfo retrieves the p2p information of the node, such as PeerID and Addresses, via an RPC call.
func (n *Node) GetP2PInfo(ctx context.Context) (types.P2PInfo, error) {
	url := fmt.Sprintf("http://0.0.0.0:%s", n.externalPorts.RPC)
	p2pInfo, err := callRPC[types.P2PInfo](ctx, url, "p2p.Info", []any{})
	if err != nil {
		return types.P2PInfo{}, fmt.Errorf("failed to fetch p2p info: %w", err)
	}
	return p2pInfo, nil
}

// Supporting types for JSON RPC

type HeaderResult struct {
	Header Header `json:"header"`
}

type Header struct {
	Height string `json:"height"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// RPCResponse is a Generic RPC response.
type RPCResponse[T any] struct {
	Result T         `json:"result"`
	Error  *RPCError `json:"error"`
}

// callRPC sends a JSON-RPC POST request and unmarshals the result into a typed response.
func callRPC[T any](ctx context.Context, url, method string, params interface{}) (T, error) {
	var zero T

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return zero, fmt.Errorf("failed to marshal JSON payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return zero, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return zero, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return zero, fmt.Errorf("unexpected status code: %s, body: %s", resp.Status, data)
	}

	var rpcResp RPCResponse[T]
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return zero, fmt.Errorf("failed to decode JSON response: %w", err)
	}

	if rpcResp.Error != nil {
		return zero, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}
