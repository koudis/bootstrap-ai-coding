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

// AdditionalMounts returns the read-only bind-mount for ~/.claude.json.
// If the file does not exist on the host, the mount is omitted.
func (a *claudeAgent) AdditionalMounts(homeDir string) []docker.Mount {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	src := filepath.Join(home, ".claude.json")
	if _, err := os.Stat(src); err != nil {
		return nil // file absent or unreadable — skip gracefully
	}
	return []docker.Mount{
		{
			HostPath:      src,
			ContainerPath: filepath.Join(homeDir, ".claude.json"),
			ReadOnly:      true,
		},
	}
}

func (a *claudeAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string, username string) error {
	exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"su", "-", username, "-c", "claude --version"})
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
