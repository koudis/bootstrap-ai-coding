package agent

import "github.com/koudis/bootstrap-ai-coding/internal/docker"

// AdditionalMounter is an optional interface that agents can implement to
// declare additional bind-mounts beyond the single primary credential store.
// The core calls this after processing CredentialStorePath/ContainerMountPath
// and appends the returned mounts to the container spec.
type AdditionalMounter interface {
	AdditionalMounts(homeDir string) []docker.Mount
}
