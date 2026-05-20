# Project Structure

All packages live under `internal/`. The Go compiler enforces that nothing outside this module can import them. The only public surface is the compiled binary.

```
bootstrap-ai-coding/
├── main.go                              # Entry point; blank-imports agent modules to trigger init() registration
│
└── internal/
    ├── constants/
    │   └── constants.go                 # All constants from the requirements glossary — single source of truth
    │
    ├── pathutil/
    │   └── pathutil.go                  # Shared path helpers (ExpandHome) — zero internal dependencies
    │
    ├── cmd/
    │   └── root.go                      # Cobra root command, flag definitions, top-level orchestration logic
    │
    ├── naming/
    │   └── naming.go                    # Container name resolution from project path ("bac-" prefix, human-readable, collision-resistant)
    │
    ├── docker/
    │   ├── client.go                    # Docker SDK client wrapper; prerequisite checks (daemon reachable, version >= constants.MinDockerVersion)
    │   ├── builder.go                   # DockerfileBuilder — incremental Dockerfile assembly
    │   └── runner.go                    # Container create/start/stop/inspect helpers
    │
    ├── ssh/
    │   ├── keys.go                      # SSH public key discovery (~/.ssh/id_ed25519.pub → id_rsa.pub → --ssh-key)
    │   ├── known_hosts.go               # ~/.ssh/known_hosts sync
    │   └── ssh_config.go                # ~/.ssh/config sync
    │
    ├── datadir/
    │   ├── datadir.go                   # Tool_Data_Dir (~/.config/bootstrap-ai-coding/<name>/): port, host keys, manifest
    │   ├── credentials.go               # Credential store path resolution, directory creation (merged from credentials/)
    │   └── portfinder.go                # SSH port auto-selection starting at constants.SSHPortStart (merged from portfinder/)
    │
    ├── agent/
    │   ├── agent.go                     # Agent interface — the stable API boundary between core and agent modules
    │   ├── preparer.go                  # CredentialPreparer optional interface
    │   └── registry.go                  # AgentRegistry: Register / Lookup / All / KnownIDs
    │
    └── agents/
        ├── claude/
        │   └── claude.go                # Claude Code agent module (reference implementation)
        └── augment/
            └── augment.go               # Augment Code agent module
        # future agents: internal/agents/<name>/<name>.go — no core files change
```

## Architectural Rules

- **All packages live under `internal/`.** The Go compiler enforces that nothing outside this module can import them.
- **Core has zero knowledge of agents.** Packages under `internal/cmd/`, `internal/naming/`, `internal/docker/`, `internal/ssh/`, `internal/datadir/`, `internal/pathutil/`, and `internal/agent/` must never import anything under `internal/agents/`.
- **Agent modules are wired in via blank imports in `main.go` only.** Each agent's `init()` calls `agent.Register()`.
- **Agent modules may import `internal/agent`, `internal/docker`, `internal/constants`, and `internal/pathutil` from the core.** They must not import `internal/cmd`, `internal/naming`, `internal/ssh`, `internal/datadir`, or `internal/docker/runner`.
- **No package may hardcode values that exist in `internal/constants/`.** Always import and reference `constants.*`.
- **Path expansion (`~/` → home dir) must use `pathutil.ExpandHome`.** No package may define its own `expandHome` helper.
- **Adding a new agent = one new package under `internal/agents/`.** No other files change.

## Import Path Pattern

```go
// In main.go:
import (
    "github.com/koudis/bootstrap-ai-coding/internal/cmd"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
)

// In internal packages:
import (
    "github.com/koudis/bootstrap-ai-coding/internal/constants"
    "github.com/koudis/bootstrap-ai-coding/internal/pathutil"
    "github.com/koudis/bootstrap-ai-coding/internal/naming"
)
```

## Key Conventions

- Container names: `bac-<dirname>` derived from the project directory name (sanitized to `[a-z0-9_.-]`); falls back to `bac-<parentdir>_<dirname>` on conflict, then `bac-<parentdir>_<dirname>-2`, `-3`, … — checked only against existing `bac-`-prefixed containers
- Tool data directory: `~/.config/bootstrap-ai-coding/<container-name>/` — stores SSH port, SSH host key, agent manifest
- Base image: always `ubuntu:26.04` (constants.BaseContainerImage) — no other base image or Ubuntu version
- Container user: matches Host_User's username (resolved at runtime via `hostinfo.Current()`), UID/GID matching the host user who invoked the CLI
- Container user home: matches Host_User's home directory path (resolved at runtime via `hostinfo.Current()`)
- Workspace mount: `/workspace` (constants.WorkspaceMountPath)
- SSH port: starts at `2222` (constants.SSHPortStart), increments until free, persisted per project
- SSH host key type: `ed25519` (constants.SSHHostKeyType) — generated once per project, reused across rebuilds
- Manifest file inside image: `/bac-manifest.json` (constants.ManifestFilePath) — lists enabled agent IDs for rebuild detection
- Default agents: `claude-code,augment-code` (constants.DefaultAgents)
- File permissions: Tool_Data_Dir `0700` (constants.ToolDataDirPerm), all files within `0600` (constants.ToolDataFilePerm)
- Headless keyring: D-Bus session bus + gnome-keyring-daemon started via `/etc/profile.d/dbus-keyring.sh` on SSH login — enables libsecret-based credential storage (CC-7)
