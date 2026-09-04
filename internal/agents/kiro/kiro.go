// Package kiro implements the Kiro CLI agent module.
// It self-registers with the agent registry via init() and satisfies the
// agent.Agent interface. The core application has no direct dependency on
// this package — it is wired in exclusively via a blank import in main.go.
//
// The module serves two goals that share one image: it pre-installs the
// `kiro-cli` binary so it can be run over SSH, and it ensures the Node.js
// runtime dependency is present so the Kiro IDE can connect to the container
// as a remote workspace via the SSH server the core already provides.
// Authentication state persists entirely through the bind-mounted ~/.kiro
// directory — there is no dependency on any host environment-variable
// passthrough. Kiro is opt-in only (not part of constants.DefaultAgents).
package kiro

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

type kiroAgent struct{}

func init() {
	agent.Register(&kiroAgent{})
}

// ID returns the stable Agent_ID for the Kiro agent.
// Satisfies: KIRO-1
func (a *kiroAgent) ID() string {
	return constants.KiroAgentName
}

// Install appends Dockerfile RUN steps that install Node.js 22 (deduplicated
// via the shared Is/MarkNodeInstalled pattern) and the Kiro CLI via the
// official installer. The installer places `kiro-cli` under a user-local
// directory that is not on the default PATH when `docker exec` runs as root,
// so the binary is symlinked into /usr/local/bin — on every user's default
// PATH and every exec context.
// Satisfies: KIRO-2, KIRO-4.4
func (a *kiroAgent) Install(b *docker.DockerfileBuilder) {
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates git unzip && rm -rf /var/lib/apt/lists/*")
	if !b.IsNodeInstalled() {
		b.Run("curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && DEBIAN_FRONTEND=noninteractive apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
		b.MarkNodeInstalled()
	}

	// Install the Kiro CLI non-interactively, then symlink the resulting binary
	// into /usr/local/bin so both `docker exec` (running as root, HOME=/root)
	// and the Container_User's login shell can locate `kiro-cli` on PATH. The
	// `find` locates the installed binary regardless of the exact directory the
	// installer chooses.
	b.Run("curl -fsSL https://cli.kiro.dev/install | bash")
	b.Run("ln -sf \"$(find / -name kiro-cli -type f 2>/dev/null | head -n1)\" /usr/local/bin/kiro-cli")

	// Pre-create the credential/config directory so the runtime bind-mount does
	// not cause intermediate parents to be created as root, which would block
	// writes by the non-root Container_User (same pattern as the OpenCode module).
	b.Run(fmt.Sprintf(
		"mkdir -p %[1]s/.kiro && chown -R %[2]s:%[2]s %[1]s/.kiro",
		b.HomeDir(), b.Username(),
	))
}

// CredentialStorePath returns the default host-side credential/config directory
// for Kiro (~/.kiro).
// Satisfies: KIRO-5
func (a *kiroAgent) CredentialStorePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kiro")
}

// ContainerMountPath returns the path inside the container where ~/.kiro is
// bind-mounted. homeDir is the runtime-resolved Container_User_Home (Req 22).
// Satisfies: KIRO-5
func (a *kiroAgent) ContainerMountPath(homeDir string) string {
	return filepath.Join(homeDir, ".kiro")
}

// HasCredentials reports whether the Kiro_Config_Store contains recognisable
// authentication state. Any non-empty regular file under ~/.kiro is treated as
// evidence that the user has established auth/config on the Host — Kiro does
// not document a single stable token filename, so this mirrors the Augment
// module's conservative "non-empty directory" heuristic. Returns (false, nil)
// when the directory is absent or empty — not an error.
// Satisfies: KIRO-6
func (a *kiroAgent) HasCredentials(storePath string) (bool, error) {
	entries, err := os.ReadDir(storePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking kiro credentials: %w", err)
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

// HealthCheck verifies that kiro-cli is present and executable inside the
// running container by executing `kiro-cli --version` as the Container_User.
// Satisfies: KIRO-7
func (a *kiroAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string, username string) error {
	exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"su", "-", username, "-c", "kiro-cli --version"})
	if err != nil {
		return fmt.Errorf("kiro health check failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("kiro health check failed: 'kiro-cli --version' exited with code %d", exitCode)
	}
	return nil
}

// SummaryInfo returns no additional session summary information for the Kiro agent.
func (a *kiroAgent) SummaryInfo(ctx context.Context, c *docker.Client, containerID string) ([]agent.KeyValue, error) {
	return nil, nil
}
