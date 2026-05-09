// Package buildresources implements a pseudo-agent module that installs
// common build toolchains and language runtimes into the container.
// It self-registers with the agent registry via init() and satisfies the
// agent.Agent interface. The core application has no direct dependency on
// this package — it is wired in exclusively via a blank import in main.go.
package buildresources

import (
	"context"
	"fmt"
	"strings"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// aptPackages lists all system packages installed by this agent, grouped by
// category for readability and easy modification.
var aptPackages = []string{
	// Python
	"python3", "python3-pip", "python3-venv", "python3-dev", "python3-pytest",
	"python3-setuptools", "python3-wheel",
	// C/C++ build toolchain
	"build-essential", "cmake", "pkg-config",
	// Java
	"default-jdk",
	// Common build dependencies
	"libssl-dev", "libffi-dev",
	// Search and text processing
	"ripgrep", "fd-find", "jq",
	// Version control extras
	"git-lfs",
	// Terminal and shell utilities
	"tmux", "less", "file", "shellcheck",
	// Database
	"sqlite3",
	// Archive handling
	"zip",
	// General utilities
	"curl", "ca-certificates", "unzip", "wget", "neovim",
}

// goVersion is the Go release installed via the official tarball.
const goVersion = "1.24.2"

type buildResourcesAgent struct{}

func init() {
	agent.Register(&buildResourcesAgent{})
}

// ID returns the stable Agent_ID for the Build Resources agent.
// Satisfies: BR-1
func (a *buildResourcesAgent) ID() string {
	return constants.BuildResourcesAgentName
}

// Install appends Dockerfile RUN steps that install Python 3, uv, CMake,
// build-essential, OpenJDK, Go, and common build dependencies.
// Satisfies: BR-2
func (a *buildResourcesAgent) Install(b *docker.DockerfileBuilder) {
	// System packages (as root)
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends " +
		strings.Join(aptPackages, " ") +
		" && rm -rf /var/lib/apt/lists/*")

	// Go — official tarball to /usr/local/go (architecture-aware)
	b.Run(fmt.Sprintf("curl -fsSL https://go.dev/dl/go%s.linux-$(dpkg --print-architecture).tar.gz | tar -C /usr/local -xz", goVersion))
	b.Run("echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/golang.sh && chmod +x /etc/profile.d/golang.sh")

	// Python uv — installed system-wide to /usr/local/bin via official installer
	// Using UV_INSTALL_DIR to place the binary where all users can access it,
	// avoiding user-local PATH issues with docker exec (which runs as root).
	b.Run("curl -LsSf https://astral.sh/uv/install.sh | UV_INSTALL_DIR=/usr/local/bin sh")
}

// CredentialStorePath returns empty — no credentials to persist.
// Satisfies: BR-3
func (a *buildResourcesAgent) CredentialStorePath() string {
	return ""
}

// ContainerMountPath returns empty — no bind-mount needed.
// Satisfies: BR-3
func (a *buildResourcesAgent) ContainerMountPath(homeDir string) string {
	return ""
}

// HasCredentials always returns true — nothing to check.
// Satisfies: BR-3
func (a *buildResourcesAgent) HasCredentials(storePath string) (bool, error) {
	return true, nil
}

// HealthCheck verifies all build tools are installed and executable.
// Satisfies: BR-4
func (a *buildResourcesAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string) error {
	checks := []struct {
		cmd  []string
		name string
	}{
		{[]string{"python3", "--version"}, "python3"},
		{[]string{"uv", "--version"}, "uv"},
		{[]string{"cmake", "--version"}, "cmake"},
		{[]string{"javac", "-version"}, "javac"},
		{[]string{"bash", "-lc", "go version"}, "go"},
		{[]string{"rg", "--version"}, "ripgrep"},
		{[]string{"fdfind", "--version"}, "fd-find"},
		{[]string{"jq", "--version"}, "jq"},
		{[]string{"git-lfs", "--version"}, "git-lfs"},
		{[]string{"tmux", "-V"}, "tmux"},
	}
	for _, chk := range checks {
		exitCode, err := docker.ExecInContainer(ctx, c, containerID, chk.cmd)
		if err != nil {
			return fmt.Errorf("build-resources health check failed (%s): %w", chk.name, err)
		}
		if exitCode != 0 {
			return fmt.Errorf("build-resources health check failed: '%s' exited with code %d", chk.name, exitCode)
		}
	}
	return nil
}
