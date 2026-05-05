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

// CredentialPreparer is an optional interface that agents can implement to
// perform additional credential synchronisation before the container starts.
// For example, copying external state files into the credential store so they
// are available inside the bind-mounted directory.
type CredentialPreparer interface {
	PrepareCredentials(storePath string) error
}
