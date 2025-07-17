package docker

import (
	"bytes"
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
)

var _ types.DANode = &DANode{}

const (
	// Default port numbers (without /tcp suffix)
	defaultDANodeRPCPort = "26658" // Default RPC port for DANode
	defaultDANodeP2PPort = "2121"  // Default P2P port for DANode
	defaultCoreRPCPort   = "26657" // Default RPC port for Core
	defaultCoreGRPCPort  = "9090"  // Default GRPC port for Core
)

// newDANode initializes and returns a new DANode instance using the provided context, test name, and configuration.
func newDANode(ctx context.Context, testName string, cfg Config, idx int, nodeType types.DANodeType) (*DANode, error) {
	if cfg.DataAvailabilityNetworkConfig == nil {
		return nil, fmt.Errorf("data availability network config is nil")
	}

	defaultImage := cfg.DataAvailabilityNetworkConfig.Image

	logger := cfg.Logger.With(
		zap.String("node_type", nodeType.String()),
	)
	daNode := &DANode{
		cfg:      cfg,
		nodeType: nodeType,
		Node: container.NewNode(cfg.DockerNetworkID, cfg.DockerClient, testName, defaultImage, "/home/celestia", idx, nodeType.String(), logger),
	}

	daNode.SetContainerLifecycle(container.NewLifecycle(cfg.Logger, cfg.DockerClient, daNode.Name()))

	// image may be overridden by each node, update Node with the final image
	daNode.Image = daNode.getImage()

	// create and setup volume using shared logic
	if err := daNode.CreateAndSetupVolume(ctx, daNode.Name()); err != nil {
		return nil, err
	}

	return daNode, nil
}

// DANode is a docker implementation of a celestia da node.
type DANode struct {
	*container.Node
	cfg            Config
	mu             sync.Mutex
	hasBeenStarted bool
	nodeType       types.DANodeType
	wallet         types.Wallet
	// adminAuthToken is a token that has admin access, it should be generated after init.
	adminAuthToken string
	// ports that are resolvable from the test runners themselves.
	hostRPCPort string
	hostP2PPort string
}

func (n *DANode) GetWallet() (types.Wallet, error) {
	return n.wallet, nil
}

func (n *DANode) GetAuthToken() (string, error) {
	if n.adminAuthToken == "" {
		return "", fmt.Errorf("admin token has not yet been generated for da node: %s", n.Name())
	}
	return n.adminAuthToken, nil
}

func (n *DANode) GetInternalHostName() (string, error) {
	return n.HostName(), nil
}

// GetInternalRPCAddress returns the internal RPC address resolvable within the network
func (n *DANode) GetInternalRPCAddress() (string, error) {
	rpcPort := strings.TrimSuffix(n.getRPCPort(), "/tcp")
	return fmt.Sprintf("%s:%s", n.HostName(), rpcPort), nil
}

// GetInternalP2PAddress returns the internal P2P address resolvable within the network
func (n *DANode) GetInternalP2PAddress() (string, error) {
	p2pPort := strings.TrimSuffix(n.getP2PPort(), "/tcp")
	return fmt.Sprintf("%s:%s", n.HostName(), p2pPort), nil
}

// GetType returns the type of the DANode as defined by the types.DANodeType enum.
func (n *DANode) GetType() types.DANodeType {
	return n.nodeType
}

// Name returns the container name for the DANode.
func (n *DANode) Name() string {
	return fmt.Sprintf("da-%s-%d-%s", n.nodeType.String(), n.Index, SanitizeContainerName(n.TestName))
}

// HostName returns the condensed hostname for the DANode.
func (n *DANode) HostName() string {
	return CondenseHostName(n.Name())
}

// GetHostRPCAddress returns the externally resolvable RPC address of the bridge node.
func (n *DANode) GetHostRPCAddress() string {
	return n.hostRPCPort
}

// Stop terminates the DANode by stopping its associated container gracefully using the provided context.
func (n *DANode) Stop(ctx context.Context) error {
	return n.StopContainer(ctx)
}

// Start initializes and starts the DANode with the provided core IP and genesis hash in the given context.
// It returns an error if the node initialization or startup fails.
func (n *DANode) Start(ctx context.Context, opts ...types.DANodeStartOption) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// if the container has already been started, we just start the container with existing settings.
	if n.hasBeenStarted {
		return n.StartContainer(ctx)
	}
	return n.startAndInitialize(ctx, opts...)
}

