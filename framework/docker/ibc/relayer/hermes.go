package relayer

import (
	"context"
	"fmt"

	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/docker/ibc"
	"github.com/celestiaorg/tastora/framework/types"
	dockerclient "github.com/moby/moby/client"
	"go.uber.org/zap"
)

const (
	hermesDefaultImage   = "ghcr.io/informalsystems/hermes"
	hermesDefaultVersion = "1.8.2"
	hermesDefaultUIDGID  = "1000:1000"
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
	
	// TODO: Generate Hermes configuration
	// TODO: Start Hermes Docker container as daemon
	h.started = true
	return nil
}

// Stop stops the Hermes relayer.
func (h *Hermes) Stop(ctx context.Context) error {
	if !h.started {
		return nil
	}
	
	// Stop the container if it's running
	if h.ContainerLifecycle != nil {
		if err := h.StopContainer(ctx); err != nil {
			return err
		}
	}
	
	h.started = false
	return nil
}

// CreateClients creates IBC clients on both chains.
func (h *Hermes) CreateClients(ctx context.Context, chainA, chainB types.Chain) error {
	// Use container.Job to execute hermes create client commands
	job := container.NewJob(h.Logger, h.DockerClient, h.NetworkID, h.TestName, hermesDefaultImage, hermesDefaultVersion)
	
	opts := container.Options{
		Binds: h.Bind(),
	}
	
	// TODO: Execute hermes create client command for chainA->chainB
	cmd := []string{"hermes", "create", "client", "--host-chain", chainA.GetChainID(), "--reference-chain", chainB.GetChainID()}
	result := job.Run(ctx, cmd, opts)
	if result.Err != nil {
		return result.Err
	}
	
	// TODO: Execute hermes create client command for chainB->chainA  
	cmd = []string{"hermes", "create", "client", "--host-chain", chainB.GetChainID(), "--reference-chain", chainA.GetChainID()}
	result = job.Run(ctx, cmd, opts)
	return result.Err
}

// CreateConnections creates IBC connections between the chains.
func (h *Hermes) CreateConnections(ctx context.Context, chainA, chainB types.Chain) error {
	// Use container.Job to execute hermes create connection commands
	job := container.NewJob(h.Logger, h.DockerClient, h.NetworkID, h.TestName, hermesDefaultImage, hermesDefaultVersion)
	
	opts := container.Options{
		Binds: h.Bind(),
	}
	
	// TODO: Execute hermes create connection command
	cmd := []string{"hermes", "create", "connection", "--a-chain", chainA.GetChainID(), "--b-chain", chainB.GetChainID()}
	result := job.Run(ctx, cmd, opts)
	return result.Err
}

// CreateChannel creates an IBC channel between the chains.
func (h *Hermes) CreateChannel(ctx context.Context, chainA, chainB types.Chain, opts ibc.CreateChannelOptions) error {
	// Use container.Job to execute hermes create channel commands
	job := container.NewJob(h.Logger, h.DockerClient, h.NetworkID, h.TestName, hermesDefaultImage, hermesDefaultVersion)
	
	runOpts := container.Options{
		Binds: h.Bind(),
	}
	
	// TODO: Execute hermes create channel command
	cmd := []string{
		"hermes", "create", "channel",
		"--order", string(opts.Order),
		"--a-chain", chainA.GetChainID(),
		"--a-port", opts.SourcePortName,
		"--b-chain", chainB.GetChainID(), 
		"--b-port", opts.DestPortName,
		"--channel-version", opts.Version,
	}
	result := job.Run(ctx, cmd, runOpts)
	return result.Err
}

// AddWallet adds a wallet for the specified chain ID to the relayer configuration.
func (h *Hermes) AddWallet(chainID string, wallet types.Wallet) error {
	h.wallets[chainID] = wallet
	// TODO: Add wallet to Hermes config
	return nil
}

// AddChain adds a chain to the relayer configuration.
func (h *Hermes) AddChain(chain types.Chain) error {
	h.chains[chain.GetChainID()] = chain
	// TODO: Add chain to Hermes config
	return nil
}