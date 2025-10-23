package docker

import (
	"github.com/moby/moby/client"
)

// LabeledClient wraps a Docker client with an associated cleanup label.
// the cleanup label is used to tag all resources (containers, volumes, networks)
// created by this client, enabling cleanup to find and remove exactly the resources
// associated with a specific test run.
type LabeledClient struct {
	*client.Client
	cleanupLabel string
}

// NewLabeledClient creates a new LabeledClient with the given Docker client and cleanup label.
func NewLabeledClient(c *client.Client, cleanupLabel string) *LabeledClient {
	return &LabeledClient{
		Client:       c,
		cleanupLabel: cleanupLabel,
	}
}

// CleanupLabel returns the cleanup label associated with this client.
func (l *LabeledClient) CleanupLabel() string {
	return l.cleanupLabel
}
