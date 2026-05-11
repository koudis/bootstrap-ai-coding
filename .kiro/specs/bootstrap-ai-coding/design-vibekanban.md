# Vibe Kanban Agent Module Design

This document describes the Vibe Kanban agent module, a web-based project management tool that runs as a background service inside the container.

> **Related documents:**
> - [design.md](design.md) - Overview and document index
> - [design-architecture.md](design-architecture.md) - High-level architecture
> - [design-agents.md](design-agents.md) - Agent modules: contract, implementations
> - [design-build-resources.md](design-build-resources.md) - Build Resources agent module
> - [requirements-agents.md](requirements-agents.md) - VK-1 through VK-8

---

## Overview

Vibe Kanban is a web-based kanban board for AI coding agents, distributed as the `vibe-kanban` npm package. Unlike other agent modules (which are CLI tools invoked on demand), Vibe Kanban is a **web application** that must be running as a background service after container start so the user can access it from their host browser.

The key design challenges are:
1. Auto-starting the service without replacing the container CMD (`/usr/sbin/sshd -D`)
2. Crash recovery with backoff to prevent resource exhaustion
3. Discovering the auto-assigned port for the session summary
4. Running as the Container_User (not root)

**Package:** `internal/agents/vibekanban/vibekanban.go`

**Validates: VK-1 through VK-8**

---

## Architecture

### Auto-Start Mechanism: ENTRYPOINT Wrapper Script

The container CMD is `["/usr/sbin/sshd", "-D"]`, set by `Finalize()` on the instance builder. Agent `Install()` methods run on the **base image builder** and cannot modify CMD. However, Docker's ENTRYPOINT + CMD interaction provides the solution:

- When both ENTRYPOINT and CMD are set, Docker executes: `ENTRYPOINT <CMD args...>`
- The agent installs a wrapper script at `/usr/local/bin/bac-entrypoint.sh` and sets `ENTRYPOINT ["/usr/local/bin/bac-entrypoint.sh"]`
- The wrapper starts background services, then exec's `"$@"` (which receives `/usr/sbin/sshd -D` from CMD)

This is the standard Docker pattern for initialization before the main process. It does NOT modify CMD - it adds ENTRYPOINT.

```mermaid
sequenceDiagram
    participant Docker
    participant Entrypoint as bac-entrypoint.sh
    participant Supervisor as vibe-kanban-supervisor.sh
    participant VK as vibe-kanban
    participant SSHD as sshd -D

    Docker->>Entrypoint: Start (args: /usr/sbin/sshd -D)
    Entrypoint->>Supervisor: Launch in background (&)
    Supervisor->>VK: Start as Container_User
    Entrypoint->>SSHD: exec "$@"
    Note over SSHD: PID 1, handles signals
    Note over Supervisor: Monitors VK, restarts on crash

```

### Crash Recovery: Supervisor Script

The supervisor script (`/usr/local/bin/vibe-kanban-supervisor.sh`) implements crash recovery with backoff:

- Runs in an infinite loop, starting `vibe-kanban` each iteration
- Tracks restart timestamps in an array
- Before each restart, checks if 5 restarts have occurred in the last 60 seconds
- If the limit is hit, logs an error and exits (preventing resource exhaustion)
- Sleeps 5 seconds between restart attempts
- Runs the vibe-kanban process as the Container_User via `su -c`

### Port Discovery

After the container starts and the health check passes, the CLI discovers the Vibe Kanban port by:

1. Executing `ss -tlnp` inside the container to list listening TCP sockets
2. Grepping for the `vibe-kanban` process
3. Parsing the port number from the output
4. Retrying up to 30 seconds (the server may take time to bind)

This is done via `docker.ExecInContainer()` with a shell pipeline.

### DockerfileBuilder Extension

The `DockerfileBuilder` needs a new `Entrypoint()` method:

```go
// Entrypoint appends an ENTRYPOINT instruction in exec form.
func (b *DockerfileBuilder) Entrypoint(args ...string) {
    quoted := make([]string, len(args))
    for i, a := range args {
        quoted[i] = fmt.Sprintf("%q", a)
    }
    b.lines = append(b.lines, fmt.Sprintf("ENTRYPOINT [%s]", strings.Join(quoted, ", ")))
}
```

