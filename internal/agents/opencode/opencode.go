// Package opencode implements the OpenCode AI coding agent module.
// It self-registers with the agent registry via init() and satisfies both
// the agent.Agent and agent.AdditionalMounter interfaces. The core application
// has no direct dependency on this package — it is wired in exclusively via a
// blank import in main.go.
package opencode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

type opencodeAgent struct{}

func init() {
	agent.Register(&opencodeAgent{})
}

// ID returns the stable Agent_ID for the OpenCode agent.
func (a *opencodeAgent) ID() string {
	return constants.OpenCodeAgentName
}

// Install appends Dockerfile RUN steps that install Node.js 22 (deduplicated)
// and the opencode-ai npm package globally.
func (a *opencodeAgent) Install(b *docker.DockerfileBuilder) {
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates git && rm -rf /var/lib/apt/lists/*")
	if !b.IsNodeInstalled() {
		b.Run("curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && DEBIAN_FRONTEND=noninteractive apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
		b.MarkNodeInstalled()
	}
	b.Run("npm install -g --no-fund --no-audit opencode-ai")
}

// CredentialStorePath returns the default host-side credential directory for
// OpenCode authentication tokens (~/.local/share/opencode).
func (a *opencodeAgent) CredentialStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode")
}

// ContainerMountPath returns the path inside the container where the
// primary credential store is bind-mounted.
func (a *opencodeAgent) ContainerMountPath(homeDir string) string {
	return filepath.Join(homeDir, ".local", "share", "opencode")
}

// HasCredentials reports whether the credential store contains the auth.json
// file with size > 0, indicating that the user has authenticated OpenCode.
// Returns (false, nil) when the directory or auth.json is absent or zero-length.
func (a *opencodeAgent) HasCredentials(storePath string) (bool, error) {
	info, err := os.Stat(filepath.Join(storePath, "auth.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking opencode credentials: %w", err)
	}
	if info.Size() == 0 {
		return false, nil
	}
	return true, nil
}

// HealthCheck verifies that the opencode binary is present and executable inside
// the running container by executing `opencode --version`.
func (a *opencodeAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string) error {
	exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"opencode", "--version"})
	if err != nil {
		return fmt.Errorf("opencode health check failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("opencode health check failed: 'opencode --version' exited with code %d", exitCode)
	}
	return nil
}

// SummaryInfo returns no additional session summary information for the
// OpenCode agent.
func (a *opencodeAgent) SummaryInfo(ctx context.Context, c *docker.Client, containerID string) ([]agent.KeyValue, error) {
	return nil, nil
}

// AdditionalMounts returns the extra bind-mounts required by OpenCode beyond
// the primary credential store. OpenCode needs its config directory
// (~/.config/opencode) mounted into the container.
func (a *opencodeAgent) AdditionalMounts(homeDir string) []docker.Mount {
	hostHome, _ := os.UserHomeDir()
	return []docker.Mount{
		{
			HostPath:      filepath.Join(hostHome, ".config", "opencode"),
			ContainerPath: filepath.Join(homeDir, ".config", "opencode"),
			ReadOnly:      false,
		},
	}
}
