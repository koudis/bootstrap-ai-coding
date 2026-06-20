// Package codex implements the OpenAI Codex CLI agent module.
// It self-registers with the agent registry via init() and satisfies the
// agent.Agent interface. The core application has no direct dependency on
// this package — it is wired in exclusively via a blank import in main.go.
package codex

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

type codexAgent struct{}

func init() {
	agent.Register(&codexAgent{})
}

// ID returns the stable Agent_ID for the Codex agent.
// Satisfies: CX-1
func (a *codexAgent) ID() string {
	return constants.CodexAgentName
}

// Install appends Dockerfile RUN steps that install Node.js 22 and the
// @openai/codex npm package globally.
// Satisfies: CX-2
func (a *codexAgent) Install(b *docker.DockerfileBuilder) {
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates git && rm -rf /var/lib/apt/lists/*")
	if !b.IsNodeInstalled() {
		b.Run("curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && DEBIAN_FRONTEND=noninteractive apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
		b.MarkNodeInstalled()
	}
	b.Run("npm install -g --no-fund --no-audit @openai/codex")
}

// CredentialStorePath returns the default host-side credential directory for
// Codex authentication tokens.
// Satisfies: CX-3
func (a *codexAgent) CredentialStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}

// ContainerMountPath returns the path inside the container where the
// Credential_Store is bind-mounted.
// Satisfies: CX-3
func (a *codexAgent) ContainerMountPath(homeDir string) string {
	return filepath.Join(homeDir, ".codex")
}

// HasCredentials reports whether the credential store contains the auth.json
// file, indicating that the user has authenticated Codex.
// Returns (false, nil) when the directory or auth.json is absent — not an error.
// Satisfies: CX-4
func (a *codexAgent) HasCredentials(storePath string) (bool, error) {
	_, err := os.Stat(filepath.Join(storePath, "auth.json"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("checking codex credentials: %w", err)
}

// HealthCheck verifies that the codex binary is present and executable inside
// the running container by executing `codex --version`.
// Satisfies: CX-5
func (a *codexAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string) error {
	exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"codex", "--version"})
	if err != nil {
		return fmt.Errorf("codex health check failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("codex health check failed: 'codex --version' exited with code %d", exitCode)
	}
	return nil
}

// SummaryInfo returns no additional session summary information for the
// Codex agent.
// Satisfies: Req 9 (Session Summary)
func (a *codexAgent) SummaryInfo(ctx context.Context, c *docker.Client, containerID string) ([]agent.KeyValue, error) {
	return nil, nil
}
