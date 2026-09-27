# Part 2 — Agent Module Design

## Agent Module Contract

An agent module is a Go package under `internal/agents/`. It must:

1. Define a private struct that implements the `agent.Agent` interface.
2. Call `agent.Register()` in its `init()` function.
3. Import only `internal/agent`, `internal/docker`, and `internal/constants` from the core — never `internal/cmd`, `internal/naming`, `internal/ssh`, `internal/datadir`, or `internal/docker/runner`.
4. Be wired into the binary via a blank import in `main.go`.
5. Use `b.HomeDir()` (from `DockerfileBuilder`) in `Install()` for container-side paths that reference the user's home directory (Req 22).
6. Accept `homeDir string` in `ContainerMountPath()` — the runtime-resolved value from `*hostinfo.Info` — instead of referencing any hardcoded constant.

No other file in the repository needs to change when a new agent module is added.

## Claude Code Agent Module

**Package:** `agents/claude/claude.go`
**Agent ID:** `"claude-code"`
**Validates:** Agent Req CC-1 through CC-8

### Implementation

```go
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

const agentID = constants.ClaudeCodeAgentName

type claudeAgent struct{}

func init() {
    agent.Register(&claudeAgent{})
}

// ID returns the stable agent identifier. (CC-1)
func (c *claudeAgent) ID() string { return agentID }

// Install contributes Node.js LTS + Claude Code npm package install steps. (CC-2)
// Also bakes the host CLAUDE.md memory file into the image as a baseline (best-effort).
func (c *claudeAgent) Install(b *docker.DockerfileBuilder) {
    b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates git && rm -rf /var/lib/apt/lists/*")
    if !b.IsNodeInstalled() {
        b.Run("curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && DEBIAN_FRONTEND=noninteractive apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
        b.MarkNodeInstalled()
    }
    b.Run("npm install -g --no-fund --no-audit @anthropic-ai/claude-code")
    c.injectMemory(b)
}

// injectMemory bakes host ~/.claude/CLAUDE.md into the image using base64 encoding. (CC-2)
// The bind-mount at runtime takes precedence; this is a baseline only.
func (c *claudeAgent) injectMemory(b *docker.DockerfileBuilder) {
    home, err := os.UserHomeDir()
    if err != nil {
        return
    }
    data, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md"))
    if err != nil || len(data) == 0 {
        return
    }
    containerClaudeDir := filepath.Join(b.HomeDir(), ".claude")
    containerMemoryFile := filepath.Join(containerClaudeDir, "CLAUDE.md")
    encoded := base64.StdEncoding.EncodeToString(data)
    b.Run(fmt.Sprintf(
        "mkdir -p %s && echo %s | base64 -d > %s && chown -R %s:%s %s",
        containerClaudeDir, encoded, containerMemoryFile,
        b.Username(), b.Username(), containerClaudeDir,
    ))
}

// CredentialStorePath returns the default host-side credential directory. (CC-3)
func (c *claudeAgent) CredentialStorePath() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".claude")
}

// ContainerMountPath returns where credentials are mounted inside the container. (CC-3)
// homeDir is the runtime-resolved Container_User_Home from info.HomeDir (Req 22).
func (c *claudeAgent) ContainerMountPath(homeDir string) string {
    return filepath.Join(homeDir, ".claude")
}

// HasCredentials checks for ~/.claude/.credentials.json. (CC-4)
func (c *claudeAgent) HasCredentials(storePath string) (bool, error) {
    tokenFile := filepath.Join(storePath, ".credentials.json")
    _, err := os.Stat(tokenFile)
    if os.IsNotExist(err) {
        return false, nil
    }
    return err == nil, err
}

// AdditionalMounts returns the read-write bind-mount for ~/.claude.json. (CC-8)
// If the file does not exist on the host, the mount is omitted.
func (c *claudeAgent) AdditionalMounts(homeDir string) []docker.Mount {
    home, err := os.UserHomeDir()
    if err != nil {
        return nil
    }
    src := filepath.Join(home, ".claude.json")
    info, err := os.Stat(src)
    if err != nil || !info.Mode().IsRegular() {
        return nil
    }
    return []docker.Mount{
        {
            HostPath:      src,
            ContainerPath: filepath.Join(homeDir, ".claude.json"),
            ReadOnly:      false, // RW: changes inside the container persist back to the host
        },
    }
}

// HealthCheck verifies `claude --version` exits 0 inside the container. (CC-5)
func (c *claudeAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string, username string) error {
    exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"su", "-", username, "-c", "claude --version"})
    if err != nil {
        return fmt.Errorf("claude health check failed: %w", err)
    }
    if exitCode != 0 {
        return fmt.Errorf("claude health check failed: 'claude --version' exited with code %d", exitCode)
    }
    return nil
}
```

### Design Notes

