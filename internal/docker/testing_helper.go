package docker

import dockerclient "github.com/docker/docker/client"

// NewClientForTest creates a Client wrapping the given Docker SDK client.
// Intended for use in test code across packages that need to inject a fake
// Docker client without connecting to a real daemon.
func NewClientForTest(inner *dockerclient.Client) *Client {
	return &Client{inner: inner}
}
