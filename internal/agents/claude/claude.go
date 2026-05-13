// Package claude implements the Claude Code AI coding agent module.
// It self-registers with the agent registry via init() and satisfies the
// agent.Agent interface. The core application has no direct dependency on
// this package — it is wired in exclusively via a blank import in main.go.
package claude

import (
	"context"
	"encoding/base64"
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
	return constants.ClaudeCodeAgentName
}

func (a *claudeAgent) Install(b *docker.DockerfileBuilder) {
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates git && rm -rf /var/lib/apt/lists/*")
	if !b.IsNodeInstalled() {
		b.Run("curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && DEBIAN_FRONTEND=noninteractive apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
		b.MarkNodeInstalled()
	}
	b.Run("npm install -g --no-fund --no-audit @anthropic-ai/claude-code")

	// Symlink ~/.claude.json into the credential mount directory so that a single
	// bind-mount on ~/.claude/ persists both OAuth tokens (.credentials.json) and
	// onboarding state (claude.json). Without this, Claude Code triggers the full
	// login/onboarding flow on every container start.
	b.Run(fmt.Sprintf(
		"ln -sf %s/claude.json %s/.claude.json",
		filepath.Join(b.HomeDir(), ".claude"),
		b.HomeDir(),
	))

	// Copy host user's Claude Code memory (CLAUDE.md) into the image so that
	// global instructions are available even before the bind-mount overlays.
	// The bind-mount at runtime will take precedence, but this ensures the
	// memory is baked into the image as a baseline.
	a.injectMemory(b)
}

// injectMemory copies the host user's ~/.claude/CLAUDE.md into the container
// image during build. Uses base64 encoding to safely embed file content in a
// RUN instruction (same pattern as gitconfig injection in the base builder).
func (a *claudeAgent) injectMemory(b *docker.DockerfileBuilder) {
	home, err := os.UserHomeDir()
	if err != nil {
		return // best-effort; skip if we can't determine home
	}

	claudeDir := filepath.Join(home, ".claude")
	memoryFile := filepath.Join(claudeDir, "CLAUDE.md")

	data, err := os.ReadFile(memoryFile)
	if err != nil {
		return // file doesn't exist or unreadable — skip silently
	}
	if len(data) == 0 {
		return
	}

	containerClaudeDir := filepath.Join(b.HomeDir(), ".claude")
	containerMemoryFile := filepath.Join(containerClaudeDir, "CLAUDE.md")

	encoded := base64.StdEncoding.EncodeToString(data)
	b.Run(fmt.Sprintf(
		"mkdir -p %s && echo %s | base64 -d > %s && chown -R %s:%s %s",
		containerClaudeDir,
		encoded,
		containerMemoryFile,
		b.Username(), b.Username(),
		containerClaudeDir,
	))
}

func (a *claudeAgent) CredentialStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func (a *claudeAgent) ContainerMountPath(homeDir string) string {
	return filepath.Join(homeDir, ".claude")
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

// PrepareCredentials copies ~/.claude.json into the credential store as
// claude.json (if it exists and the destination is absent or older).
// Inside the container a symlink at ~/.claude.json points to this file,
// so the bind-mount on ~/.claude/ covers both OAuth tokens and onboarding state.
func (a *claudeAgent) PrepareCredentials(storePath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil // best-effort; skip if we can't determine home
	}
	src := filepath.Join(home, ".claude.json")
	dst := filepath.Join(storePath, "claude.json")

	srcInfo, err := os.Stat(src)
	if err != nil {
		// Source doesn't exist — nothing to sync (first-time user).
		return nil
	}

	// Only copy if destination is missing or older than source.
	dstInfo, err := os.Stat(dst)
	if err == nil && !dstInfo.ModTime().Before(srcInfo.ModTime()) {
		return nil // destination is up-to-date
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", dst, err)
	}
	return nil
}

func (a *claudeAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string) error {
	exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"claude", "--version"})
	if err != nil {
		return fmt.Errorf("claude health check failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("claude health check failed: 'claude --version' exited with code %d", exitCode)
	}
	return nil
}

func (a *claudeAgent) SummaryInfo(ctx context.Context, c *docker.Client, containerID string) ([]agent.KeyValue, error) {
	return nil, nil
}
