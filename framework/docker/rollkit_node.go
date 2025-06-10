package docker

import (
	"context"
	"fmt"
	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
	"path"
	"sync"
)

var _ types.RollkitNode = &RollkitNode{}

type RollkitNode struct {
	*node
	cfg Config
	log *zap.Logger
	mu  sync.Mutex
}

func NewRollkitNode(log *zap.Logger, cfg Config, testName string, image DockerImage, index int) *RollkitNode {
	rn := &RollkitNode{
		log: log.With(
			zap.Int("i", index),
		),
		cfg:  cfg,
		node: newNode(cfg.DockerNetworkID, cfg.DockerClient, testName, image, path.Join("/var", "rollkit"), index, "rollkit"),
	}

	rn.containerLifecycle = NewContainerLifecycle(log, cfg.DockerClient, rn.Name())
	return rn
}

// Name of the test node container.
func (rn *RollkitNode) Name() string {
	return fmt.Sprintf("%s-rollkit-%d-%s", rn.cfg.RollkitChainConfig.ChainID, rn.Index, SanitizeContainerName(rn.TestName))
}

func (rn *RollkitNode) logger() *zap.Logger {
	return rn.cfg.Logger.With(
		zap.String("chain_id", rn.cfg.ChainConfig.ChainID),
		zap.String("test", rn.TestName),
	)
}

func (rn *RollkitNode) initFullNodeFiles(ctx context.Context) error {
	if err := rn.initHomeFolder(ctx); err != nil {
		return err
	}
	return nil
	//return rn.setTestConfig(ctx)
}

// initHomeFolder initializes a home folder for the given node.
func (rn *RollkitNode) initHomeFolder(ctx context.Context) error {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	_, _, err := tn.execBin(ctx, tn.getInitCommand()...)
	return err
}

func (rn *RollkitNode) execBin(ctx context.Context, command ...string) ([]byte, []byte, error) {
	cmd := []string{rn.cfg.RollkitChainConfig.Bin, "--home", rn.homeDir}
	return rn.exec(ctx, rn.logger(), append(cmd, command...), rn.cfg.RollkitChainConfig.Env)
}

// isAggregator returns true if the RollkitNode is the aggregator
func (rn *RollkitNode) isAggregator() bool {
	return rn.Index == 0
}

// Init initializes the RollkitNode
func (rn *RollkitNode) Init(ctx context.Context, initArguments ...string) error {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	cmd := []string{rn.cfg.RollkitChainConfig.Bin, "--home", rn.homeDir, "--chain_id", rn.cfg.RollkitChainConfig.ChainID, "init"}
	if rn.isAggregator() {
		cmd = append(cmd, "--rollkit.node.aggregator", "--rollkit.node.passphrase="+rn.cfg.RollkitChainConfig.AggregatorPassphrase)
	}

	_, _, err := rn.exec(ctx, rn.logger(), append(cmd, initArguments...), rn.cfg.RollkitChainConfig.Env)
	return err
}

// Start starts an individual rollkit node.
func (rn *RollkitNode) Start(ctx context.Context, startArguments ...string) error {

	// create the container

	// start the container

	return nil
}
