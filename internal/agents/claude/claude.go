// Package claude implements the Claude Code AI coding agent module.
// It self-registers with the agent registry via init() and satisfies the
// agent.Agent interface. The core application has no direct dependency on
// this package — it is wired in exclusively via a blank import in main.go.
package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

type claudeAgent struct{}

func init() {
	agent.Register(&claudeAgent{})
}

func (a *claudeAgent) ID() string {
	return constants.DefaultAgent
}

func (a *claudeAgent) Install(b *docker.DockerfileBuilder) {
	b.Run("apt-get update && apt-get install -y --no-install-recommends curl ca-certificates git && rm -rf /var/lib/apt/lists/*")
	b.Run("curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - && apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
	b.Run("npm install -g @anthropic-ai/claude-code")
}

func (a *claudeAgent) CredentialStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func (a *claudeAgent) ContainerMountPath() string {
	return filepath.Join(constants.ContainerUserHome, ".claude")
}

func (a *claudeAgent) HasCredentials(storePath string) (bool, error) {
	credFile := filepath.Join(storePath, ".credentials.json")
	_, err := os.Stat(credFile)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking claude credentials: %w", err)
	}
	return true, nil
}

func (a *claudeAgent) HealthCheck(ctx context.Context, containerID string) error {
	exitCode, err := docker.ExecInContainer(ctx, containerID, []string{"claude", "--version"})
	if err != nil {
		return fmt.Errorf("claude health check failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("claude health check failed: 'claude --version' exited with code %d", exitCode)
	}
	return nil
}