- **`~/.claude` (credential directory)**: primary bind-mount, read-write — tokens written inside the container persist to the host automatically via `CredentialStorePath`/`ContainerMountPath`.
- **`~/.claude.json` (config file)**: additional bind-mount via `AdditionalMounts`, also read-write — Claude Code updates onboarding state, MCP server registrations, and preferences to this file at runtime; the RW mount ensures those changes are persisted back to the host.
- **Memory injection**: `~/.claude/CLAUDE.md` is base64-encoded and baked into the image at build time as a baseline. The runtime bind-mount takes precedence, so live edits to the host file are immediately visible in new sessions without rebuilding.

## Augment Code Agent Module

**Package:** `agents/augment/augment.go`
**Agent ID:** `"augment-code"`
**Validates:** Agent Req AC-1 through AC-6

### Overview

Augment Code's CLI tool is **Auggie**, distributed as the `@augmentcode/auggie` npm package. It requires Node.js 22 or later. Authentication tokens and settings are stored in `~/.augment` on the host. The agent is invoked inside the container via the `auggie` command.

### Implementation

```go
package augment

import (
    "context"
    "fmt"
    "os"
    "path/filepath"

    "github.com/koudis/bootstrap-ai-coding/internal/agent"
    "github.com/koudis/bootstrap-ai-coding/internal/constants"
    "github.com/koudis/bootstrap-ai-coding/internal/docker"
)

const agentID = "augment-code"

type augmentAgent struct{}

func init() {
    agent.Register(&augmentAgent{})
}

// ID returns the stable agent identifier. (AC-1)
func (a *augmentAgent) ID() string { return agentID }

// Install contributes Node.js 22+ and Auggie npm package install steps. (AC-2)
func (a *augmentAgent) Install(b *docker.DockerfileBuilder) {
    b.Run("apt-get update && apt-get install -y --no-install-recommends curl ca-certificates git && rm -rf /var/lib/apt/lists/*")
    b.Run("curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
    b.Run("npm install -g @augmentcode/auggie")
}

// CredentialStorePath returns the default host-side credential directory. (AC-3)
func (a *augmentAgent) CredentialStorePath() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".augment")
}

// ContainerMountPath returns where credentials are mounted inside the container. (AC-3)
// homeDir is the runtime-resolved Container_User_Home from info.HomeDir (Req 22).
func (a *augmentAgent) ContainerMountPath(homeDir string) string {
    return filepath.Join(homeDir, ".augment")
}

// HasCredentials checks whether ~/.augment/ is non-empty (contains any file). (AC-4)
// Augment Code does not document a specific token filename, so we probe for any
// non-empty file in the directory as evidence that the user has authenticated.
func (a *augmentAgent) HasCredentials(storePath string) (bool, error) {
    entries, err := os.ReadDir(storePath)
    if os.IsNotExist(err) {
        return false, nil
    }
    if err != nil {
        return false, fmt.Errorf("checking augment credentials: %w", err)
    }
    for _, e := range entries {
        if !e.IsDir() {
            info, err := e.Info()
            if err == nil && info.Size() > 0 {
                return true, nil
            }
        }
    }
    return false, nil
}

// HealthCheck verifies `auggie --version` exits 0 inside the container. (AC-5)
func (a *augmentAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string) error {
    exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"auggie", "--version"})
    if err != nil {
        return fmt.Errorf("augment health check failed: %w", err)
    }
    if exitCode != 0 {
        return fmt.Errorf("augment health check failed: 'auggie --version' exited with code %d", exitCode)
    }
    return nil
}
```

### Design Notes

- **Node.js version**: Auggie requires Node.js 22+. The install step uses `setup_22.x` from NodeSource, pinning to the Node.js 22 LTS line. This differs from the Claude Code module which uses `setup_lts.x`.
- **Credential detection**: Augment Code does not publicly document a specific token filename. The `HasCredentials` implementation probes for any non-empty file in `~/.augment/` as a proxy for "user has authenticated". This is conservative: a non-empty directory is treated as having credentials.
- **Login instruction**: When credentials are absent, the core will instruct the user to run `auggie login` inside the container.

---

## Adding a Future Agent

To add a new agent (e.g. `agents/aider/aider.go`):

1. Create the package implementing `agent.Agent`.
2. Call `agent.Register(&aiderAgent{})` in `init()`.
3. Add blank imports for all agent packages to `main.go`:
   ```go
   _ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
   _ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
   _ "github.com/koudis/bootstrap-ai-coding/internal/agents/aider"
   ```
4. Add a section to `requirements-agents.md` documenting the new agent's requirements.

No other files change.

**Note (Req 22):** `ContainerMountPath(homeDir string)` receives the runtime-resolved Container_User_Home from `info.HomeDir` (via `*hostinfo.Info`). Agent modules must use this parameter (not a hardcoded path) to construct their container-side credential mount path. The `DockerfileBuilder` also exposes `HomeDir()` for use in `Install()` steps that need to reference the container user's home directory.
