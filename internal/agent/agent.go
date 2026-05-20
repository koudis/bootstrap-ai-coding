// Package agent defines the Agent interface and the global agent registry.
// Agent modules implement this interface and self-register via init().
package agent

import (
	"context"

	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// KeyValue represents a single labelled line in the session summary.
// Agents return slices of these from SummaryInfo().
type KeyValue struct {
	Key   string
	Value string
}

// Agent is the contract every AI coding agent module must satisfy.
type Agent interface {
	ID() string
	Install(b *docker.DockerfileBuilder)
	CredentialStorePath() string
	ContainerMountPath(homeDir string) string
	HasCredentials(storePath string) (bool, error)
	HealthCheck(ctx context.Context, c *docker.Client, containerID string) error
	SummaryInfo(ctx context.Context, c *docker.Client, containerID string) ([]KeyValue, error)
}

