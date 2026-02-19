package spamoor

import (
    "context"

    "github.com/celestiaorg/tastora/framework/docker/container"
    "github.com/celestiaorg/tastora/framework/types"
    "go.uber.org/zap"
)

type Builder struct {
    testName      string
    dockerClient  types.TastoraDockerClient
    dockerNetwork string
    logger        *zap.Logger
    image         container.Image

    rpcHosts   []string
    privKey    string
    nameSuffix string
}

func NewNodeBuilder(testName string) *Builder {
    return &Builder{
        testName: testName,
        image:    container.NewImage("ethpandaops/spamoor", "latest", ""),
    }
}

func (b *Builder) WithDockerClient(c types.TastoraDockerClient) *Builder { b.dockerClient = c; return b }
func (b *Builder) WithDockerNetworkID(id string) *Builder { b.dockerNetwork = id; return b }
func (b *Builder) WithLogger(l *zap.Logger) *Builder      { b.logger = l; return b }
func (b *Builder) WithImage(img container.Image) *Builder { b.image = img; return b }
func (b *Builder) WithRPCHosts(hosts ...string) *Builder  { b.rpcHosts = hosts; return b }
func (b *Builder) WithPrivateKey(pk string) *Builder      { b.privKey = pk; return b }
func (b *Builder) WithNameSuffix(s string) *Builder       { b.nameSuffix = s; return b }

func (b *Builder) Build(ctx context.Context) (*Node, error) {
    cfg := Config{
        DockerClient:    b.dockerClient,
        DockerNetworkID: b.dockerNetwork,
        Logger:          b.logger,
        Image:           b.image,
        RPCHosts:        b.rpcHosts,
        PrivateKey:      b.privKey,
    }
    return newNode(ctx, cfg, b.testName, 0, b.nameSuffix)
}

