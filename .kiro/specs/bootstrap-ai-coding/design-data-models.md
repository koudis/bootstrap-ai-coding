# Data Models, Error Handling & Test Infrastructure

This document defines the core data structures, error handling strategy, and integration test infrastructure for the `bootstrap-ai-coding` system.

> **Related documents:**
> - [design.md](design.md) — Overview and document index
> - [design-architecture.md](design-architecture.md) — High-level architecture, package layout, sequence diagrams
> - [design-components.md](design-components.md) — Core component designs
> - [design-docker.md](design-docker.md) — Two-layer Docker image architecture
> - [design-build-resources.md](design-build-resources.md) — Build Resources agent module design
> - [design-agents.md](design-agents.md) — Agent modules: contract, implementations
> - [design-properties.md](design-properties.md) — Correctness properties and testing strategy

---

## Core Data Models

### Mode

```go
type Mode int

const (
    ModeStart Mode = iota // ¬S ∧ ¬U — start or reconnect
    ModeStop              // S ∧ ¬U  — stop and remove
    ModePurge             // U ∧ ¬S  — remove all tool data
)

func ResolveMode(stopAndRemove, purge bool) (Mode, error)
```

### Config

```go
type Config struct {
    Mode               Mode
    ProjectPath        string
    EnabledAgents      []string
    SSHKeyPath         string
    SSHPort            int    // 0 = auto-select
    Rebuild            bool
    Verbose            bool
    NoUpdateKnownHosts bool
    NoUpdateSSHConfig  bool
    RestartPolicy      string // Docker restart policy (default: "unless-stopped")
    HostNetworkOff     bool   // Req 26: when true, use bridge mode instead of host network
    CredStoreOverrides map[string]string
    HostInfo           *hostinfo.Info  // Req 22: runtime-resolved host user identity
}
```

### ContainerSpec

```go
type ContainerSpec struct {
    Name           string
    ImageTag       string
    Dockerfile     string
    Mounts         []Mount
    SSHPort        int
    Labels         map[string]string
    NoCache        bool               // When true, disable Docker layer cache during image build
    HostNetworkOff bool               // Req 26: when true, use bridge mode; when false (default), use host network
    RestartPolicy  string             // Req 25: Docker restart policy name
    HostInfo       *hostinfo.Info     // Req 22: runtime-resolved host user identity (UID, GID, Username, HomeDir)
}

type Mount struct {
    HostPath      string
    ContainerPath string
    ReadOnly      bool
}
```

### SessionSummary

```go
type SessionSummary struct {
    DataDir       string
    ProjectDir    string
    SSHPort       int
    SSHConnect    string   // e.g. "ssh bac-my-project" (relies on SSH_Config_Entry from Req 19)
    EnabledAgents []string
    Username      string   // Req 22: from info.Username (for SSH connect display)
}
```

---

## Core Error Handling

### CLI Flag Combination Errors (validated before all other checks)

| Condition | Requirement | Behaviour |
|---|---|---|
| `--stop-and-remove` and `--purge` both set | CLI-1 | Descriptive error → stderr, exit 1 |
| START or STOP mode and `<project-path>` absent | CLI-2 | Usage message → stderr, exit 1 |
| PURGE mode and `<project-path>` provided | CLI-2 | Descriptive error → stderr, exit 1 |
| STOP or PURGE mode and any of `--agents`, `--port`, `--ssh-key`, `--rebuild`, `--no-update-known-hosts`, `--no-update-ssh-config`, `--verbose`, `--docker-restart-policy`, `--host-network-off` set | CLI-3 | Descriptive error naming the incompatible flag(s) → stderr, exit 1 |
| `--port` value outside 1024–65535 | CLI-5 | Descriptive error → stderr, exit 1 |
| `--agents` parses to empty list | CLI-6 | Descriptive error → stderr, exit 1 |
| `--agents` contains unknown agent ID | CLI-6 | Unknown ID + available IDs → stderr, exit 1 |
| `--docker-restart-policy` invalid value | CLI-7 | Valid values listed → stderr, exit 1 |

