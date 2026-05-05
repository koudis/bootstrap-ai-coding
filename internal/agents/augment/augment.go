// Package augment implements the Augment Code AI coding agent module.
// It self-registers with the agent registry via init() and satisfies the
// agent.Agent interface. The core application has no direct dependency on
// this package — it is wired in exclusively via a blank import in main.go.
package augment

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

type augmentAgent struct{}

func init() {
	agent.Register(&augmentAgent{})
}

// ID returns the stable Agent_ID for the Augment Code agent.
// Satisfies: AC-1
func (a *augmentAgent) ID() string {
	return constants.AugmentCodeAgentName
}

// Install appends Dockerfile RUN steps that install Node.js 22 and the
// @augmentcode/auggie npm package globally.
// Satisfies: AC-2
func (a *augmentAgent) Install(b *docker.DockerfileBuilder) {
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates git && rm -rf /var/lib/apt/lists/*")
	if !b.IsNodeInstalled() {
		b.Run("curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && DEBIAN_FRONTEND=noninteractive apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
		b.MarkNodeInstalled()
	}
	b.Run("npm install -g --no-fund --no-audit @augmentcode/auggie")
}

// CredentialStorePath returns the default host-side credential directory for
// Augment Code authentication tokens.
// Satisfies: AC-3
func (a *augmentAgent) CredentialStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".augment")
}

// ContainerMountPath returns the path inside the container where the
// Credential_Store is bind-mounted.
// Satisfies: AC-3
func (a *augmentAgent) ContainerMountPath() string {
	return filepath.Join(constants.ContainerUserHome, ".augment")
}

// HasCredentials reports whether the credential store contains any non-empty
// files, indicating that the user has authenticated Augment Code.
// Returns (false, nil) when the directory is absent or empty — not an error.
// Satisfies: AC-4
func (a *augmentAgent) HasCredentials(storePath string) (bool, error) {
	entries, err := os.ReadDir(storePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking augment credentials: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return false, fmt.Errorf("checking augment credentials: %w", err)
		}
		if info.Size() > 0 {
			return true, nil
		}
	}
	return false, nil
}

// HealthCheck verifies that the auggie binary is present and executable inside
// the running container by executing `auggie --version`.
// Satisfies: AC-5
func (a *augmentAgent) HealthCheck(ctx context.Context, containerID string) error {
	exitCode, err := docker.ExecInContainer(ctx, containerID, []string{"auggie", "--version"})
	if err != nil {
		return fmt.Errorf("augment health check failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("augment health check failed: 'auggie --version' exited with code %d", exitCode)
	}
	return nil
}
