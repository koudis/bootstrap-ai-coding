// Package kimi implements the Kimi Code agent module.
// It self-registers with the agent registry via init() and satisfies the
// agent.Agent interface. The core application has no direct dependency on
// this package — it is wired in exclusively via a blank import in main.go.
//
// Kimi Code is a native binary (no Node.js required). Authentication state
// persists through the bind-mounted ~/.kimi-code directory. Credential
// presence is detected by inspecting the credentials/ subdirectory rather
// than the top-level store directory. Kimi Code is opt-in only (not part of
// constants.DefaultAgents).
package kimi

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

type kimiAgent struct{}

func init() {
	agent.Register(&kimiAgent{})
}

// ID returns the stable Agent_ID for the Kimi Code agent.
// Satisfies: Requirement 1.1, Requirement 1.2
func (a *kimiAgent) ID() string {
	return constants.KimiCodeAgentName
}

// Install appends Dockerfile RUN steps that install the Kimi Code CLI.
//
// The official install.sh has a known bug (github.com/MoonshotAI/kimi-cli/issues/2272):
// it installs uv to ~/.local/bin but does not source its env file, so the
// immediately following "uv tool install kimi-cli" fails with "uv not found"
// in fresh container environments. We work around this by installing uv
// explicitly and then invoking it directly with an absolute path, all in
// one RUN layer as the Container_User.
//
// Satisfies: Requirement 2.1, Requirement 2.2, Requirement 2.4, Requirement 2.5,
//            Requirement 9.1, Requirement 9.2, Requirement 9.3, Requirement 9.5
func (a *kimiAgent) Install(b *docker.DockerfileBuilder) {
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates python3 && rm -rf /var/lib/apt/lists/*")

	// Pre-create ~/.local/bin and ~/.kimi-code as root then chown to the
	// Container_User so the subsequent user-level install can write there.
	b.Run(fmt.Sprintf(
		"mkdir -p %[1]s/.local/bin %[1]s/.kimi-code && chown -R %[2]s:%[2]s %[1]s/.local %[1]s/.kimi-code",
		b.HomeDir(), b.Username(),
	))

	// Add ~/.local/bin to PATH permanently so kimi is reachable in all exec
	// contexts (docker exec as root, su - user, SSH login shell).
	b.Env("PATH", fmt.Sprintf("$PATH:%s/.local/bin", b.HomeDir()))

	// Install uv then immediately use it (absolute path) to install kimi-cli.
	// Running as the Container_User means both uv and the kimi shim land in
	// the user's ~/.local/bin, which is now on the image PATH above.
	installCmd := fmt.Sprintf(
		"curl -fsSL https://astral.sh/uv/install.sh | env UV_UNMANAGED_INSTALL=%s/.local/bin sh && %s/.local/bin/uv tool install kimi-cli",
		b.HomeDir(), b.HomeDir(),
	)
	b.RunAsUser(installCmd)
}

// CredentialStorePath returns the default host-side credential/config directory
// for Kimi Code (~/.kimi-code).
// Satisfies: Requirement 3.1
func (a *kimiAgent) CredentialStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kimi-code")
}

// ContainerMountPath returns the path inside the container where ~/.kimi-code
// is bind-mounted. homeDir is the runtime-resolved Container_User_Home.
// Satisfies: Requirement 3.2
func (a *kimiAgent) ContainerMountPath(homeDir string) string {
	return filepath.Join(homeDir, ".kimi-code")
}

// HasCredentials reports whether the Kimi_Config_Store contains recognisable
// authentication state. Unlike other agents that check the top-level store
// directory, Kimi Code stores OAuth tokens under a credentials/ subdirectory,
// so only that subdir is inspected. Returns (false, nil) when the directory is
// absent or contains only empty files — not an error.
// Satisfies: Requirement 4.1, Requirement 4.2, Requirement 4.3, Requirement 4.4
func (a *kimiAgent) HasCredentials(storePath string) (bool, error) {
	credDir := filepath.Join(storePath, "credentials")
	entries, err := os.ReadDir(credDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking kimi-code credentials: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil && info.Size() > 0 {
			return true, nil
		}
	}
	return false, nil
}

// HealthCheck verifies that the kimi binary is present and executable inside
// the running container by executing `kimi --version` as the Container_User.
// Satisfies: Requirement 5.1, Requirement 5.3
func (a *kimiAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string, username string) error {
	exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"su", "-", username, "-c", "kimi --version"})
	if err != nil {
		return fmt.Errorf("kimi health check failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("kimi health check failed: 'kimi --version' exited with code %d", exitCode)
	}
	return nil
}

// SummaryInfo returns no additional session summary information for the Kimi Code agent.
// Satisfies: Requirement 8.1, Requirement 8.2
func (a *kimiAgent) SummaryInfo(ctx context.Context, c *docker.Client, containerID string) ([]agent.KeyValue, error) {
	return nil, nil
}
