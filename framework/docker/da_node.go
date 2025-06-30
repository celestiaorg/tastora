package docker

import (
	"bytes"
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
	"path"
	"regexp"
	"sync"
)

var _ types.DANode = &DANode{}

const (
	daNodeRPCPort = "26658/tcp"
	daNodeP2PPort = "2121/tcp"
)

// daNodePorts defines the default port mappings for the DANode's RPC and P2P communication.
var daNodePorts = nat.PortMap{
	nat.Port(daNodeRPCPort): {},
	nat.Port(daNodeP2PPort): {},
}

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
		cfg:           cfg,
		nodeType:      nodeType,
		ContainerNode: newContainerNode(cfg.DockerNetworkID, cfg.DockerClient, testName, defaultImage, "/home/celestia", idx, nodeType.String(), logger),
	}

	daNode.containerLifecycle = NewContainerLifecycle(cfg.Logger, cfg.DockerClient, daNode.Name())

	// image may be overridden by each node.
	daNode.Image = daNode.getImage()
	if err := daNode.createAndSetupVolume(ctx); err != nil {
		return nil, fmt.Errorf("failed to create and setup volume: %w", err)
	}

	return daNode, nil
}

// DANode is a docker implementation of a celestia da node.
type DANode struct {
	*ContainerNode
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

// GetType returns the type of the DANode as defined by the types.DANodeType enum.
func (n *DANode) GetType() types.DANodeType {
	return n.nodeType
}

// GetHostRPCAddress returns the externally resolvable RPC address of the bridge node.
func (n *DANode) GetHostRPCAddress() string {
	return n.hostRPCPort
}

// Stop terminates the DANode by stopping its associated container gracefully using the provided context.
func (n *DANode) Stop(ctx context.Context) error {
	return n.stopContainer(ctx)
}

// Start initializes and starts the DANode with the provided core IP and genesis hash in the given context.
// It returns an error if the node initialization or startup fails.
func (n *DANode) Start(ctx context.Context, opts ...types.DANodeStartOption) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// if the container has already been started, we just start the container with existing settings.
	if n.hasBeenStarted {
		return n.startContainer(ctx)
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

// Name of the test node container.
func (n *ContainerNode) Name() string {
	return fmt.Sprintf("%s-%d-%s", n.GetType(), n.Index, SanitizeContainerName(n.TestName))
}

// HostName of the test node container.
func (n *ContainerNode) HostName() string {
	return CondenseHostName(n.Name())
}

// ModifyConfigFiles modifies the specified config files with the provided TOML modifications.
func (n *DANode) ModifyConfigFiles(ctx context.Context, configModifications map[string]toml.Toml) error {
	for filePath, modifications := range configModifications {
		if err := ModifyConfigFile(
			ctx,
			n.logger,
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

	if err := n.containerLifecycle.StartContainer(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Set the host ports once since they will not change after the container has started.
	hostPorts, err := n.containerLifecycle.GetHostPorts(ctx, daNodeRPCPort, daNodeP2PPort)
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
	cmd := []string{"celestia", n.nodeType.String(), "init", "--p2p.network", chainID, "--keyring.keyname", "my-key", "--node.store", n.homeDir}
	_, _, err := n.exec(ctx, n.logger, cmd, env)
	return err
}

// createWallet creates a wallet for use on this node. Creating one explicitly
// gives us access to the address for use in tests.
func (n *DANode) createWallet(ctx context.Context) error {
	cmd := []string{"cel-key", "add", "my-key", "--node.type", n.nodeType.String(), "--keyring-dir", path.Join(n.homeDir, "keys"), "--output", "json"}
	_, stderr, err := n.exec(ctx, n.logger, cmd, nil)
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
	usingPorts := nat.PortMap{}
	for k, v := range daNodePorts {
		usingPorts[k] = v
	}
	return n.containerLifecycle.CreateContainer(ctx, n.TestName, n.NetworkID, n.getImage(), usingPorts, "", n.bind(), nil, n.HostName(), cmd, env, []string{})
}

// initAuthToken initialises an admin auth token.
func (n *DANode) initAuthToken(ctx context.Context) error {
	// Command to generate admin token
	cmd := []string{"celestia", n.nodeType.String(), "auth", "admin"}

	// Run the command inside the container
	stdout, stderr, err := n.exec(ctx, n.logger, cmd, nil)
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
		n.logger.Debug("no node config found for node", zap.Int("index", n.Index), zap.String("type", n.nodeType.String()))
	}

	return nodeConfig
}

// getImage returns the Docker image for this node, preferring per-node config over the default image
func (n *DANode) getImage() DockerImage {
	if nodeConfig := n.getNodeConfig(); nodeConfig != nil && nodeConfig.Image != nil {
		return *nodeConfig.Image
	}
	return n.Image
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
