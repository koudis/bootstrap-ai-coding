package docker

import dockerclient "github.com/docker/docker/client"

// NewClientForTest creates a Client wrapping the given Docker SDK client.
// This is exported only for testing (via the _test.go convention) so that
// external test packages can inject a fake Docker client.
func NewClientForTest(inner *dockerclient.Client) *Client {
	return &Client{inner: inner}
}
