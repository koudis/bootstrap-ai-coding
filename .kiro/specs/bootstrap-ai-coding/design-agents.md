# Part 2 — Agent Module Design

## Agent Module Contract

An agent module is a Go package under `internal/agents/`. It must:

1. Define a private struct that implements the `agent.Agent` interface.
2. Call `agent.Register()` in its `init()` function.
3. Import only `internal/agent`, `internal/docker`, and `internal/constants` from the core — never `internal/cmd`, `internal/naming`, `internal/ssh`, `internal/credentials`, `internal/datadir`, `internal/portfinder`, or `internal/docker/runner`.
4. Be wired into the binary via a blank import in `main.go`.

No other file in the repository needs to change when a new agent module is added.

## Claude Code Agent Module

**Package:** `agents/claude/claude.go`
**Agent ID:** `"claude-code"`
**Validates:** Agent Req CC-1 through CC-6

### Implementation

```go
package claude

import (
    "context"
    "os"
    "path/filepath"

    "github.com/koudis/bootstrap-ai-coding/internal/agent"
    "github.com/koudis/bootstrap-ai-coding/internal/constants"
    "github.com/koudis/bootstrap-ai-coding/internal/docker"
)

const agentID = constants.DefaultAgent

type claudeAgent struct{}

func init() {
    agent.Register(&claudeAgent{})
}

// ID returns the stable agent identifier. (CC-1)
func (c *claudeAgent) ID() string { return agentID }

// Install contributes Node.js LTS + Claude Code npm package install steps. (CC-2)
func (c *claudeAgent) Install(b *docker.DockerfileBuilder) {
    b.Run("apt-get update && apt-get install -y --no-install-recommends curl ca-certificates git && rm -rf /var/lib/apt/lists/*")
    b.Run("curl -fsSL https://deb.nodesource.com/setup_lts.x | bash - && apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
    b.Run("npm install -g @anthropic-ai/claude-code")
}

// CredentialStorePath returns the default host-side credential directory. (CC-3)
func (c *claudeAgent) CredentialStorePath() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".claude")
}

// ContainerMountPath returns where credentials are mounted inside the container. (CC-3)
func (c *claudeAgent) ContainerMountPath() string {
    return filepath.Join(constants.ContainerUserHome, ".claude")
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

// HealthCheck verifies `claude --version` exits 0 inside the container. (CC-5)
func (c *claudeAgent) HealthCheck(ctx context.Context, containerID string) error {
    return execInContainer(ctx, containerID, []string{"claude", "--version"})
}
```

## Adding a Future Agent

To add a new agent (e.g. `agents/aider/aider.go`):

1. Create the package implementing `agent.Agent`.
2. Call `agent.Register(&aiderAgent{})` in `init()`.
3. Add `_ "github.com/koudis/bootstrap-ai-coding/internal/agents/aider"` to `main.go`.
4. Add a section to `requirements-agents.md` documenting the new agent's requirements.

No other files change.
