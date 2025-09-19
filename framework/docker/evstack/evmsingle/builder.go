package evmsingle

import (
    "context"
    "testing"

    "github.com/celestiaorg/tastora/framework/docker/container"
    dockerclient "github.com/moby/moby/client"
    "go.uber.org/zap"
    "go.uber.org/zap/zaptest"
)

// AppBuilder constructs a set of ev-node-evm-single nodes
type AppBuilder struct {
    t            *testing.T
    testName     string
    logger       *zap.Logger
    dockerClient *dockerclient.Client
    networkID    string
    image        container.Image
    env          []string
    addlArgs     []string
    nodes        []NodeConfig
}

func NewBuilder(t *testing.T) *AppBuilder {
    return NewBuilderWithTestName(t, t.Name())
}

func NewBuilderWithTestName(t *testing.T, testName string) *AppBuilder {
    t.Helper()
    return (&AppBuilder{}).
        WithT(t).
        WithTestName(testName).
        WithLogger(zaptest.NewLogger(t)).
        WithImage(DefaultImage())
}

func (b *AppBuilder) WithT(t *testing.T) *AppBuilder {
    b.t = t
    return b
}

func (b *AppBuilder) WithTestName(name string) *AppBuilder {
    b.testName = name
    return b
}

func (b *AppBuilder) WithLogger(l *zap.Logger) *AppBuilder {
    b.logger = l
    return b
}

func (b *AppBuilder) WithDockerClient(c *dockerclient.Client) *AppBuilder {
    b.dockerClient = c
    return b
}

func (b *AppBuilder) WithDockerNetworkID(id string) *AppBuilder {
    b.networkID = id
    return b
}

func (b *AppBuilder) WithImage(img container.Image) *AppBuilder {
    b.image = img
    return b
}

func (b *AppBuilder) WithEnv(env ...string) *AppBuilder {
    b.env = env
    return b
}

func (b *AppBuilder) WithAdditionalStartArgs(args ...string) *AppBuilder {
    b.addlArgs = args
    return b
}

func (b *AppBuilder) WithNode(cfg NodeConfig) *AppBuilder {
    b.nodes = append(b.nodes, cfg)
    return b
}

func (b *AppBuilder) WithNodes(cfgs ...NodeConfig) *AppBuilder {
    b.nodes = cfgs
    return b
}

// Build constructs an App with nodes created and volumes initialized (not started)
func (b *AppBuilder) Build(ctx context.Context) (*App, error) {
    cfg := Config{
        Logger:             b.logger,
        DockerClient:       b.dockerClient,
        DockerNetworkID:    b.networkID,
        Image:              b.image,
        Bin:                "evm-single",
        Env:                b.env,
        AdditionalStartArgs: b.addlArgs,
    }

    app := &App{
        cfg:       cfg,
        log:       b.logger,
        testName:  b.testName,
        nodes:     make(map[string]*Node),
        nextIndex: 0,
    }

    for i, nc := range b.nodes {
        n, err := newNode(ctx, cfg, b.testName, i, nc)
        if err != nil {
            return nil, err
        }
        app.nodes[n.Name()] = n
        app.nextIndex++
    }
    return app, nil
}

