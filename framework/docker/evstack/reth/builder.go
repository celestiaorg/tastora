package reth

import (
    "context"
    "testing"

    "github.com/celestiaorg/tastora/framework/docker/container"
    dockerclient "github.com/moby/moby/client"
    "go.uber.org/zap"
    "go.uber.org/zap/zaptest"
)

// NodeBuilder constructs a single Reth Node and builds its Docker resources (volume, etc.).
type NodeBuilder struct {
	t                   *testing.T
	testName            string
	logger              *zap.Logger
	dockerClient        *dockerclient.Client
	networkID           string
	image               container.Image
	env                 []string
	additionalStartArgs []string
	bin                 string
	genesis             []byte
    jwtSecretHex        string
}

func NewNodeBuilder(t *testing.T) *NodeBuilder {
	return NewNodeBuilderWithTestName(t, t.Name())
}

func NewNodeBuilderWithTestName(t *testing.T, testName string) *NodeBuilder {
	t.Helper()
	return (&NodeBuilder{}).
		WithT(t).
		WithTestName(testName).
		WithLogger(zaptest.NewLogger(t)).
		WithImage(DefaultImage()).
		WithBin("ev-reth")
}

func (b *NodeBuilder) WithT(t *testing.T) *NodeBuilder {
	b.t = t
	return b
}
func (b *NodeBuilder) WithTestName(name string) *NodeBuilder {
	b.testName = name
	return b
}
func (b *NodeBuilder) WithLogger(l *zap.Logger) *NodeBuilder {
	b.logger = l
	return b
}
func (b *NodeBuilder) WithDockerClient(c *dockerclient.Client) *NodeBuilder {
	b.dockerClient = c
	return b
}
func (b *NodeBuilder) WithDockerNetworkID(id string) *NodeBuilder {
	b.networkID = id
	return b
}
func (b *NodeBuilder) WithImage(img container.Image) *NodeBuilder {
	b.image = img
	return b
}
func (b *NodeBuilder) WithEnv(env ...string) *NodeBuilder {
	b.env = env
	return b
}
func (b *NodeBuilder) WithAdditionalStartArgs(args ...string) *NodeBuilder {
	b.additionalStartArgs = args
	return b
}
func (b *NodeBuilder) WithBin(bin string) *NodeBuilder {
	b.bin = bin
	return b
}
func (b *NodeBuilder) WithGenesis(genesis []byte) *NodeBuilder {
	b.genesis = genesis
	return b
}

func (b *NodeBuilder) WithJWTSecretHex(secret string) *NodeBuilder {
    b.jwtSecretHex = secret
    return b
}

// Build constructs the Node and initializes its Docker volume but does not start the container.
func (b *NodeBuilder) Build(ctx context.Context) (*Node, error) {
    cfg := Config{
        Logger:              b.logger,
        DockerClient:        b.dockerClient,
        DockerNetworkID:     b.networkID,
        Image:               b.image,
        Bin:                 b.bin,
        Env:                 b.env,
        AdditionalStartArgs: b.additionalStartArgs,
        JWTSecretHex:        b.jwtSecretHex,
        GenesisFileBz:       b.genesis,
    }

    n, err := newNode(ctx, cfg, b.testName, 0)
	if err != nil {
		return nil, err
	}

	return n, nil
}