When `Finalize()` emits `CMD ["/usr/sbin/sshd", "-D"]`, Docker will execute:
`/usr/local/bin/bac-entrypoint.sh /usr/sbin/sshd -D`

---

## Components and Interfaces

### Constants Addition

```go
// In internal/constants/constants.go:

// VibeKanbanAgentName is the stable Agent_ID for the Vibe Kanban agent module.
// Corresponds to the Agent_ID glossary term for Vibe Kanban (VK-1).
VibeKanbanAgentName = "vibe-kanban"
```

The `DefaultAgents` constant must be updated to include `vibe-kanban`:

```go
DefaultAgents = ClaudeCodeAgentName + "," + AugmentCodeAgentName + "," + BuildResourcesAgentName + "," + VibeKanbanAgentName
```

### Agent Module Interface Implementation

| Method | Return Value |
|--------|-------------|
| `ID()` | `constants.VibeKanbanAgentName` ("vibe-kanban") |
| `Install(b)` | Appends Node.js (conditional), npm install, entrypoint + supervisor scripts |
| `CredentialStorePath()` | `""` (no credentials) |
| `ContainerMountPath(homeDir)` | `""` (no bind-mount) |
| `HasCredentials(storePath)` | `(true, nil)` always |
| `HealthCheck(ctx, c, containerID)` | Binary check + process running check with retries |

### Session Summary Extension

The `SessionSummary` struct in `cmd/root.go` needs a new field:

```go
type SessionSummary struct {
    DataDir       string
    ProjectDir    string
    SSHPort       int
    SSHConnect    string
    EnabledAgents []string
    VibeKanbanURL string // empty if not discovered or not enabled
}
```

`FormatSessionSummary` conditionally includes the Vibe Kanban line:

```go
func FormatSessionSummary(s SessionSummary) string {
    var sb strings.Builder
    fmt.Fprintf(&sb, "Data directory:  %s\n", s.DataDir)
    fmt.Fprintf(&sb, "Project directory: %s\n", s.ProjectDir)
    fmt.Fprintf(&sb, "SSH port:        %d\n", s.SSHPort)
    fmt.Fprintf(&sb, "SSH connect:     %s\n", s.SSHConnect)
    fmt.Fprintf(&sb, "Enabled agents:  %s\n", strings.Join(s.EnabledAgents, ", "))
    if s.VibeKanbanURL != "" {
        fmt.Fprintf(&sb, "Vibe Kanban:     %s\n", s.VibeKanbanURL)
    }
    return sb.String()
}
```

### Port Discovery Function

A new exported function in `internal/docker/` for discovering a process's listening port:

```go
// DiscoverListeningPort executes `ss -tlnp` inside the container and returns
// the first port where the given process name is listening. Returns 0 if not found.
func DiscoverListeningPort(ctx context.Context, c *Client, containerID string, processName string) (int, error) {
    // Uses ExecInContainerWithOutput (new helper) to capture stdout
    // Parses ss output for lines containing processName
    // Extracts port from the Local Address:Port column
}
```

---

## Data Models

### Generated Scripts

**`/usr/local/bin/bac-entrypoint.sh`** (installed by Install()):
```bash
#!/bin/bash
set -e

# Start Vibe Kanban supervisor in background
/usr/local/bin/vibe-kanban-supervisor.sh &

# Execute the original CMD (sshd -D)
exec "$@"
```

**`/usr/local/bin/vibe-kanban-supervisor.sh`** (installed by Install()):
```bash
#!/bin/bash
# Vibe Kanban supervisor with crash recovery
# Max 5 restarts within any 60-second window, 5-second delay between attempts

MAX_RESTARTS=5
WINDOW_SECONDS=60
DELAY_SECONDS=5
RESTART_TIMES=()
USERNAME="__USERNAME__"

while true; do
    # Prune timestamps older than the window
    NOW=$(date +%s)
    PRUNED=()
    for ts in "${RESTART_TIMES[@]}"; do
        if (( NOW - ts < WINDOW_SECONDS )); then
            PRUNED+=("$ts")
        fi
    done
    RESTART_TIMES=("${PRUNED[@]}")

    # Check if we've exceeded the restart limit
    if (( ${#RESTART_TIMES[@]} >= MAX_RESTARTS )); then
        echo "vibe-kanban-supervisor: exceeded $MAX_RESTARTS restarts in ${WINDOW_SECONDS}s, giving up" >&2
        exit 1
    fi

    # Record this restart attempt
    RESTART_TIMES+=("$(date +%s)")

    # Start vibe-kanban as the container user
    su -c "vibe-kanban" "$USERNAME" || true

    # Wait before restarting
    sleep "$DELAY_SECONDS"
done
```

