# Kiro Agent Module Design

This document describes the Kiro agent module, which pre-installs the Kiro CLI into the container and enables the Kiro IDE to connect remotely over the container's existing SSH server.

> **Related documents:**
> - [design.md](design.md) — Overview and document index
> - [design-architecture.md](design-architecture.md) — High-level architecture, package layout, sequence diagrams
> - [design-components.md](design-components.md) — Core component designs
> - [design-docker.md](design-docker.md) — Two-layer Docker image architecture
> - [design-data-models.md](design-data-models.md) — Data models, error handling, test infrastructure
> - [design-agents.md](design-agents.md) — Agent modules: contract, implementations
> - [design-build-resources.md](design-build-resources.md) — Build Resources pseudo-agent design (referenced for the install-location pattern)
> - [design-properties.md](design-properties.md) — Correctness properties and testing strategy
> - [requirements-kiro.md](agents/requirements-kiro.md) — Kiro agent requirements (KIRO-1 … KIRO-8)

---

## Overview

Kiro is AWS's AI-powered coding agent. The module serves two goals that share one image:

1. **Remote IDE connection (KIRO-3).** The container already runs `sshd` on the persisted `SSH_Port` and the tool writes an `ssh bac-<container-name>` alias to `~/.ssh/config`. The Kiro IDE on the Host uses its Open Remote - SSH capability to open the container as a remote workspace. The module contributes no SSH machinery — it only ensures the runtime dependency (Node.js) that Kiro's remote server bootstraps against is present.
2. **In-container Kiro CLI (KIRO-2).** The `kiro-cli` binary is installed via the official installer so it can be run directly over SSH.

Authentication state persists entirely through the `~/.kiro` bind-mount (KIRO-4, KIRO-5) — the same credential-persistence mechanism every other agent uses. There is **no** dependency on forwarding a Host `KIRO_API_KEY` into the container, because the core has no runtime environment-variable passthrough (`docker.ContainerSpec` has no env field and `CreateContainer` never sets `container.Config.Env`).

Kiro is **opt-in only** — it is not part of `constants.DefaultAgents`. Users enable it with `--agents kiro` (or e.g. `--agents claude-code,kiro`).

**Package:** `internal/agents/kiro/kiro.go`

**Validates: KIRO-1 through KIRO-8**

---

## Constant

A new Agent_ID constant is added to `internal/constants/constants.go` (no other package hardcodes the literal, per the constants rule and KIRO-8.5):

```go
// KiroAgentName is the stable Agent_ID for the Kiro CLI agent module.
// Corresponds to the Agent_ID glossary term for Kiro (KIRO-1). Opt-in only —
// NOT included in DefaultAgents.
KiroAgentName = "kiro"
```

`DefaultAgents` is **not** changed.

---

## Interface Mapping

| Method | Behaviour | Requirement |
|---|---|---|
| `ID()` | returns `constants.KiroAgentName` (`"kiro"`) | KIRO-1 |
| `Install(b)` | installs Node.js 22 (deduped) + Kiro CLI to a system-wide location | KIRO-2 |
| `CredentialStorePath()` | `~/.kiro` on the Host | KIRO-5 |
| `ContainerMountPath(homeDir)` | `<homeDir>/.kiro` | KIRO-5 |
| `HasCredentials(storePath)` | inspects `~/.kiro` for auth state | KIRO-6 |
| `HealthCheck(ctx, c, id, username)` | `kiro-cli --version` as the Container_User, exit 0 | KIRO-7 |
| `SummaryInfo(ctx, c, id)` | `(nil, nil)` (nothing to report) | SI-* |

The module does **not** implement `AdditionalMounter` (single credential directory) and does **not** implement `CredentialPreparer` (nothing to copy before start).

---

## Implementation

