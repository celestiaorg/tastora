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
		cfg:      *cfg.DANodeConfig,
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
	nodeType types.DANodeType
	cfg      DANodeConfig
	log      *zap.Logger
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

// Stop terminates the DANode by removing its associated container gracefully using the provided context.
func (n *DANode) Stop(ctx context.Context) error {
	return n.removeContainer(ctx)
}

// Start initializes and starts the DANode with the provided core IP and genesis hash in the given context.
// It returns an error if the node initialization or startup fails.
func (n *DANode) Start(ctx context.Context, opts ...types.DANodeStartOption) error {
	startOpts := types.DANodeStartOptions{}
	for _, fn := range opts {
		fn(&startOpts)
	}

	env := []string{
		fmt.Sprintf("P2P_NETWORK=%s", n.cfg.ChainID),
	}

	customEnvVar := fmt.Sprintf("CELESTIA_CUSTOM=%s", n.cfg.ChainID)
	if startOpts.GenesisBlockHash != "" {
		customEnvVar = fmt.Sprintf("%s:%s", customEnvVar, startOpts.GenesisBlockHash)
	}
	if startOpts.P2PAddress != "" {
		customEnvVar = fmt.Sprintf("%s:%s", customEnvVar, startOpts.P2PAddress)
	}
	env = append(env, customEnvVar)

	if err := n.initNode(ctx, env); err != nil {
		return fmt.Errorf("failed to initialize da node: %w", err)
	}

	if err := n.startNode(ctx, startOpts, env); err != nil {
		return fmt.Errorf("failed to start da node: %w", err)
	}

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

// modifyConfigToml disables RPC authentication so that the tests can use the endpoints without configuring auth.
func (n *DANode) modifyConfigToml(ctx context.Context) error {
	modifications := make(toml.Toml)
	rpc := make(toml.Toml)
	rpc["SkipAuth"] = true
	modifications["RPC"] = rpc
	return ModifyConfigFile(
		ctx,
		n.log,
		n.DockerClient,
		n.TestName,
		n.VolumeName,
		"config.toml",
		modifications,
	)
}

// startNode initializes and starts the DANode container and updates its configuration based on the provided options.
func (n *DANode) startNode(ctx context.Context, opts types.DANodeStartOptions, env []string) error {
	if err := n.createNodeContainer(ctx, opts, env); err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	if err := n.modifyConfigToml(ctx); err != nil {
		return fmt.Errorf("failed to disable RPC auth: %w", err)
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
func (n *DANode) initNode(ctx context.Context, env []string) error {
	// note: my_celes_key is the default key name for the bridge node.
	cmd := []string{"celestia", n.nodeType.String(), "init", "--p2p.network", n.cfg.ChainID, "--keyring.keyname", "my_celes_key", "--node.store", n.homeDir}
	_, _, err := n.exec(ctx, n.log, cmd, env)
	return err
}

// createNodeContainer creates and initializes a container for the DANode with specified context, options, and environment variables.
func (n *DANode) createNodeContainer(ctx context.Context, opts types.DANodeStartOptions, env []string) error {
	cmd := []string{"celestia", n.nodeType.String(), "start", "--p2p.network", n.cfg.ChainID, "--core.ip", opts.CoreIP, "--rpc.addr", "0.0.0.0", "--rpc.port", "26658", "--keyring.keyname", "my_celes_key", "--node.store", n.homeDir}
	usingPorts := nat.PortMap{}
	for k, v := range daNodePorts {
		usingPorts[k] = v
	}

	return n.containerLifecycle.CreateContainer(ctx, n.TestName, n.NetworkID, n.Image, usingPorts, "", n.bind(), nil, n.HostName(), cmd, env, []string{})
}
