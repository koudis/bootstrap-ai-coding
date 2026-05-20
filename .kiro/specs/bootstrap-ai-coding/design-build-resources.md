# Build Resources Agent Module Design

This document describes the Build Resources pseudo-agent module, which installs common build toolchains and language runtimes into the container.

> **Related documents:**
> - [design.md](design.md) — Overview and document index
> - [design-architecture.md](design-architecture.md) — High-level architecture, package layout, sequence diagrams
> - [design-components.md](design-components.md) — Core component designs
> - [design-docker.md](design-docker.md) — Two-layer Docker image architecture
> - [design-data-models.md](design-data-models.md) — Data models, error handling, test infrastructure
> - [design-agents.md](design-agents.md) — Agent modules: contract, implementations
> - [design-properties.md](design-properties.md) — Correctness properties and testing strategy

---

## Overview

Build Resources is a pseudo-agent that installs common build toolchains and language runtimes into the container. It does not provide an AI coding tool — it exists purely to ensure the development environment is ready for compilation and packaging out of the box. It follows the standard agent module pattern for architectural simplicity.

**Package:** `internal/agents/buildresources/buildresources.go`

**Validates: BR-1 through BR-6**

---

## Implementation

```go
package buildresources

import (
    "context"
    "fmt"
    "strings"

    "github.com/koudis/bootstrap-ai-coding/internal/agent"
    "github.com/koudis/bootstrap-ai-coding/internal/constants"
    "github.com/koudis/bootstrap-ai-coding/internal/docker"
)

type buildResourcesAgent struct{}

func init() {
    agent.Register(&buildResourcesAgent{})
}

// ID returns the stable Agent_ID "build-resources".
// Satisfies: BR-1
func (a *buildResourcesAgent) ID() string {
    return constants.BuildResourcesAgentName
}

// Install appends Dockerfile RUN steps that install Python 3, uv, CMake,
// build-essential, OpenJDK, and Go.
// Satisfies: BR-2
func (a *buildResourcesAgent) Install(b *docker.DockerfileBuilder) {
    // All apt packages installed by this agent, listed explicitly for easy
    // modification and test assertions.
    aptPackages := []string{
        // Python
        "python3", "python3-pip", "python3-venv", "python3-dev",
        "python3-setuptools", "python3-wheel",
        // C/C++ build toolchain
        "build-essential", "cmake", "pkg-config",
        // Java
        "default-jdk",
        // Common build dependencies
        "libssl-dev", "libffi-dev",
        // Utilities
        "curl", "ca-certificates", "unzip", "wget",
    }

    // System packages (as root)
    b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends " +
        strings.Join(aptPackages, " ") +
        " && rm -rf /var/lib/apt/lists/*")

    // Go — official tarball to /usr/local/go
    b.Run("curl -fsSL https://go.dev/dl/go1.24.2.linux-$(dpkg --print-architecture).tar.gz | tar -C /usr/local -xz")
    b.Run("echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/golang.sh && chmod +x /etc/profile.d/golang.sh")

    // Python uv — installed system-wide to /usr/local/bin via official installer.
    // Using UV_INSTALL_DIR avoids user-local PATH issues with docker exec (runs as root).
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
        {[]string{"bash", "-lc", "uv --version"}, "uv"},
        {[]string{"cmake", "--version"}, "cmake"},
        {[]string{"javac", "-version"}, "javac"},
        {[]string{"bash", "-lc", "go version"}, "go"},
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
```

---

## Design Decisions

1. **Pseudo-agent pattern:** Reuses the existing agent module architecture (self-registration, `Install()`, `HealthCheck()`) rather than introducing a separate "toolchain installer" concept. This keeps the codebase uniform and means `--agents build-resources` works like any other agent for inclusion/exclusion.

2. **No credential store:** `CredentialStorePath()` and `ContainerMountPath()` return empty strings. The core skips bind-mount creation and credential checks for agents with empty paths. `HasCredentials()` returns `(true, nil)` so the core never prints a "please authenticate" message for this module.

