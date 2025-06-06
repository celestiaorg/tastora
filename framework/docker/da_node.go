package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/docker/consts"
	"github.com/celestiaorg/tastora/framework/testutil/toml"
	"github.com/celestiaorg/tastora/framework/types"
	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/docker/go-connections/nat"
	"go.uber.org/zap"
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

	image := cfg.DataAvailabilityNetworkConfig.Image

	bn := &DANode{
		nodeType: nodeType,
		log: cfg.Logger.With(
			zap.String("node_type", nodeType.String()),
		),
		node: newNode(cfg.DockerNetworkID, cfg.DockerClient, testName, image, "/home/celestia", idx, nodeType.String()),
	}

	bn.containerLifecycle = NewContainerLifecycle(cfg.Logger, cfg.DockerClient, bn.Name())

	v, err := cfg.DockerClient.VolumeCreate(ctx, volumetypes.CreateOptions{
		Labels: map[string]string{
			consts.CleanupLabel:   testName,
			consts.NodeOwnerLabel: bn.Name(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating volume for chain node: %w", err)
	}
	bn.VolumeName = v.Name

	if err := SetVolumeOwner(ctx, VolumeOwnerOptions{
		Log:        bn.log,
		Client:     cfg.DockerClient,
		VolumeName: v.Name,
		ImageRef:   image.Ref(),
		TestName:   testName,
		UidGid:     image.UIDGID,
	}); err != nil {
		return nil, fmt.Errorf("set volume owner: %w", err)
	}

	return bn, nil
}

// DANode is a docker implementation of a celestia bridge node.
type DANode struct {
	*node
	mu             sync.Mutex
	hasBeenStarted bool
	nodeType       types.DANodeType
	log            *zap.Logger
	// ports that are resolvable from the test runners themselves.
	hostRPCPort string
	hostP2PPort string
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

	if err := n.startNode(ctx, startOpts.StartArguments, startOpts.ConfigModifications, env); err != nil {
		return fmt.Errorf("failed to start da node: %w", err)
	}

	n.hasBeenStarted = true
	return nil
}

// Name of the test node container.
func (n *node) Name() string {
	return fmt.Sprintf("%s-%d-%s", n.GetType(), n.Index, SanitizeContainerName(n.TestName))
}

// HostName of the test node container.
func (n *node) HostName() string {
	return CondenseHostName(n.Name())
}

// ModifyConfigFiles modifies the specified config files with the provided TOML modifications.
func (n *DANode) ModifyConfigFiles(ctx context.Context, configModifications map[string]toml.Toml) error {
	for filePath, modifications := range configModifications {
		if err := ModifyConfigFile(
			ctx,
			n.log,
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
	// note: my_celes_key is the default key name for the da node.
	cmd := []string{"celestia", n.nodeType.String(), "init", "--p2p.network", chainID, "--keyring.keyname", "my_celes_key", "--node.store", n.homeDir}
	_, _, err := n.exec(ctx, n.log, cmd, env)
	return err
}

// createNodeContainer creates and initializes a container for the DANode with specified context, options, and environment variables.
func (n *DANode) createNodeContainer(ctx context.Context, additionalStartArgs []string, env []string) error {
	cmd := []string{"celestia", n.nodeType.String(), "start"}
	cmd = append(cmd, additionalStartArgs...)
	usingPorts := nat.PortMap{}
	for k, v := range daNodePorts {
		usingPorts[k] = v
	}
	return n.containerLifecycle.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, usingPorts, "", n.bind(), nil, n.HostName(), cmd, env, []string{})
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