```go
// Package kiro implements the Kiro CLI agent module. It self-registers with
// the agent registry via init() and satisfies the agent.Agent interface. The
// core application has no direct dependency on this package — it is wired in
// exclusively via a blank import in main.go.
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
// official installer, placed in a system-wide location so both `docker exec`
// (running as root) and the Container_User's login shell can find `kiro-cli`.
// Satisfies: KIRO-2, KIRO-4.4
func (a *kiroAgent) Install(b *docker.DockerfileBuilder) {
    b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates git unzip && rm -rf /var/lib/apt/lists/*")
    if !b.IsNodeInstalled() {
        b.Run("curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && DEBIAN_FRONTEND=noninteractive apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
        b.MarkNodeInstalled()
    }

    // Install the Kiro CLI non-interactively. The official installer places the
    // binary under a user-local dir (~/.local/bin) by default, which is NOT on
    // the default PATH for `docker exec` (runs as root, HOME=/root). We install
    // and then symlink the resulting binary into /usr/local/bin so it is on the
    // default PATH for every user and every exec context. (See design note 2.)
    b.Run("curl -fsSL https://cli.kiro.dev/install | bash")
    b.Run("ln -sf \"$(find / -name kiro-cli -type f 2>/dev/null | head -n1)\" /usr/local/bin/kiro-cli")

    // Pre-create the credential/config directory so the runtime bind-mount does
    // not cause intermediate parents to be created as root, blocking writes by
    // the non-root Container_User. (Same pattern as the OpenCode module.)
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
// authentication state. Returns (false, nil) when the directory is absent or
// contains no auth state — not an error.
// Satisfies: KIRO-6
func (a *kiroAgent) HasCredentials(storePath string) (bool, error) {
    entries, err := os.ReadDir(storePath)
    if os.IsNotExist(err) {
        return false, nil
    }
    if err != nil {
        return false, fmt.Errorf("checking kiro credentials: %w", err)
    }
    // Any non-empty regular file under ~/.kiro is treated as evidence that the
    // user has established auth/config on the Host. Kiro does not document a
    // single stable token filename, so this mirrors the Augment module's
    // conservative "non-empty directory" heuristic.
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

// SummaryInfo returns no additional session summary information.
// Satisfies: SI-*
func (a *kiroAgent) SummaryInfo(ctx context.Context, c *docker.Client, containerID string) ([]agent.KeyValue, error) {
    return nil, nil
}
```

---

## Design Decisions

1. **Bind-mount is the only credential path (KIRO-4).** All authentication and configuration persistence rides on the `~/.kiro` ↔ `<homeDir>/.kiro` bind-mount, identical to Claude/Codex/OpenCode. No Host env var is forwarded — the core does not support runtime env passthrough, and baking a secret via `ENV` would leak it into image layers and `docker inspect`. The design therefore never touches `container.Config.Env` and requires **zero core changes** beyond the new constant.

2. **System-wide binary location.** The official installer (`curl -fsSL https://cli.kiro.dev/install | bash`) drops `kiro-cli` in a user-local directory (e.g. `~/.local/bin`) that is not on the default PATH when `docker exec` runs as root (`HOME=/root`). This is the same class of problem `design-build-resources.md` solves for `uv`/`graphify`. We resolve it by symlinking the installed binary into `/usr/local/bin`, which is on every user's default PATH — so both the `HealthCheck` exec and interactive SSH sessions find `kiro-cli` without a login-shell workaround. The `find … | head -n1` locates the installed binary regardless of the exact install directory the script chooses.

3. **Health check runs as the Container_User (`su - username -c …`).** Matches the Codex/OpenCode convention. Running under a login shell (`su -`) also sources `/etc/profile.d`, so PATH is correct even if a future installer changes location. `--version` is a non-interactive, no-auth command, so the check passes regardless of whether the user has signed in.

4. **`unzip` in the apt step.** The Kiro installer may download a ZIP archive on some platforms (documented for Arm Linux). Installing `unzip` up front keeps the install non-interactive and robust across architectures. `curl`/`ca-certificates`/`git` mirror the other Node-based agents.

5. **Node.js deduplication.** Uses `IsNodeInstalled()`/`MarkNodeInstalled()` so that when Kiro is combined with another Node-based agent (Claude, Augment, Codex, OpenCode) only one Node.js layer is emitted. Kiro pins `setup_22.x` for consistency with the other modules.

6. **`HasCredentials` heuristic.** Kiro does not publish a single stable token filename, so the check treats any non-empty regular file under `~/.kiro` as "authenticated" — the same conservative approach the Augment module uses. When it returns `false`, the core prints its standard "please authenticate" guidance (KIRO-6.5); for Kiro that means signing in via the IDE or `kiro-cli` so state lands in `~/.kiro`.