The `__USERNAME__` placeholder is replaced at image build time with the actual Container_User username from `b.Username()`.

---

## Implementation

```go
package vibekanban

import (
    "context"
    "fmt"
    "time"

    "github.com/koudis/bootstrap-ai-coding/internal/agent"
    "github.com/koudis/bootstrap-ai-coding/internal/constants"
    "github.com/koudis/bootstrap-ai-coding/internal/docker"
)

type vibeKanbanAgent struct{}

func init() {
    agent.Register(&vibeKanbanAgent{})
}

// ID returns the stable Agent_ID "vibe-kanban".
// Satisfies: VK-1
func (a *vibeKanbanAgent) ID() string {
    return constants.VibeKanbanAgentName
}

// Install appends Dockerfile RUN steps that install Node.js (if not already
// installed), the vibe-kanban npm package, and the auto-start mechanism.
// Satisfies: VK-2, VK-3
func (a *vibeKanbanAgent) Install(b *docker.DockerfileBuilder) {
    // 1. Node.js (conditional — skip if another agent already installed it)
    if !b.IsNodeInstalled() {
        b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates && rm -rf /var/lib/apt/lists/*")
        b.Run("curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && DEBIAN_FRONTEND=noninteractive apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
        b.MarkNodeInstalled()
    }

    // 2. Install vibe-kanban globally
    b.Run("npm install -g --no-fund --no-audit vibe-kanban")

    // 3. Install the supervisor script with crash recovery
    username := b.Username()
    supervisorScript := fmt.Sprintf(`#!/bin/bash
