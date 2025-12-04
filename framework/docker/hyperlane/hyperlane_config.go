package hyperlane

import (
	hyputil "github.com/bcp-innovations/hyperlane-cosmos/util"
	"github.com/celestiaorg/tastora/framework/docker/container"
	"github.com/celestiaorg/tastora/framework/types"
	"go.uber.org/zap"
)

// Config holds configuration for the Hyperlane deployment coordinator
type Config struct {
	Logger          *zap.Logger
	DockerClient    types.TastoraDockerClient
	DockerNetworkID string

	// HyperlaneImage for hyperlane CLI (contains hyperlane binary)
	HyperlaneImage container.Image
}

// DefaultDeployerImage returns the default hyperlane CLI image
// TODO: replace this with an image that just has the hyperlane cli, not using the hyperlane-init image.
func DefaultDeployerImage() container.Image {
	return container.Image{Repository: "ghcr.io/celestiaorg/hyperlane-init", Version: "latest", UIDGID: "1000:1000"}
}

// CosmosConfig contains the IDs of all deployed cosmos-native hyperlane components
type CosmosConfig struct {
	IsmID     hyputil.HexAddress `json:"ism_id"`
	HooksID   hyputil.HexAddress `json:"hooks_id"`
	MailboxID hyputil.HexAddress `json:"mailbox_id"`
	TokenID   hyputil.HexAddress `json:"token_id"`
}