### Runtime Errors

| Failure Condition | Detection Point | Behaviour |
|---|---|---|
| CLI invoked as root (UID 0) | After flag validation | "Running as root is not permitted" → stderr, exit 1 |
| Project path missing | After flag validation | Descriptive error → stderr, exit 1 |
| No SSH public key found | SSH key discovery | Descriptive error → stderr, exit 1 |
| Docker daemon unreachable | Docker prerequisite check | "Start Docker" message → stderr, exit 1 |
| Docker version < `constants.MinDockerVersion` | Docker prerequisite check | Detected + required version → stderr, exit 1 |
| Duplicate agent registration | `agent.Register()` at startup | Panic (programming error, caught immediately) |
| Conflicting_Image_User found, user declines rename | UID/GID conflict check | "Cannot build without resolving UID/GID conflict" → stderr, exit 1 |
| Agent manifest mismatch | Image inspect on startup | "Run with --rebuild" message → stdout, exit 0 |
| Image build failure | Docker build | Build log → stderr, exit 1 |
| Image build timeout (`constants.ImageBuildTimeout`) | Docker build | Timeout error → stderr, exit 1 |
| Container start failure | Docker start | Stop container, error → stderr, exit 1 |
| SSH health check timeout | Post-start TCP poll | Stop container, error → stderr, exit 1 |
| Persisted port in use by another process | Port check before start | Port conflict message → stderr, exit 1 |
| `--stop-and-remove`, container not found | Docker inspect | Informational message → stdout, exit 0 |
| Container already running | Docker inspect before create | Session summary → stdout, exit 0 |
| `--purge` user declines confirmation | Confirmation prompt | Exit 0, nothing deleted |

---

## Integration Test Infrastructure

### Shared helpers (`internal/testutil`)

All integration test packages share common setup logic via `internal/testutil/consent.go` (gated by `//go:build integration`):

**`RequireIntegrationConsent()`** — checks `BAC_INTEGRATION_CONSENT` env var. If not set to `yes`, prints a warning to stderr and exits with code 1. Called from `TestMain` in every integration test package after verifying Docker is available.

**`EnsureBaseImageAbsent()`** — connects to Docker, checks if `constants.BaseContainerImage` is present locally, and removes it if so. This guarantees every suite starts from a clean slate: the first test that builds a container triggers a fresh pull of the base image. Called from `TestMain` after `RequireIntegrationConsent()`.

The consent check runs after the Docker availability check — if Docker is not installed, the suite proceeds directly to `m.Run()` and individual tests skip themselves gracefully.

### Consent gate

When `BAC_INTEGRATION_CONSENT` is **not** set to `yes`, the suite prints a warning to stderr and aborts with exit code 1.

```
WARNING: Integration tests interact with the local Docker daemon.
They may pull, build, delete, and update Docker images and containers.

To run these tests, set the environment variable:
  BAC_INTEGRATION_CONSENT=yes go test -tags integration ./...

Aborted — no consent given.
```

**To run integration tests:**

```bash
BAC_INTEGRATION_CONSENT=yes go test -tags integration -timeout 30m ./...
```

### Base image precondition

`EnsureBaseImageAbsent()` removes `constants.BaseContainerImage` from the local Docker store at the start of every integration suite. This ensures:

1. The auto-pull path is always exercised (the first test in each package triggers a pull)
2. No stale cached image can mask regressions in pull logic
3. Developers don't need to manually run `docker rmi` before testing

The `TestAFindConflictingUserPullsImageIfAbsent` test (in `internal/docker`) is named with an "A" prefix so it runs first alphabetically. It calls `FindConflictingUser` on an absent image and asserts the function succeeds (pulling the image automatically). All subsequent tests in the suite benefit from the now-cached image.