func (n *DANode) startAndInitialize(ctx context.Context, opts ...types.DANodeStartOption) error {
	startOpts := types.DANodeStartOptions{
		ChainID: "test",
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

	if err := n.initNode(ctx, startOpts.ChainID, env); err != nil {
		return fmt.Errorf("failed to initialize da node: %w", err)
	}

	if err := n.initAuthToken(ctx); err != nil {
		return fmt.Errorf("failed to initialize auth token: %w", err)
	}

	if err := n.startNode(ctx, startOpts.StartArguments, startOpts.ConfigModifications, env); err != nil {
		return fmt.Errorf("failed to start da node: %w", err)
	}

	n.hasBeenStarted = true
	return nil
}

// ModifyConfigFiles modifies the specified config files with the provided TOML modifications.
func (n *DANode) ModifyConfigFiles(ctx context.Context, configModifications map[string]toml.Toml) error {
	for filePath, modifications := range configModifications {
		if err := ModifyConfigFile(
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

// startNode initializes and starts the DANode container and updates its configuration based on the provided options.
func (n *DANode) startNode(ctx context.Context, additionalStartArgs []string, configModifications map[string]toml.Toml, env []string) error {
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
	hostPorts, err := n.ContainerLifecycle.GetHostPorts(ctx, n.getRPCPort(), n.getP2PPort())
	if err != nil {
		return err
	}

	n.hostRPCPort, n.hostP2PPort = hostPorts[0], hostPorts[1]
	return nil
}

// initNode initializes the DANode by running the "init" command for the specified DANode type, network, and keyring settings.
func (n *DANode) initNode(ctx context.Context, chainID string, env []string) error {
	if err := n.createWallet(ctx); err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}

	// note: my_celes_key is the default key name for the da node.
	cmd := []string{"celestia", n.nodeType.String(), "init", "--p2p.network", chainID, "--keyring.keyname", "my-key", "--node.store", n.HomeDir()}
	_, _, err := n.Exec(ctx, n.Logger, cmd, env)
	return err
}

// createWallet creates a wallet for use on this node. Creating one explicitly
// gives us access to the address for use in tests.
func (n *DANode) createWallet(ctx context.Context) error {
	cmd := []string{"cel-key", "add", "my-key", "--node.type", n.nodeType.String(), "--keyring-dir", path.Join(n.HomeDir(), "keys"), "--output", "json"}
	_, stderr, err := n.Exec(ctx, n.Logger, cmd, nil)
	if err != nil {
		return fmt.Errorf("failed to create wallet: %w", err)
	}

	address := extractAddressFromCreateWalletOutput(string(stderr))
	w := NewWallet(nil, address, "celestia", "my-key")
	n.wallet = &w
	return nil
}

// extractAddressFromCreateWalletOutput extracts the address from the output of the create wallet command.
// since the output is not fully structured we still need to parse the string.
//
// Sample output of the create wallet command:
//
// Starting Celestia Node with command:
// cel-key add my-key --node.type bridge --output json
//
// using directory:  /home/celestia/keys
// {"name":"my-key","type":"local","address":"celestia1y7qyj3mxzjxun02nf7s7msh4u8fnqyegtxwp6e","pubkey":"{\"@type\":\"/cosmos.crypto.secp256k1.PubKey\",\"key\":\"A612x2rTnch9iYXdzZ3EBBnpR9MLREhZyG+G2ox+uct1\"}","mnemonic":"slender night portion collect oyster kitten zone require tower have glance mixture siege turn text convince worry wagon aim jar ceiling harbor second jealous"}
func extractAddressFromCreateWalletOutput(output string) string {
	re := regexp.MustCompile(`"address"\s*:\s*"([^"]+)"`)
	matches := re.FindStringSubmatch(output)
	if len(matches) < 2 {
		panic("address not found")
	}
	return matches[1]
}

// createNodeContainer creates and initializes a container for the DANode with specified context, options, and environment variables.
func (n *DANode) createNodeContainer(ctx context.Context, additionalStartArgs []string, env []string) error {
	cmd := []string{"celestia", n.nodeType.String(), "start"}
	cmd = append(cmd, additionalStartArgs...)
	usingPorts := n.getPortMap() // Use configurable port map
	return n.ContainerLifecycle.CreateContainer(ctx, n.TestName, n.NetworkID, n.getImage(), usingPorts, "", n.Bind(), nil, n.HostName(), cmd, env, []string{})
}

// initAuthToken initialises an admin auth token.
func (n *DANode) initAuthToken(ctx context.Context) error {
	// Command to generate admin token
	cmd := []string{"celestia", n.nodeType.String(), "auth", "admin"}

	// Run the command inside the container
	stdout, stderr, err := n.Exec(ctx, n.Logger, cmd, nil)
	if err != nil {
		return fmt.Errorf("failed to generate auth token (stderr=%q): %w", stderr, err)
	}

	n.adminAuthToken = string(bytes.TrimSpace(stdout))
	return nil
}

// getNodeConfig returns the per-node configuration if it exists
func (n *DANode) getNodeConfig() *DANodeConfig {
	cfg := n.cfg.DataAvailabilityNetworkConfig
	if cfg == nil {
		return nil
	}

	var configMap map[int]*DANodeConfig
	switch n.nodeType {
	case types.BridgeNode:
		configMap = cfg.BridgeNodeConfigs
	case types.FullNode:
		configMap = cfg.FullNodeConfigs
	case types.LightNode:
		configMap = cfg.LightNodeConfigs
	default:
		return nil
	}

	if configMap == nil {
		return nil
	}

	nodeConfig, ok := configMap[n.Index]
	if !ok {
		n.Logger.Debug("no node config found for node", zap.Int("index", n.Index), zap.String("type", n.nodeType.String()))
	}

	return nodeConfig
}

// getImage returns the Docker image for this node, preferring per-node config over the default image
func (n *DANode) getImage() container.Image {
	if nodeConfig := n.getNodeConfig(); nodeConfig != nil && nodeConfig.Image != nil {
		return *nodeConfig.Image
	}
	return n.Image
}

// getRPCPort returns the RPC port for this node, with per-node config taking priority over network defaults
func (n *DANode) getRPCPort() string {
	if nodeConfig := n.getNodeConfig(); nodeConfig != nil && nodeConfig.RPCPort != "" {
		return nodeConfig.RPCPort + "/tcp"
	}
	if n.cfg.DataAvailabilityNetworkConfig.DefaultRPCPort != "" {
		return n.cfg.DataAvailabilityNetworkConfig.DefaultRPCPort + "/tcp"
	}
	return defaultDANodeRPCPort + "/tcp"
}

// getP2PPort returns the P2P port for this node, with per-node config taking priority over network defaults
func (n *DANode) getP2PPort() string {
	if nodeConfig := n.getNodeConfig(); nodeConfig != nil && nodeConfig.P2PPort != "" {
		return nodeConfig.P2PPort + "/tcp"
	}
	if n.cfg.DataAvailabilityNetworkConfig.DefaultP2PPort != "" {
		return n.cfg.DataAvailabilityNetworkConfig.DefaultP2PPort + "/tcp"
	}
	return defaultDANodeP2PPort + "/tcp"
}

// getCoreRPCPort returns the core RPC port this node should connect to
func (n *DANode) getCoreRPCPort() string {
	if nodeConfig := n.getNodeConfig(); nodeConfig != nil && nodeConfig.CoreRPCPort != "" {
		return nodeConfig.CoreRPCPort
	}
	if n.cfg.DataAvailabilityNetworkConfig.DefaultCoreRPCPort != "" {
		return n.cfg.DataAvailabilityNetworkConfig.DefaultCoreRPCPort
	}
	return defaultCoreRPCPort
}

// getCoreGRPCPort returns the core GRPC port this node should connect to
func (n *DANode) getCoreGRPCPort() string {
	if nodeConfig := n.getNodeConfig(); nodeConfig != nil && nodeConfig.CoreGRPCPort != "" {
		return nodeConfig.CoreGRPCPort
	}
	if n.cfg.DataAvailabilityNetworkConfig.DefaultCoreGRPCPort != "" {
		return n.cfg.DataAvailabilityNetworkConfig.DefaultCoreGRPCPort
	}
	return defaultCoreGRPCPort
}

// getPortMap returns the port mapping for this node using configurable ports
func (n *DANode) getPortMap() nat.PortMap {
	return nat.PortMap{
		nat.Port(n.getRPCPort()): {},
		nat.Port(n.getP2PPort()): {},
	}
}

// disableRPCAuthModification provides a modification which disables RPC authentication so that the tests can use the endpoints without configuring auth.
func disableRPCAuthModification() map[string]toml.Toml {
	modifications := toml.Toml{
		"RPC": toml.Toml{
			"SkipAuth": true,
		},
	}
	return map[string]toml.Toml{"config.toml": modifications}
}
