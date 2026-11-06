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

// Install appends Dockerfile RUN steps that install opencode v2 via the
// official installer script (https://opencode.ai/v2/install). The installer
// is a native binary — no Node.js or npm required.
//
// The binary lands at ~/.opencode/bin/opencode (with an opencode2 shim).
// A profile.d script adds that directory to PATH for all login shells so
// the binary is available when the user SSHes into the container.
func (a *opencodeAgent) Install(b *docker.DockerfileBuilder) {
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates git && rm -rf /var/lib/apt/lists/*")
	// Run the v2 installer as the container user so the binary lands in that
	// user's home directory (~/.opencode/bin/), not root's.
	b.Run(fmt.Sprintf(
		`su - %[1]s -c 'curl -fsSL https://opencode.ai/v2/install | bash'`,
		b.Username(),
	))
	// Symlink the binaries into /usr/local/bin so they are on PATH for all
	// shell types (login, non-login, exec). The profile.d entry covers any
	// edge-cases where a login shell is preferred.
	b.Run(fmt.Sprintf(
		`ln -sf %[1]s/.opencode/bin/opencode /usr/local/bin/opencode && ln -sf %[1]s/.opencode/bin/opencode /usr/local/bin/opencode2`,
		b.HomeDir(),
	))
	b.Run(fmt.Sprintf(
		`printf '#!/bin/sh\nexport PATH="%s/.opencode/bin:$PATH"\n' > /etc/profile.d/opencode-path.sh && chmod +x /etc/profile.d/opencode-path.sh`,
		b.HomeDir(),
	))
	// Pre-create XDG directories that opencode v2 writes to on first run.
	// Without this, the Docker bind-mounts for the credential/config paths
	// cause intermediate directories to be owned by root, preventing writes.
	// ~/.local/state/opencode holds model.json and prompt-history.jsonl, which
	// are bind-mounted from the host so selections persist across sessions.
	b.Run(fmt.Sprintf(
		"mkdir -p %[1]s/.local/share/opencode %[1]s/.local/state/opencode %[1]s/.cache/opencode %[1]s/.config/opencode && chown -R %[2]s:%[2]s %[1]s/.local %[1]s/.cache %[1]s/.config",
		b.HomeDir(), b.Username(),
	))
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
func (a *opencodeAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string, username string) error {
	exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"su", "-", username, "-c", "opencode --version"})
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
// the primary credential store:
//   - ~/.config/opencode — user configuration (opencode.jsonc, agents, etc.)
//   - ~/.local/state/opencode — runtime state (model.json, prompt-history.jsonl)
//
// Mounting the state directory ensures that the selected model and prompt history
// are shared between the host and the container, so changes made inside the
// container persist on the host and vice versa.
func (a *opencodeAgent) AdditionalMounts(homeDir string) []docker.Mount {
	hostHome, _ := os.UserHomeDir()
	return []docker.Mount{
		{
			HostPath:      filepath.Join(hostHome, ".config", "opencode"),
			ContainerPath: filepath.Join(homeDir, ".config", "opencode"),
			ReadOnly:      false,
		},
		{
			HostPath:      filepath.Join(hostHome, ".local", "state", "opencode"),
			ContainerPath: filepath.Join(homeDir, ".local", "state", "opencode"),
			ReadOnly:      false,
		},
	}
}
