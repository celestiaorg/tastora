package reth

import (
    "context"
    "testing"

	"github.com/celestiaorg/tastora/framework/docker/container"
	dockerclient "github.com/moby/moby/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// ChainBuilder constructs a Reth Chain and its nodes
type ChainBuilder struct {
	t            *testing.T
	testName     string
	logger       *zap.Logger
	dockerClient *dockerclient.Client
	networkID    string
	image        container.Image
	env          []string
	addlArgs     []string
	bin          string
	genesis      []byte
	nodes        []NodeConfig
}

func NewChainBuilder(t *testing.T) *ChainBuilder {
    return NewChainBuilderWithTestName(t, t.Name())
}

func NewChainBuilderWithTestName(t *testing.T, testName string) *ChainBuilder {
	t.Helper()
	return (&ChainBuilder{}).
		WithT(t).
		WithTestName(testName).
		WithLogger(zaptest.NewLogger(t)).
		WithImage(DefaultImage()).
		WithBin("ev-reth")
}

func (b *ChainBuilder) WithT(t *testing.T) *ChainBuilder {
    b.t = t
    return b
}

func (b *ChainBuilder) WithTestName(name string) *ChainBuilder {
    b.testName = name
    return b
}

func (b *ChainBuilder) WithLogger(l *zap.Logger) *ChainBuilder {
    b.logger = l
    return b
}
func (b *ChainBuilder) WithDockerClient(c *dockerclient.Client) *ChainBuilder {
	b.dockerClient = c
	return b
}
func (b *ChainBuilder) WithDockerNetworkID(id string) *ChainBuilder {
    b.networkID = id
    return b
}

func (b *ChainBuilder) WithImage(img container.Image) *ChainBuilder {
    b.image = img
    return b
}

func (b *ChainBuilder) WithEnv(env ...string) *ChainBuilder {
    b.env = env
    return b
}
func (b *ChainBuilder) WithAdditionalStartArgs(args ...string) *ChainBuilder {
	b.addlArgs = args
	return b
}
func (b *ChainBuilder) WithBin(bin string) *ChainBuilder {
    b.bin = bin
    return b
}

func (b *ChainBuilder) WithGenesis(genesis []byte) *ChainBuilder {
    b.genesis = genesis
    return b
}

func (b *ChainBuilder) WithNode(cfg NodeConfig) *ChainBuilder {
    b.nodes = append(b.nodes, cfg)
    return b
}

func (b *ChainBuilder) WithNodes(cfgs ...NodeConfig) *ChainBuilder {
    b.nodes = cfgs
    return b
}

// Build constructs a Chain with nodes created and volumes initialized.
func (b *ChainBuilder) Build(ctx context.Context) (*Chain, error) {
    cfg := Config{
        Logger:          b.logger,
        DockerClient:    b.dockerClient,
        DockerNetworkID: b.networkID,
        Image:           b.image,
        Bin:             b.bin,
        Env:             b.env,
        AdditionalStartArgs: b.addlArgs,
        GenesisFileBz:       b.genesis,
    }

    chain := &Chain{
        cfg:       cfg,
        log:       b.logger,
        testName:  b.testName,
        nodes:     make(map[string]*Node),
        nextIndex: 0,
    }

    // Pre-create nodes and volumes (without starting)
    for i, nc := range b.nodes {
        n, err := newNode(ctx, cfg, b.testName, i, nc)
        if err != nil {
            return nil, err
        }
        chain.nodes[n.Name()] = n
        chain.nextIndex++
    }
    return chain, nil
}