7. **No `AdditionalMounter`, no `CredentialPreparer`.** A single directory (`~/.kiro`) covers config and auth, so the optional interfaces are unnecessary. Keeping the surface minimal matches the Codex module.

8. **Opt-in, not default (KIRO-8 note).** `KiroAgentName` is added to `constants` but deliberately left out of `DefaultAgents`, matching Codex/OpenCode. This avoids installing Kiro (and its Node layer) for users who did not ask for it.

9. **Pre-create `~/.kiro` and `chown` at build time.** As with OpenCode, this prevents Docker from creating bind-mount parent directories as root, which would otherwise block the non-root Container_User from writing config/auth state at runtime.

---

## Wiring into `main.go`

A single blank import is added — the only core file that changes:

```go
import (
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/buildresources"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/codex"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/kiro" // add this line
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/opencode"
)
```

---

## Dockerfile Layer Order (with `--agents kiro` alongside a Node agent)

Kiro's steps are appended to the **Base_Image** builder like every other agent (agent `Install()` runs before `Finalize()`), after any earlier agent has already installed Node.js:

```
FROM ubuntu:26.04
… base: openssh-server, user, sudo, dbus/keyring, workspace-cd, gitconfig …
RUN curl ca-certificates git + nodejs               ← shared Node (first Node agent)
RUN npm install -g <first Node agent>               ← e.g. claude-code (if enabled)
RUN apt-get install curl ca-certificates git unzip  ← Kiro deps
# (Node dedup: skipped because IsNodeInstalled() is true)
RUN curl … cli.kiro.dev/install | bash              ← Kiro CLI
RUN ln -sf $(find … kiro-cli) /usr/local/bin/kiro-cli ← put on PATH
RUN mkdir -p <home>/.kiro && chown …                ← credential dir
RUN echo manifest > /bac-manifest.json              ← manifest (includes "kiro")
# NO CMD — belongs to the Instance_Image
```

If Kiro is the **only** enabled agent, its own `IsNodeInstalled()` check is false, so it installs Node.js itself before the CLI.

---

## Testing Strategy

### Unit tests — `internal/agents/kiro/kiro_test.go` (no build tag)

Pure, Docker-free assertions on the module's contract:

- `ID()` returns `constants.KiroAgentName` == `"kiro"`.
- `Install(b)` on a fresh builder emits a Dockerfile containing: the Kiro installer URL (`cli.kiro.dev/install`), the `/usr/local/bin/kiro-cli` symlink, a Node.js `setup_22.x` step, and the `~/.kiro` mkdir+chown.
- `Install(b)` on a builder where `MarkNodeInstalled()` was already called does **not** emit a second Node.js `setup_22.x` step (dedup).
- `CredentialStorePath()` ends with `.kiro`; `ContainerMountPath("/home/dev")` == `/home/dev/.kiro`.
- `HasCredentials` returns `(false, nil)` for a missing dir and an empty `t.TempDir()`; `(true, nil)` when a non-empty file exists in the dir.
- The agent is discoverable via `agent.Lookup(constants.KiroAgentName)` (import the package for its `init()`), confirming self-registration (KIRO-8).

### Integration test — `internal/agents/kiro/integration_test.go` (`//go:build integration`)

Mirrors the Codex/OpenCode scaffold exactly (shared container built once in `TestMain`, consent gate via `testutil.RequireIntegrationConsent()`, `testutil.EnsureBaseImageAbsent()`, `t.Skip` when Docker is absent, `-p 1`):

- **`TestKiroAvailableInContainer`** — `docker.ExecInContainer(… {"kiro-cli", "--version"})` exits 0 (KIRO-2.3).
- **`TestKiroHealthCheck`** — `agent.Lookup(constants.KiroAgentName)` then `HealthCheck(ctx, client, name, username)` returns nil (KIRO-7).
- **`TestKiroCredentialMountExists`** — mount a `t.TempDir()` at `<info.HomeDir>/.kiro`, then `ls -d` that path inside the container exits 0 (KIRO-5).

Because the Kiro install pulls a Node layer + the CLI, the setup uses the same extended `WaitForSSH` timeout (120s) as Codex/OpenCode.

---

## Adding to `design-agents.md`

The "Adding a Future Agent" section's `main.go` example is extended to include the `kiro` blank import, and the agent-module index table in `requirements-agents.md` gains the Kiro row (already added). No other design or core file changes.
