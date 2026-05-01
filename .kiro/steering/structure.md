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
    │   └── keys.go                      # SSH public key discovery (~/.ssh/id_ed25519.pub → id_rsa.pub → --ssh-key)
    │
    ├── credentials/
    │   └── store.go                     # Credential store path resolution, directory creation, agent-agnostic
    │
    ├── datadir/
    │   └── datadir.go                   # Tool_Data_Dir (~/.config/bootstrap-ai-coding/<name>/): port, host keys, manifest
    │
    ├── portfinder/
    │   └── portfinder.go                # SSH port auto-selection starting at constants.SSHPortStart, incrementing until free
    │
    ├── agent/
    │   ├── agent.go                     # Agent interface — the stable API boundary between core and agent modules
    │   └── registry.go                  # AgentRegistry: Register / Lookup / All / KnownIDs
    │
    └── agents/
        └── claude/
            └── claude.go                # Claude Code agent module (reference implementation)
        # future agents: internal/agents/<name>/<name>.go — no core files change
```

## Architectural Rules

- **All packages live under `internal/`.** The Go compiler enforces that nothing outside this module can import them.
- **Core has zero knowledge of agents.** Packages under `internal/cmd/`, `internal/naming/`, `internal/docker/`, `internal/ssh/`, `internal/credentials/`, `internal/datadir/`, `internal/portfinder/`, and `internal/agent/` must never import anything under `internal/agents/`.
- **Agent modules are wired in via blank imports in `main.go` only.** Each agent's `init()` calls `agent.Register()`.
- **Agent modules may import `internal/agent`, `internal/docker`, and `internal/constants` from the core.** They must not import `internal/cmd`, `internal/naming`, `internal/ssh`, `internal/credentials`, `internal/datadir`, `internal/portfinder`, or `internal/docker/runner`.
- **No package may hardcode values that exist in `internal/constants/`.** Always import and reference `constants.*`.
- **Adding a new agent = one new package under `internal/agents/`.** No other files change.

## Import Path Pattern

```go
// In main.go:
import (
    "github.com/koudis/bootstrap-ai-coding/internal/cmd"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
)

// In internal packages:
import (
    "github.com/koudis/bootstrap-ai-coding/internal/constants"
    "github.com/koudis/bootstrap-ai-coding/internal/naming"
)
```

## Key Conventions

- Container names: `bac-<dirname>` derived from the project directory name (sanitized to `[a-z0-9_.-]`); falls back to `bac-<parentdir>_<dirname>` on conflict, then `bac-<parentdir>_<dirname>-2`, `-3`, … — checked only against existing `bac-`-prefixed containers
- Tool data directory: `~/.config/bootstrap-ai-coding/<container-name>/` — stores SSH port, SSH host key, agent manifest
- Base image: always `ubuntu:26.04` (constants.BaseContainerImage) — no other base image or Ubuntu version
- Container user: `dev` (constants.ContainerUser), UID/GID matching the host user who invoked the CLI
- Container user home: `/home/dev` (constants.ContainerUserHome)
- Workspace mount: `/workspace` (constants.WorkspaceMountPath)
- SSH port: starts at `2222` (constants.SSHPortStart), increments until free, persisted per project
- SSH host key type: `ed25519` (constants.SSHHostKeyType) — generated once per project, reused across rebuilds
- Manifest file inside image: `/bac-manifest.json` (constants.ManifestFilePath) — lists enabled agent IDs for rebuild detection
- Default agent: `claude-code` (constants.DefaultAgent)
- File permissions: Tool_Data_Dir `0700` (constants.ToolDataDirPerm), all files within `0600` (constants.ToolDataFilePerm)
