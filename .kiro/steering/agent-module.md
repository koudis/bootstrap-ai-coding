# Adding a New Agent Module

Agent modules are self-contained Go packages under `agents/`. Adding a new agent requires **no changes to any core file** — only a new package.

## Step-by-Step

### 1. Create the package

```
agents/<agent-name>/<agent-name>.go
```

Example: `agents/aider/aider.go`

### 2. Implement the `agent.Agent` interface

Every agent must implement all six methods:

```go
package aider

import (
    "context"
    "os"
    "path/filepath"

    "github.com/user/bootstrap-ai-coding/agent"
    "github.com/user/bootstrap-ai-coding/constants"
    "github.com/user/bootstrap-ai-coding/docker"
)

const agentID = "aider" // must be unique, stable, kebab-case

type aiderAgent struct{}

func init() {
    agent.Register(&aiderAgent{}) // self-registers — no core changes needed
}

// ID returns the unique stable identifier used in --agents flag.
func (a *aiderAgent) ID() string { return agentID }

// Install appends Dockerfile RUN steps to install this agent.
// The builder already has: ubuntu:26.04 base, openssh-server, dev user.
// Do NOT assume any other agent's steps have run.
func (a *aiderAgent) Install(b *docker.DockerfileBuilder) {
    b.Run("pip install aider-chat")
}

// CredentialStorePath returns the default host-side credential directory.
// Use "~/" prefix — the core will expand it.
func (a *aiderAgent) CredentialStorePath() string {
    home, _ := os.UserHomeDir()
    return filepath.Join(home, ".aider")
}

// ContainerMountPath returns where credentials are mounted inside the container.
// Always use constants.ContainerUserHome as the base — never hardcode "/home/dev".
func (a *aiderAgent) ContainerMountPath() string {
    return filepath.Join(constants.ContainerUserHome, ".aider")
}

// HasCredentials reports whether the credential store contains valid auth tokens.
// Return (false, nil) when empty — not an error.
func (a *aiderAgent) HasCredentials(storePath string) (bool, error) {
    tokenFile := filepath.Join(storePath, ".env") // agent-specific token file
    _, err := os.Stat(tokenFile)
    if os.IsNotExist(err) {
        return false, nil
    }
    return err == nil, err
}

// HealthCheck verifies the agent is installed and runnable inside the container.
// Called by the core after the container starts.
func (a *aiderAgent) HealthCheck(ctx context.Context, containerID string) error {
    return execInContainer(ctx, containerID, []string{"aider", "--version"})
}
```

### 3. Wire into main.go

Add a single blank import — this is the **only** core file that changes:

```go
import (
    _ "github.com/user/bootstrap-ai-coding/agents/claude"
    _ "github.com/user/bootstrap-ai-coding/agents/aider" // add this line
)
```

### 4. Document in requirements-agents.md

Add a new section to `.kiro/specs/bootstrap-ai-coding/requirements-agents.md` following the same structure as the Claude Code section (CC-1 through CC-6).

## Interface Contract Summary

| Method | What it must return |
|---|---|
| `ID()` | Unique, stable, kebab-case string (e.g. `"claude-code"`, `"aider"`) |
| `Install(b)` | Appends `RUN` steps to `b`; must be idempotent and self-contained |
| `CredentialStorePath()` | Default host path for auth tokens; may use `~/` prefix |
| `ContainerMountPath()` | Absolute path inside container; use `constants.ContainerUserHome` as base |
| `HasCredentials(path)` | `(true, nil)` if tokens exist; `(false, nil)` if empty; `(false, err)` on error |
| `HealthCheck(ctx, id)` | `nil` if agent is ready; non-nil error if not |

## Import Rules for Agent Modules

Agent modules may import:
- `github.com/user/bootstrap-ai-coding/agent` — to call `agent.Register()`
- `github.com/user/bootstrap-ai-coding/docker` — for `*docker.DockerfileBuilder`
- `github.com/user/bootstrap-ai-coding/constants` — for `ContainerUserHome` and other glossary values
- Standard library packages

Agent modules must **NOT** import:
- `cmd`, `naming`, `ssh`, `credentials`, `datadir`, `portfinder`, `docker/runner`
- Any other agent module

## Naming Convention

- Agent ID: kebab-case, lowercase (e.g. `"claude-code"`, `"aider"`, `"gemini-code"`)
- Package name: single word, lowercase (e.g. `claude`, `aider`, `gemini`)
- Directory: matches package name (e.g. `agents/claude/`, `agents/aider/`)