MAX_RESTARTS=5
WINDOW_SECONDS=60
DELAY_SECONDS=5
RESTART_TIMES=()
while true; do
    NOW=$(date +%%s)
    PRUNED=()
    for ts in "${RESTART_TIMES[@]}"; do
        if (( NOW - ts < WINDOW_SECONDS )); then
            PRUNED+=("$ts")
        fi
    done
    RESTART_TIMES=("${PRUNED[@]}")
    if (( ${#RESTART_TIMES[@]} >= MAX_RESTARTS )); then
        echo "vibe-kanban-supervisor: exceeded $MAX_RESTARTS restarts in ${WINDOW_SECONDS}s, giving up" >&2
        exit 1
    fi
    RESTART_TIMES+=("$(date +%%s)")
    su -c "vibe-kanban --host 0.0.0.0" "%s" || true
    sleep "$DELAY_SECONDS"
done`, username)

    b.Run(fmt.Sprintf("printf '%%s' '%s' > /usr/local/bin/vibe-kanban-supervisor.sh && chmod +x /usr/local/bin/vibe-kanban-supervisor.sh",
        supervisorScript))

    // 4. Install the entrypoint wrapper
    entrypoint := `#!/bin/bash
set -e
/usr/local/bin/vibe-kanban-supervisor.sh &
exec "$@"`

    b.Run(fmt.Sprintf("printf '%%s' '%s' > /usr/local/bin/bac-entrypoint.sh && chmod +x /usr/local/bin/bac-entrypoint.sh",
        entrypoint))

    // 5. Set ENTRYPOINT so the supervisor starts before sshd
    b.Entrypoint("/usr/local/bin/bac-entrypoint.sh")
}

// CredentialStorePath returns empty - no credentials to persist.
// Satisfies: VK-4
func (a *vibeKanbanAgent) CredentialStorePath() string {
    return ""
}

// ContainerMountPath returns empty - no bind-mount needed.
// Satisfies: VK-4
func (a *vibeKanbanAgent) ContainerMountPath(homeDir string) string {
    return ""
}

// HasCredentials always returns true - nothing to check.
// Satisfies: VK-4
func (a *vibeKanbanAgent) HasCredentials(storePath string) (bool, error) {
    return true, nil
}

// HealthCheck verifies that:
// 1. The vibe-kanban binary is present (vibe-kanban --version exits 0)
// 2. The vibe-kanban process is running (pgrep with retries)
// Satisfies: VK-5
func (a *vibeKanbanAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string) error {
    // Check 1: Binary presence
    exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"vibe-kanban", "--version"})
    if err != nil {
        return fmt.Errorf("vibe-kanban health check failed (binary): %w", err)
    }
    if exitCode != 0 {
        return fmt.Errorf("vibe-kanban health check failed: 'vibe-kanban --version' exited with code %d", exitCode)
    }

    // Check 2: Process running (with retries)
    const maxRetries = 5
    const retryInterval = 2 * time.Second

    for attempt := 1; attempt <= maxRetries; attempt++ {
        exitCode, err = docker.ExecInContainer(ctx, c, containerID, []string{"pgrep", "-f", "vibe-kanban"})
        if err != nil {
            return fmt.Errorf("vibe-kanban health check failed (process check): %w", err)
        }
        if exitCode == 0 {
            return nil // Process is running
        }
        if attempt < maxRetries {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-time.After(retryInterval):
            }
        }
    }

    return fmt.Errorf("vibe-kanban health check failed: process not running after %d attempts", maxRetries)
}
```

---

## Core Changes Required

### 1. `internal/constants/constants.go`

Add the constant and update `DefaultAgents`:

```go
// VibeKanbanAgentName is the stable Agent_ID for the Vibe Kanban agent module.
// Corresponds to the Agent_ID glossary term for Vibe Kanban (VK-1).
VibeKanbanAgentName = "vibe-kanban"

// Update DefaultAgents:
DefaultAgents = ClaudeCodeAgentName + "," + AugmentCodeAgentName + "," + BuildResourcesAgentName + "," + VibeKanbanAgentName
```

### 2. `internal/docker/builder.go`

Add the `Entrypoint()` method:

```go
// Entrypoint appends an ENTRYPOINT instruction in exec form.
// Used by agent modules that need to run initialization before the main CMD.
func (b *DockerfileBuilder) Entrypoint(args ...string) {
    quoted := make([]string, len(args))
    for i, a := range args {
        quoted[i] = fmt.Sprintf("%q", a)
    }
    b.lines = append(b.lines, fmt.Sprintf("ENTRYPOINT [%s]", strings.Join(quoted, ", ")))
}
```

### 3. `internal/docker/runner.go`

Add a helper to execute a command and capture stdout:

```go
// ExecInContainerWithOutput runs a command inside a running container and
// returns the exit code and stdout content.
func ExecInContainerWithOutput(ctx context.Context, c *Client, containerID string, cmd []string) (int, string, error) {
    execID, err := c.ContainerExecCreate(ctx, containerID, container.ExecOptions{
        Cmd:          cmd,
        AttachStdout: true,
        AttachStderr: true,
    })
    if err != nil {
        return -1, "", fmt.Errorf("creating exec: %w", err)
    }

    resp, err := c.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
    if err != nil {
        return -1, "", fmt.Errorf("attaching to exec: %w", err)
    }
    defer resp.Close()

    var stdout, stderr bytes.Buffer
    _, _ = stdcopy.StdCopy(&stdout, &stderr, resp.Reader)

    inspect, err := c.ContainerExecInspect(ctx, execID.ID)
    if err != nil {
        return -1, "", fmt.Errorf("inspecting exec: %w", err)
    }

    return inspect.ExitCode, stdout.String(), nil
}
```

### 4. `internal/cmd/root.go`

#### SessionSummary struct extension:

```go
type SessionSummary struct {
    DataDir       string
    ProjectDir    string
    SSHPort       int
    SSHConnect    string
    EnabledAgents []string
    VibeKanbanURL string // empty if not discovered or not enabled
}
```

#### FormatSessionSummary update:

```go
func FormatSessionSummary(s SessionSummary) string {
    var sb strings.Builder
    fmt.Fprintf(&sb, "Data directory:  %s\n", s.DataDir)
    fmt.Fprintf(&sb, "Project directory: %s\n", s.ProjectDir)
    fmt.Fprintf(&sb, "SSH port:        %d\n", s.SSHPort)
    fmt.Fprintf(&sb, "SSH connect:     %s\n", s.SSHConnect)
    fmt.Fprintf(&sb, "Enabled agents:  %s\n", strings.Join(s.EnabledAgents, ", "))
    if s.VibeKanbanURL != "" {
        fmt.Fprintf(&sb, "Vibe Kanban:     %s\n", s.VibeKanbanURL)
    }
    return sb.String()
}
```

#### Port discovery in `runStart()`:

After the health check passes and before printing the session summary, `runStart()` checks if `vibe-kanban` is among the enabled agents. If so, it attempts port discovery:

```go
// Discover Vibe Kanban port if the agent is enabled
var vibeKanbanURL string
for _, a := range enabledAgents {
    if a.ID() == constants.VibeKanbanAgentName {
        port, err := discoverVibeKanbanPort(ctx, c, containerName)
        if err != nil {
            fmt.Fprintf(os.Stderr, "warning: could not discover Vibe Kanban port: %v\n", err)
        } else if port > 0 {
            vibeKanbanURL = fmt.Sprintf("http://localhost:%d", port)
        }
        break
    }
}
```

The `discoverVibeKanbanPort` function:

```go
// discoverVibeKanbanPort attempts to find the port Vibe Kanban is listening on
// by executing ss inside the container. Retries for up to 30 seconds.
func discoverVibeKanbanPort(ctx context.Context, c *dockerpkg.Client, containerID string) (int, error) {
    deadline := time.Now().Add(30 * time.Second)
    for time.Now().Before(deadline) {
        exitCode, output, err := dockerpkg.ExecInContainerWithOutput(ctx, c, containerID,
            []string{"bash", "-c", "ss -tlnp 2>/dev/null | grep vibe-kanban | awk '{print $4}' | grep -oP ':\\K[0-9]+' | head -1"})
        if err != nil {
            return 0, err
        }
        if exitCode == 0 && output != "" {
            port := 0
            fmt.Sscanf(strings.TrimSpace(output), "%d", &port)
            if port > 0 {
                return port, nil
            }
        }
        select {
        case <-ctx.Done():
            return 0, ctx.Err()
        case <-time.After(2 * time.Second):
        }
    }
    return 0, fmt.Errorf("timed out after 30s waiting for vibe-kanban to bind a port")
}
```

### 5. `main.go`

Add the blank import:

```go
import (
    "github.com/koudis/bootstrap-ai-coding/internal/cmd"

    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/buildresources"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/vibekanban"
)
```

---

## Design Decisions

### 1. ENTRYPOINT wrapper (not supervisord, not cron, not systemd)

**Why:** Docker containers with a fixed CMD have limited options for running background services. The ENTRYPOINT + CMD pattern is the idiomatic Docker solution:
- ENTRYPOINT runs initialization (starts background services)
- CMD provides the main process arguments
- `exec "$@"` in the entrypoint ensures sshd becomes PID 1 and receives signals correctly

**Rejected alternatives:**
- **supervisord**: Heavy dependency (Python-based), overkill for one background process, adds image size
- **systemd**: Not available in Docker containers (no systemd as PID 1)
- **cron @reboot**: Requires crond running, which isn't started by sshd
- **/etc/profile.d/**: Only runs on SSH login, not on container start (violates VK-3.1)
- **Custom CMD**: Violates the constraint that agents cannot modify CMD

### 2. Shell-based supervisor (not a Go binary)

**Why:** The supervisor is a simple bash script installed via Dockerfile RUN steps. This avoids:
- Compiling and copying a separate binary into the image
- Adding complexity to the build process
- The script is ~20 lines and trivially auditable

### 3. `su -c` for user switching (not sudo, not USER directive)

**Why:** The entrypoint runs as root (Docker default). The supervisor uses `su -c "vibe-kanban" "$USERNAME"` to drop privileges. This is simpler than sudo (no sudoers parsing) and works reliably in the container environment.

### 4. Port discovery via `ss -tlnp` (not reading a config file)

**Why:** Vibe Kanban auto-assigns its port at startup. There's no config file to read. The `ss` command (part of iproute2, installed in Ubuntu by default) shows which port the process is actually listening on. This is the most reliable method since it reflects runtime state.

### 5. 30-second timeout for port discovery

**Why:** Vibe Kanban needs time to start up (Node.js initialization, port binding). 30 seconds is generous but bounded. If it fails, the CLI prints a warning but does NOT fail the overall startup - the container is still usable for SSH and other agents.

### 6. Graceful degradation for port discovery failure

**Why (VK-8.4):** If port discovery times out, the session summary simply omits the Vibe Kanban URL line. The user can still SSH into the container and discover the port manually. This prevents a flaky network or slow startup from blocking the entire workflow.

### 7. `--host 0.0.0.0` flag for vibe-kanban

**Why:** By default, Node.js servers may bind to `127.0.0.1` inside the container. With host network mode, this is fine (the host IS the container's network namespace). But to be safe and explicit, we pass `--host 0.0.0.0` so the server accepts connections on all interfaces.

### 8. Core changes are minimal and generic

The core changes (SessionSummary field, FormatSessionSummary conditional line, port discovery) reference `constants.VibeKanbanAgentName` - they don't import the agent package. The `discoverVibeKanbanPort` function lives in `cmd/root.go` and uses only `docker.ExecInContainerWithOutput`. This maintains the "no core coupling" principle at the package level while allowing the session summary to show the URL.

**Note on VK-6 (No Core Coupling):** VK-6.1 states the module SHALL NOT be referenced by name in core code. However, VK-8.3 requires the session summary to include the Vibe Kanban URL. These requirements are in tension. The resolution: `cmd/root.go` references `constants.VibeKanbanAgentName` (a constant, not a string literal or import path) to check if the agent is enabled. This is the same pattern used for `DefaultAgents`. The constant lives in `internal/constants/` which is shared infrastructure, not agent-specific code.

---

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Node.js already installed by another agent | Skip Node.js installation (check `b.IsNodeInstalled()`) |
| `npm install -g vibe-kanban` fails | Image build fails (standard Docker behavior) |
| Entrypoint script fails to start supervisor | sshd still starts (supervisor failure is non-fatal to exec "$@") |
| Vibe Kanban crashes | Supervisor restarts it (up to 5 times in 60s) |
| Supervisor gives up after max restarts | Logs error to stderr, exits; container continues running (sshd is PID 1) |
| Health check: binary not found | Returns error identifying "binary" check |
| Health check: process not running after 5 retries | Returns error identifying "process" check with retry count |
| Port discovery times out (30s) | Warning printed, URL omitted from summary, startup succeeds |
| Port discovery exec fails | Warning printed, URL omitted from summary, startup succeeds |
| `--host-network-off` (bridge mode) | Port not accessible from host; URL still shown but with a note that it requires port forwarding |

---

## Testing Strategy

### Unit Tests (example-based)

| Test | What it verifies |
|------|-----------------|
| `TestID` | Returns `constants.VibeKanbanAgentName` |
| `TestInstallNodeAlreadyInstalled` | Skips Node.js when `IsNodeInstalled()` is true |
| `TestInstallNodeNotInstalled` | Installs Node.js when `IsNodeInstalled()` is false |
| `TestInstallContainsNpmPackage` | Output contains `npm install -g vibe-kanban` |
| `TestInstallContainsEntrypoint` | Output contains ENTRYPOINT instruction |
| `TestInstallContainsSupervisor` | Output contains supervisor script with crash recovery params |
| `TestInstallDoesNotContainCMD` | Output does NOT contain CMD instruction |
| `TestInstallNoRustNoPnpm` | Output does NOT contain rust/pnpm references |
| `TestCredentialStorePath` | Returns empty string |
| `TestContainerMountPath` | Returns empty string for various homeDir values |
| `TestHasCredentials` | Returns (true, nil) |
| `TestHealthCheckBinaryFailure` | Error message identifies binary check |
| `TestHealthCheckProcessFailure` | Error message identifies process check |
| `TestFormatSessionSummaryWithVibeKanban` | URL line present when VibeKanbanURL is set |
| `TestFormatSessionSummaryWithoutVibeKanban` | URL line absent when VibeKanbanURL is empty |

### Property-Based Tests

Property tests use `pgregory.net/rapid` with minimum 100 iterations.

See Correctness Properties section below.

### Integration Tests

| Test | What it verifies |
|------|-----------------|
| `TestVibeKanbanInstallsAndRuns` | Full image build, binary present, process running |
| `TestVibeKanbanHealthCheck` | Health check passes on a live container |
| `TestVibeKanbanPortDiscovery` | Port is discoverable via ss after startup |
| `TestVibeKanbanCrashRecovery` | Process restarts after being killed |
| `TestVibeKanbanAccessibleFromHost` | HTTP GET to localhost:port returns 2xx (host network mode) |

---

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system - essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: Node.js conditional installation invariant

*For any* DockerfileBuilder state (whether `IsNodeInstalled()` returns true or false), calling `Install()` on the Vibe Kanban agent SHALL result in the generated Dockerfile containing at most one Node.js installation block, and `IsNodeInstalled()` SHALL return true after the call.

**Validates: Requirements VK-2.1**

### Property 2: Install does not emit CMD

*For any* DockerfileBuilder state, calling `Install()` on the Vibe Kanban agent SHALL NOT append any line starting with `CMD` to the builder output. The agent only sets ENTRYPOINT, never CMD.

**Validates: Requirements VK-3.1**

### Property 3: No-credential-store invariant

*For any* string value passed as `homeDir` to `ContainerMountPath()`, the return value SHALL be the empty string. *For any* string value passed as `storePath` to `HasCredentials()`, the return value SHALL be `(true, nil)`.

**Validates: Requirements VK-4.2, VK-4.3**

### Property 4: Session summary includes Vibe Kanban URL for any valid port

*For any* valid TCP port number (1-65535) set as `VibeKanbanURL` in the format `http://localhost:<port>`, `FormatSessionSummary()` SHALL include a line containing that URL. When `VibeKanbanURL` is empty, the output SHALL NOT contain "Vibe Kanban:".

**Validates: Requirements VK-8.3**

### Property 5: Supervisor script contains correct backoff parameters

*For any* username string (non-empty, valid Linux username characters), the supervisor script generated by `Install()` SHALL contain the constants `MAX_RESTARTS=5`, `WINDOW_SECONDS=60`, and `DELAY_SECONDS=5`, ensuring the crash recovery backoff is correctly configured regardless of the container user.

**Validates: Requirements VK-3.5**

---

## Dockerfile Layer Order (with Vibe Kanban)

When all default agents are enabled, the Vibe Kanban layers appear in the base image:

**Base_Image (`bac-base:latest`):**
```
FROM ubuntu:26.04
RUN apt-get install openssh-server sudo          <- base
RUN useradd <username>                           <- stable per user
RUN sudoers                                      <- stable
RUN dbus-x11 gnome-keyring libsecret-1-0         <- keyring (CC-7)
RUN /etc/profile.d/dbus-keyring.sh               <- keyring startup
RUN gitconfig                                    <- git config (Req 24)
RUN curl ca-certificates git + nodejs            <- Claude/Augment shared deps
RUN npm install -g @anthropic-ai/claude-code     <- Claude Code
RUN npm install -g @augmentcode/auggie           <- Augment Code
RUN python3 cmake build-essential default-jdk    <- Build Resources (system)
RUN go tarball + /etc/profile.d/golang.sh        <- Build Resources (Go)
RUN uv install                                   <- Build Resources (uv)
RUN npm install -g vibe-kanban                   <- Vibe Kanban (binary)
RUN printf supervisor script                     <- Vibe Kanban (supervisor)
RUN printf entrypoint script                     <- Vibe Kanban (entrypoint)
ENTRYPOINT ["/usr/local/bin/bac-entrypoint.sh"]  <- Vibe Kanban (auto-start)
RUN echo manifest > /bac-manifest.json           <- manifest
```

**Instance_Image (`bac-<name>:latest`):**
```
FROM bac-base:latest
RUN SSH host key injection                       <- per-project
RUN SSH authorized_keys                          <- per-user key
RUN sshd_config hardening                        <- per-project
RUN mkdir /run/sshd                              <- stable
CMD ["/usr/sbin/sshd", "-D"]                     <- always last
```

Docker executes: `ENTRYPOINT CMD` = `/usr/local/bin/bac-entrypoint.sh /usr/sbin/sshd -D`

The entrypoint starts the supervisor in the background, then exec's sshd as PID 1.
