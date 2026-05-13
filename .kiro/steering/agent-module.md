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

    "github.com/koudis/bootstrap-ai-coding/internal/agent"
    "github.com/koudis/bootstrap-ai-coding/internal/constants"
    "github.com/koudis/bootstrap-ai-coding/internal/docker"
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
// The homeDir parameter is the Container_User's home directory (resolved at runtime).
func (a *aiderAgent) ContainerMountPath(homeDir string) string {
    return filepath.Join(homeDir, ".aider")
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
// Called by the core after the container starts. The *docker.Client is passed
// through from the caller — do not create a new client.
func (a *aiderAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string) error {
    exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"aider", "--version"})
    if err != nil {
        return fmt.Errorf("aider health check failed: %w", err)
    }
    if exitCode != 0 {
        return fmt.Errorf("aider health check failed: exit code %d", exitCode)
    }
    return nil
}
```

### 3. Wire into main.go

Add a single blank import — this is the **only** core file that changes:

```go
import (
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/aider" // add this line
)
```

### 4. Add integration tests

Every agent package must include an integration test file at:

```
agents/<agent-name>/integration_test.go
```

Example: `agents/aider/integration_test.go`

Integration tests must be gated by the `integration` build tag and live in the same package directory as the agent:

```go
//go:build integration

package aider_test

import (
    "testing"
    "github.com/stretchr/testify/require"
)

func TestAiderAgentInstallsAndRuns(t *testing.T) {
    // setup: build image with this agent enabled
    // assert: agent binary is present and executable inside the container
    // assert: HealthCheck passes
    // teardown: stop and remove container
}
```

Conventions:
- Always clean up containers in `t.Cleanup()` or `defer`
- Use `t.TempDir()` for temporary project directories
- Skip gracefully if Docker is not available:
  ```go
  if _, err := exec.LookPath("docker"); err != nil {
      t.Skip("docker not available")
  }
  ```
- At minimum, cover: agent binary present, `HealthCheck` passes, credential mount path exists

### 5. Document in requirements-agents.md

Add a new section to `.kiro/specs/bootstrap-ai-coding/requirements-agents.md` following the same structure as the Claude Code section (CC-1 through CC-6).

## Interface Contract Summary

| Method | What it must return |
|---|---|
| `ID()` | Unique, stable, kebab-case string (e.g. `"claude-code"`, `"aider"`) |
| `Install(b)` | Appends `RUN` steps to `b`; must be idempotent and self-contained |
| `CredentialStorePath()` | Default host path for auth tokens; may use `~/` prefix |
| `ContainerMountPath(homeDir)` | Absolute path inside container; use `homeDir` parameter as base |
| `HasCredentials(path)` | `(true, nil)` if tokens exist; `(false, nil)` if empty; `(false, err)` on error |
| `HealthCheck(ctx, c, id)` | `nil` if agent is ready; non-nil error if not. `c` is the existing `*docker.Client` — do not create a new one. |
| `SummaryInfo(ctx, c, id)` | `([]KeyValue, nil)` with key:value pairs for session summary; `(nil, nil)` if nothing to report; `(nil, err)` on failure |

## Import Rules for Agent Modules

Agent modules may import:
- `github.com/koudis/bootstrap-ai-coding/internal/agent` — to call `agent.Register()`
- `github.com/koudis/bootstrap-ai-coding/internal/docker` — for `*docker.DockerfileBuilder` and `*docker.Client`
- `github.com/koudis/bootstrap-ai-coding/internal/constants` — for glossary values (agent name constants, etc.)
- `github.com/koudis/bootstrap-ai-coding/internal/pathutil` — for `ExpandHome` if needed
- Standard library packages

Agent modules must **NOT** import:
- `cmd`, `naming`, `ssh`, `datadir`, `docker/runner`
- Any other agent module

## Naming Convention

- Agent ID: kebab-case, lowercase (e.g. `"claude-code"`, `"aider"`, `"gemini-code"`)
- Package name: single word, lowercase (e.g. `claude`, `aider`, `gemini`)
- Directory: matches package name (e.g. `agents/claude/`, `agents/aider/`)
