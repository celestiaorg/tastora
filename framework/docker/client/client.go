package client

import (
	"github.com/celestiaorg/tastora/framework/types"
	"github.com/moby/moby/client"
)

// Client wraps a Docker client with an associated cleanup label.
// the cleanup label is used to tag all resources (containers, volumes, networks)
// created by this client, enabling cleanup to find and remove exactly the resources
// associated with a specific test run.
//
// Client implements types.TastoraDockerClient.
type Client struct {
	*client.Client
	cleanupLabel string
}

var _ types.TastoraDockerClient = (*Client)(nil)

// NewClient creates a new Client with the given Docker client and cleanup label.
func NewClient(c *client.Client, cleanupLabel string) *Client {
	return &Client{
		Client:       c,
		cleanupLabel: cleanupLabel,
	}
}

// CleanupLabel returns the cleanup label associated with this client.
func (c *Client) CleanupLabel() string {
	return c.cleanupLabel
}
