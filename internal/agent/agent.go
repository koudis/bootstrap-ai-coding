// Package agent defines the Agent interface and the global agent registry.
// Agent modules implement this interface and self-register via init().
package agent

import (
	"context"

	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// Agent is the contract every AI coding agent module must satisfy.
type Agent interface {
	ID() string
	Install(b *docker.DockerfileBuilder)
	CredentialStorePath() string
	ContainerMountPath() string
	HasCredentials(storePath string) (bool, error)
	HealthCheck(ctx context.Context, containerID string) error
}