3. **System-wide uv:** Python uv is installed to `/usr/local/bin` using `UV_INSTALL_DIR=/usr/local/bin` with the official installer. This avoids PATH issues when `docker exec` runs commands as root (where `$HOME` resolves to `/root`, not the container user's home). Since `/usr/local/bin` is on the default PATH for all users, no profile.d script or bashrc entry is needed.

4. **Go via official tarball:** The Go binary is installed from `go.dev/dl/` to `/usr/local/go` with PATH set via `/etc/profile.d/golang.sh`. This ensures the latest stable version regardless of what Ubuntu's package manager offers.

5. **Health check uses `bash -lc` only for Go:** Go is available via a PATH entry in `/etc/profile.d/golang.sh`. Running it through `bash -lc` ensures the login profile is sourced. All other tools (python3, uv, cmake, javac) are on the default PATH and don't need login shell invocation.

6. **`RunAsUser` builder method:** The `DockerfileBuilder` has a `RunAsUser(cmd string)` helper that emits `USER <username>` before the `RUN` and `USER root` after. While the Build Resources agent no longer uses it (all installs are system-wide), it remains available for future agents that need user-local installations.

7. **`goVersion` private constant:** The Go version is declared as a private `const goVersion` in the agent package, making it easy to bump without searching through string literals.

7. **Default inclusion:** Added to `constants.DefaultAgents` so it's always present unless the user explicitly overrides `--agents`. This means `go run . /path` installs Claude Code + Augment Code + Build Resources by default.

---

## DockerfileBuilder Extension: `RunAsUser`

The `DockerfileBuilder` provides a `RunAsUser(cmd string)` method for agent modules that need to run commands as the Container_User. While the Build Resources agent no longer uses it (all tools are installed system-wide), it remains available for future agents that need user-local installations.

```go
// RunAsUser emits a USER switch, runs the command as the container user,
// then switches back to root for subsequent instructions.
func (b *DockerfileBuilder) RunAsUser(cmd string) {
    b.lines = append(b.lines, fmt.Sprintf("USER %s", b.username))
    b.lines = append(b.lines, fmt.Sprintf("RUN %s", cmd))
    b.lines = append(b.lines, "USER root")
}
```

This keeps the Dockerfile generation self-contained within the builder and avoids agents needing to know the username directly (they call `b.RunAsUser()` and the builder handles the `USER` directives).

---

## Dockerfile Layer Order (with Build Resources)

When all default agents are enabled, the generated Dockerfile layers are split across two images (see [design-docker.md](design-docker.md) for the two-layer architecture):

**Base_Image (`bac-base:latest`):**
```
FROM ubuntu:26.04
RUN apt-get install openssh-server sudo         ← base
RUN useradd <username>                          ← stable per user
RUN sudoers                                     ← stable
RUN dbus-x11 gnome-keyring libsecret-1-0        ← keyring (CC-7)
RUN /etc/profile.d/dbus-keyring.sh              ← keyring startup
RUN gitconfig                                   ← git config (Req 24)
RUN curl ca-certificates git + nodejs           ← Claude/Augment shared deps
RUN npm install -g @anthropic-ai/claude-code    ← Claude Code
RUN npm install -g @augmentcode/auggie          ← Augment Code
RUN python3 cmake build-essential default-jdk …  ← Build Resources (system)
RUN go tarball + /etc/profile.d/golang.sh       ← Build Resources (Go)
RUN uv install (UV_INSTALL_DIR=/usr/local/bin)  ← Build Resources (uv)
RUN echo manifest > /bac-manifest.json          ← manifest
# NO CMD — that belongs in Instance_Image
```

**Instance_Image (`bac-<name>:latest`):**
```
FROM bac-base:latest
RUN SSH host key injection                      ← per-project (core Req 13)
RUN SSH authorized_keys                         ← per-user key (core Req 4)
RUN sshd_config hardening + Port/ListenAddress  ← per-project (Req 26.2)
RUN mkdir /run/sshd                             ← stable
CMD ["/usr/sbin/sshd", "-D"]                    ← always last (Req 21.2)
```
