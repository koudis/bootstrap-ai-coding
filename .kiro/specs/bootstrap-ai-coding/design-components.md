# Core Component Designs

This document details the design of each core package and interface in the `bootstrap-ai-coding` system. These components form the stable foundation that agent modules build upon.

> **Related documents:**
> - [design.md](design.md) — Overview and document index
> - [design-architecture.md](design-architecture.md) — High-level architecture, package layout, sequence diagrams
> - [design-docker.md](design-docker.md) — Two-layer Docker image architecture
> - [design-data-models.md](design-data-models.md) — Data models, error handling, test infrastructure
> - [design-build-resources.md](design-build-resources.md) — Build Resources agent module design
> - [design-agents.md](design-agents.md) — Agent modules: contract, implementations
> - [design-properties.md](design-properties.md) — Correctness properties and testing strategy

---

## Constants Package — Single Source of Truth

`constants/constants.go` holds every value that originates from the requirements glossary. No other package may hardcode these values — they must always import and reference this package.

> **Note (Req 22):** `ContainerUser` and `ContainerUserHome` are **no longer compile-time constants**. They have been removed from this package. The container user's username and home directory are resolved at runtime from the host user's OS account via the `hostinfo` package (see below). All packages that previously referenced `constants.ContainerUser` or `constants.ContainerUserHome` now receive these values at runtime through the `*hostinfo.Info` struct.

```go
package constants

const (
    BaseContainerImage          = "ubuntu:26.04"
    // ContainerUser — REMOVED (Req 22): now a runtime value from Info.Username
    // ContainerUserHome — REMOVED (Req 22): now a runtime value from Info.HomeDir
    WorkspaceMountPath          = "/workspace"
    SSHPortStart                = 2222
    ToolDataDirRoot             = "~/.config/bootstrap-ai-coding"
    ContainerNamePrefix         = "bac-"
    ContainerNameParentSep      = "_"   // separator between <parentdir> and <dirname>
    ContainerNameCounterSep     = "-"   // separator before the numeric counter suffix
    ManifestFilePath            = "/bac-manifest.json"
    ClaudeCodeAgentName          = "claude-code"
    AugmentCodeAgentName         = "augment-code"
    BuildResourcesAgentName      = "build-resources"
    DefaultAgents               = ClaudeCodeAgentName + "," + AugmentCodeAgentName + "," + BuildResourcesAgentName
    SSHHostKeyType              = "ed25519"
    MinDockerVersion            = "20.10"
    ContainerSSHPort            = 22
    ToolDataDirPerm             = 0o700
    ToolDataFilePerm            = 0o600
    SSHDirPerm                  = 0o700
    KnownHostsFile              = "~/.ssh/known_hosts"
    SSHConfigFile               = "~/.ssh/config"
    ImageBuildTimeout           = 8 * time.Minute  // Image_Build_Timeout glossary term
    GitConfigPerm               = 0o444            // Host_Git_Config permissions inside container (Req 24)
    DefaultRestartPolicy        = "unless-stopped"  // Restart_Policy default (Req 25)
)
```

**Validates: All glossary-derived values across Req 1–21, CC-1–CC-6**

---

## HostInfo Package — Runtime Container User Identity (Req 22)

New package `internal/hostinfo` resolves the host user's identity at runtime. This replaces the former compile-time constants `ContainerUser` and `ContainerUserHome`. The struct is named `Info` and is passed as a single value to all components that need it (DockerfileBuilder, agent modules, SSH config, etc.).

```go
// Package hostinfo resolves the host user's identity at CLI startup.
package hostinfo

import (
    "fmt"
    "os/user"
    "strconv"
)

// Info holds the runtime-resolved host user identity.
// These values determine the Container_User username and home directory.
type Info struct {
    Username string // host username (e.g. "alice")
    HomeDir  string // host home directory (e.g. "/home/alice")
    UID      int    // host effective UID
    GID      int    // host effective GID
}

// Current returns the host user's identity. Called once at CLI startup.
// Returns an error if the OS user cannot be determined.
func Current() (*Info, error) {
    u, err := user.Current()
    if err != nil {
        return nil, fmt.Errorf("resolving host user: %w", err)
    }
    uid, _ := strconv.Atoi(u.Uid)
    gid, _ := strconv.Atoi(u.Gid)
    return &Info{
        Username: u.Username,
        HomeDir:  u.HomeDir,
        UID:      uid,
        GID:      gid,
    }, nil
}
```

**Design decisions:**

- **Single resolution point:** `hostinfo.Current()` is called once in `cmd/root.go` at the very start of the `RunE` function, before flag validation (but after the root-check). The resulting `*hostinfo.Info` is threaded through to all dependent operations.
- **No global state:** The `Info` struct is passed explicitly — no package-level `var` that could be read before initialization.
- **Linux-only:** No macOS path translation. The `HomeDir` from `os/user.Current()` is used as-is (always `/home/<username>`).
- **UID/GID included:** The struct also carries UID and GID, consolidating the existing `os.Getuid()`/`os.Getgid()` calls that were scattered across `cmd/root.go`.

**Validates: Req 22.1, 22.2, 22.3, 22.5, 22.6**

---

## Agent Interface — The Core API Boundary

The `Agent` interface is the **stable contract** between the core and all agent modules. It lives in `agent/agent.go`. The core never imports any `agents/*` package directly.

**Req 22 change:** `ContainerMountPath()` now accepts the container user's home directory as a parameter, since it is no longer available as a compile-time constant. This allows agent modules to construct their mount paths using the runtime-resolved home directory from `hostinfo.Info.HomeDir`.

```go
package agent

import (
    "context"
    "github.com/koudis/bootstrap-ai-coding/internal/docker"
)

type Agent interface {
    ID() string
    Install(b *docker.DockerfileBuilder)
    CredentialStorePath() string
    ContainerMountPath(homeDir string) string  // Req 22: homeDir from info.HomeDir
    HasCredentials(storePath string) (bool, error)
    HealthCheck(ctx context.Context, c *docker.Client, containerID string) error
}
```

**Validates: Req 7.1, Req 22.4**

## AgentRegistry

The registry is a package-level map in `agent/registry.go`. Agent modules self-register in their `init()` functions.

```go
func Register(a Agent)                  // panics on duplicate ID
func Lookup(id string) (Agent, error)   // descriptive error listing known IDs when not found
func All() []Agent
func KnownIDs() []string                // sorted alphabetically
```

Agent modules are wired into the binary exclusively via blank imports in `main.go`:

```go
import (
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
    _ "github.com/koudis/bootstrap-ai-coding/internal/agents/buildresources"
    // Add future agents here — no other file changes required
)
```

**Validates: Req 7.2**

---

## DockerfileBuilder

`docker/builder.go` assembles a Dockerfile incrementally. The base layer (`ubuntu:26.04` + Container_User setup + sshd + SSH host key injection) is always present. Each enabled agent appends its own `RUN` steps via `Install()`. A manifest `COPY` step is added last.

The builder supports two **user strategies** (Req 10, 10a):
- `UserStrategyCreate` — no UID/GID conflict; creates the Container_User with `useradd`
- `UserStrategyRename` — a Conflicting_Image_User exists; renames it with `usermod -l` instead

**Req 22 change:** The constructor now accepts a `*hostinfo.Info` struct (runtime-resolved from the host user's OS account) instead of separate `uid, gid int` parameters or compile-time constants. All Dockerfile instructions that reference the container user or home directory use the fields from this struct. Callers pass the single `*hostinfo.Info` value rather than individual arguments.

```go
type UserStrategy int

const (
    UserStrategyCreate UserStrategy = iota
    UserStrategyRename
)

// NewDockerfileBuilder creates a builder for the container Dockerfile.
// info carries the runtime-resolved Container_User identity (Req 22).
func NewDockerfileBuilder(info *hostinfo.Info,
    publicKey, hostKeyPriv, hostKeyPub string,
    strategy UserStrategy, conflictingUser string) *DockerfileBuilder

func (b *DockerfileBuilder) From(image string)
func (b *DockerfileBuilder) Run(cmd string)
func (b *DockerfileBuilder) Env(k, v string)
func (b *DockerfileBuilder) Copy(src, dst string)
func (b *DockerfileBuilder) Cmd(cmd string)
func (b *DockerfileBuilder) Finalize()        // appends CMD — must be called last, after all agent Install() steps
func (b *DockerfileBuilder) Build() string
func (b *DockerfileBuilder) Lines() []string
// Username returns the container username from the *hostinfo.Info this builder was configured with (Req 22).
func (b *DockerfileBuilder) Username() string
// HomeDir returns the container user home directory from the *hostinfo.Info this builder was configured with (Req 22).
func (b *DockerfileBuilder) HomeDir() string
```

**Generated Dockerfile user creation example** (values from `*hostinfo.Info`):
```
RUN useradd --create-home --home-dir /home/alice --uid 1000 --gid 1000 --shell /bin/bash alice
```
(Where `alice`, `/home/alice`, `1000`, `1000` are example values from `info.Username`, `info.HomeDir`, `info.UID`, `info.GID`.)

**Dockerfile instruction order (Req 21):** `NewDockerfileBuilder` seeds the base layers (FROM, openssh-server, Container_User, sudo, SSH keys, sshd_config, /run/sshd) but does **not** append `CMD`. The caller appends agent steps via `Install()`, then the manifest `RUN`, then calls `Finalize()` to append `CMD` as the final instruction. This ensures all `RUN` layers are ordered before `CMD`, keeping them in Docker's layer cache across rebuilds.

> **Note:** With the two-layer architecture (see [design-docker.md](design-docker.md)), this monolithic Dockerfile is split into a Base_Image (everything up to and including the manifest) and an Instance_Image (SSH keys, authorized_keys, sshd hardening, CMD). See that document for the updated layer split and builder API.

## Headless Keyring (D-Bus + gnome-keyring-daemon)

The container runs a headless `gnome-keyring-daemon` so that tools using `libsecret` / D-Bus Secret Service API (Claude Code, VS Code extensions) can store and retrieve OAuth tokens without a graphical desktop.

**Installed in the base layer** (inside `NewDockerfileBuilder`), not in individual agent modules, because multiple agents and IDE extensions benefit from it.

**Packages installed:**
- `dbus-x11` — provides `dbus-launch` for starting a session bus
- `gnome-keyring` — Secret Service provider
- `libsecret-1-0` — client library (used by Node.js `keytar` / `libsecret` bindings)

**Startup mechanism:**
A shell profile script (`/etc/profile.d/dbus-keyring.sh`) is installed that:
1. Starts a D-Bus session bus via `dbus-launch` (if not already running)
2. Exports `DBUS_SESSION_BUS_ADDRESS`
3. Unlocks `gnome-keyring-daemon` with an empty password via stdin pipe

```sh
#!/bin/sh
# /etc/profile.d/dbus-keyring.sh — start D-Bus + gnome-keyring for headless SSH sessions
if [ -z "$DBUS_SESSION_BUS_ADDRESS" ]; then
    eval $(dbus-launch --sh-syntax)
    export DBUS_SESSION_BUS_ADDRESS
fi
# Unlock the default keyring with an empty password
echo "" | gnome-keyring-daemon --unlock --components=secrets 2>/dev/null
```

This script runs on every SSH login (interactive shells source `/etc/profile.d/*.sh`). The keyring is per-session and uses an empty password, which is acceptable because the container is single-user and access is already gated by SSH key authentication.

**Validates: CC-7**

---

## Git Configuration Forwarding (Req 24)

The `DockerfileBuilder` injects the host user's `~/.gitconfig` into the container image at build time, following the same pattern as SSH host key injection (step 6 in the constructor). The git config content is read by the caller (`cmd/root.go`) and passed to the builder as an optional string parameter.

**Constructor change:**

```go
// NewDockerfileBuilder gains an additional parameter:
func NewDockerfileBuilder(info *hostinfo.Info, publicKey, hostKeyPriv, hostKeyPub string,
    strategy UserStrategy, conflictingUser string, gitConfig string) *DockerfileBuilder
```

The `gitConfig` parameter contains the full text content of `~/.gitconfig`. If the file does not exist on the host, the caller passes an empty string and the builder skips the injection step entirely (no Dockerfile instruction emitted).

**Caller logic in `cmd/root.go`:**

```go
// Read git config — silent skip if absent
gitConfigPath := filepath.Join(info.HomeDir, ".gitconfig")
gitConfigContent, err := os.ReadFile(gitConfigPath)
if err != nil {
    gitConfigContent = nil // file absent or unreadable — skip silently
}

b := dockerpkg.NewDockerfileBuilder(info, publicKey, hostKeyPriv, hostKeyPub,
    strategy, conflictingUser, string(gitConfigContent))
```

**Generated Dockerfile step** (only emitted when `gitConfig != ""`):

```dockerfile
RUN echo <base64-encoded-content> | base64 -d > /home/alice/.gitconfig && \
    chown alice:alice /home/alice/.gitconfig && \
    chmod 0444 /home/alice/.gitconfig
```

**Injection placement in the constructor:** After the keyring setup (step 10) and before the `// NOTE: CMD is intentionally NOT set here` comment. This places it in the stable base layer — the git config rarely changes, so it benefits from Docker layer caching.

**Design decisions:**

- **Content injection, not bind-mount:** The file is baked into the image (like SSH host keys) rather than bind-mounted at runtime. This ensures the config is available even if the host file is later deleted, and avoids adding another mount to the container spec.
- **Base64 encoding over `COPY` or raw `printf`:** Using `COPY` would require the git config to exist as a file in the Docker build context (a tar archive), which would mean the builder can no longer produce a self-contained Dockerfile string — it would need to manage build context files too. Base64 avoids all shell escaping issues (quotes, newlines, backslashes, dollar signs, backticks) that raw `printf` or `echo` would face with arbitrary git config content. This is the same pattern used for SSH host key injection.
- **Read-only (`0444`):** The container user cannot modify the injected config. If they need local overrides, they can use `git config --local` or `GIT_CONFIG_GLOBAL` env var. This prevents accidental writes that would be lost on rebuild.
- **Silent skip:** If `~/.gitconfig` is absent, no error or warning is produced — many developers may not have a global git config (they use per-repo `.git/config` instead).
- **Re-read on `--rebuild`:** Since `--rebuild` forces `NoCache`, the `os.ReadFile` in `cmd/root.go` always reads the current file content. No special logic is needed — the standard rebuild path handles this automatically.

**Validates: Req 24.1, 24.2, 24.3, 24.4, 24.5**

---

## Container Restart Policy (Req 25)

The CLI applies a Docker restart policy to every container it creates, ensuring containers survive host reboots by default.

**Flag definition in `cmd/root.go`:**

```go
rootCmd.Flags().String("docker-restart-policy", constants.DefaultRestartPolicy,
    "Docker restart policy: no, always, unless-stopped, on-failure")
```

**Validation in `cmd/root.go`** (during flag parsing, before any Docker operations):

```go
var validRestartPolicies = map[string]bool{
    "no":             true,
    "always":         true,
    "unless-stopped": true,
    "on-failure":     true,
}

func validateRestartPolicy(policy string) error {
    if !validRestartPolicies[policy] {
        return fmt.Errorf("invalid --docker-restart-policy %q: must be one of: no, always, unless-stopped, on-failure", policy)
    }
    return nil
}
```

**Application in `docker/runner.go`** (`CreateContainer`):

The `ContainerSpec.RestartPolicy` field is mapped to the Docker SDK's `container.RestartPolicy` struct in `HostConfig`:

```go
import "github.com/docker/docker/api/types/container"

hostConfig := &container.HostConfig{
    // ... existing port bindings, mounts, etc.
    RestartPolicy: container.RestartPolicy{
        Name: container.RestartPolicyMode(spec.RestartPolicy),
    },
}
```

**Threading from CLI to runner:**

1. `cmd/root.go` reads `--docker-restart-policy` flag value (default: `constants.DefaultRestartPolicy`)
2. Validates it against the allowed set
3. Stores it in `Config.RestartPolicy`
4. Passes it to `ContainerSpec.RestartPolicy` when constructing the spec
5. `docker/runner.go` applies it in `CreateContainer`

**Behaviour with `--stop-and-remove`:**

When `--stop-and-remove` is used, the container is stopped via `docker stop` (which sends SIGTERM) and then removed. A container with `unless-stopped` policy that was explicitly stopped will NOT restart on reboot — Docker tracks the "stopped by user" state. Removal deletes the container entirely, so there is nothing to restart.

**Behaviour with existing containers:**

When the CLI reconnects to an already-running container (Req 5.2), it does NOT modify the container's restart policy. The policy is immutable after creation — this is a Docker limitation. If the user wants a different policy, they must `--stop-and-remove` and re-create.

**Design decisions:**

- **`unless-stopped` as default:** This is the most practical choice for development containers. They come back after a reboot (no manual intervention), but stay stopped if the user explicitly stopped them. The `always` policy would restart containers the user intentionally stopped, which is surprising.
- **No persistence in Tool_Data_Dir:** The restart policy is not persisted — it's applied at container creation time and Docker remembers it. There's no need to store it separately.
- **START-only flag:** The policy only makes sense when creating a container. It's meaningless for `--stop-and-remove` (which removes the container) and `--purge` (which removes everything).

**Validates: Req 25.1–25.10, CLI-7**

---

## Base Image User Inspection

`docker/client.go` exposes a helper to detect UID/GID conflicts in the base image before building (Req 10a):

```go
type ImageUser struct {
    Username string
    UID      int
    GID      int
}

// FindConflictingUser runs docker run --rm on the base image, parses /etc/passwd,
// and returns the first user whose UID or GID matches. Returns (nil, nil) if no conflict.
func FindConflictingUser(ctx context.Context, client *Client, uid, gid int) (*ImageUser, error)
```

**Validates: Req 9.1–9.3, Req 10.1–10.5, Req 10a.4, Req 13.2**

---

## Docker Image Build — Verbose Mode

`docker/runner.go` exposes `BuildImage` and `BuildImageWithTimeout`. Both accept a `verbose bool` parameter that controls how the Docker daemon's build response stream is handled.

The Docker SDK's `client.ImageBuild` returns an `io.ReadCloser` whose body is a sequence of newline-delimited JSON objects, each with a `stream` field (progress text) and optionally an `error` field.

**Silent mode (`verbose == false`, default):**
The stream is drained in a background goroutine. Each decoded `stream` value is accumulated in a `strings.Builder` for error reporting only. No output is written to stdout. The "Building image..." message (Req 14.5) is the only visible indication that a build is in progress.

**Verbose mode (`verbose == true`):**
Each decoded `stream` value is written to `os.Stdout` immediately as it arrives, producing real-time layer-by-layer progress and `RUN` step output. Error detection and timeout handling are identical to silent mode.

```go
// BuildImage builds a Docker image from the spec's Dockerfile.
// When verbose is true, build output is streamed to os.Stdout in real time.
func BuildImage(ctx context.Context, c *Client, spec ContainerSpec, verbose bool) (string, error)

// BuildImageWithTimeout is the underlying implementation used by BuildImage.
func BuildImageWithTimeout(ctx context.Context, c *Client, spec ContainerSpec, timeout time.Duration, verbose bool) (string, error)
```

The `verbose` flag is threaded from `Config.Verbose` → `runStart` → `BuildImage`. It is never consulted when no build is triggered (manifest matches and `--rebuild` is absent).

**Validates: Req 20.2, 20.3, 20.4, 20.6**

---

## Naming Package

`naming/naming.go` derives a human-readable, collision-resistant container name from the absolute project path. The algorithm follows Req 5.1:

1. Extract the directory name (last path component) and parent directory name (second-to-last). If at the filesystem root, use `"root"` as the parent.
2. Sanitize each component: lowercase; replace chars outside `[a-z0-9.-]` with `-`; collapse consecutive `-`; trim leading/trailing `-`. The `_` character is reserved as the separator and is excluded from the allowed set.
3. Try candidates in order, checking only against existing `bac-`-prefixed containers supplied by the caller:
   - `bac-<dirname>`
   - `bac-<parentdir>_<dirname>`
   - `bac-<parentdir>_<dirname>-2`, `-3`, … (incrementing until free)
4. Return the first free candidate.

```go
// ContainerName returns the first candidate name not present in existingNames.
// existingNames should contain only bac-prefixed container names already on the host.
func ContainerName(projectPath string, existingNames []string) (string, error)

// SanitizeNameComponent lowercases s and replaces any char outside [a-z0-9.-] with '-',
// collapses consecutive '-', and trims leading/trailing '-'.
func SanitizeNameComponent(s string) string
```

**Validates: Req 5.1**

---

## SSH Key Discovery

`ssh/keys.go` implements public key resolution: `--ssh-key` flag > `~/.ssh/id_ed25519.pub` > `~/.ssh/id_rsa.pub`.

```go
func DiscoverPublicKey(sshKeyFlag string) (string, error)
func GenerateHostKeyPair() (priv, pub string, err error)
```

**Validates: Req 4.1, 4.4**

---

## SSH known_hosts Management

`ssh/known_hosts.go` keeps `~/.ssh/known_hosts` in sync with the container's SSH host key (Req 18). Called after the container is confirmed ready and after `--stop-and-remove` / `--purge`.

```go
// SyncKnownHosts ensures correct entries for the given port and host public key.
// If noUpdate is true, prints a notice and returns without touching the file.
func SyncKnownHosts(port int, hostPubKey string, noUpdate bool) error

// RemoveKnownHostsEntries removes all lines matching the given port. No-op if file absent.
func RemoveKnownHostsEntries(port int) error
```

Both functions guarantee they never modify lines that do not match the target port patterns.

**Validates: Req 18.1–18.9**

---

## SSH Config Management

`ssh/ssh_config.go` maintains a `Host` stanza in `~/.ssh/config` for each container (Req 19). The entry lets the user connect with `ssh bac-<dirname>` without specifying port, user, or hostname.

**Why `IdentityFile` is omitted:** The container already has the user's public key in `authorized_keys` (Req 4), and the host key is kept consistent in `known_hosts` (Req 18). SSH authenticates and verifies correctly without an explicit key path in the config entry.

```go
type SSHConfigEntry struct {
    Host     string // e.g. "bac-my-project" or "bac-path_my-project"
    HostName string // always "localhost"
    Port     int    // SSH_Port
    User     string // from info.Username (Req 22, via *hostinfo.Info)
    // StrictHostKeyChecking: always "yes" — host key kept consistent by Req 18
    // IdentityFile: intentionally omitted — public key in authorized_keys (Req 4)
}

// SyncSSHConfig ensures a correct entry exists for containerName and port.
// The user field comes from info.Username (Req 22, via *hostinfo.Info).
// If noUpdate is true, prints a notice and returns without touching the file.
// Appends if absent; no-op if matching; replaces and prints confirmation if stale.
// Never modifies entries whose Host does not match containerName.
func SyncSSHConfig(containerName string, port int, user string, noUpdate bool) error

// RemoveSSHConfigEntry removes the Host stanza for containerName. No-op if absent.
func RemoveSSHConfigEntry(containerName string) error

// RemoveAllBACSSHConfigEntries removes all stanzas whose Host starts with
// constants.ContainerNamePrefix. Called by --purge. No-op if file absent.
func RemoveAllBACSSHConfigEntries() error
```

**Parsing strategy:** `~/.ssh/config` is read line-by-line. A stanza begins at a `Host <name>` line and ends at the next `Host` line or EOF. The tool identifies its own stanzas by matching the `Host` value against `constants.ContainerNamePrefix`. All other stanzas are preserved verbatim.

**Validates: Req 19.1–19.9**

---

## Credentials (merged into DataDir — Req 28)

> **Removed as a standalone package.** The two functions (`Resolve` and `EnsureDir`) now live in `datadir/credentials.go`. The API is unchanged — only the import path changes from `credentials.Resolve` / `credentials.EnsureDir` to `datadir.ResolveCredentialPath` / `datadir.EnsureCredentialDir`.

```go
// ResolveCredentialPath returns override if non-empty, else expands ~ in agentDefault.
func ResolveCredentialPath(agentDefault, override string) string

// EnsureCredentialDir creates the directory at path if it does not already exist.
func EnsureCredentialDir(path string) error
```

**Validates: Req 8.3, 8.4, Req 28**

---

## DataDir Package

`datadir/datadir.go` manages the Tool_Data_Dir (`~/.config/bootstrap-ai-coding/<container-name>/`). Single source of truth for all persistent per-project data: SSH port, SSH host key pair, agent manifest, credential paths, and port auto-selection.

```go
// --- datadir.go (core data directory management) ---
func New(containerName string) (*DataDir, error)
func (d *DataDir) Path() string
func (d *DataDir) ReadPort() (int, error)
func (d *DataDir) WritePort(port int) error
func (d *DataDir) ReadHostKey() (priv, pub string, err error)
func (d *DataDir) WriteHostKey(priv, pub string) error
func (d *DataDir) ReadManifest() ([]string, error)
func (d *DataDir) WriteManifest(agentIDs []string) error
func PurgeRoot() error
func ListContainerNames() ([]string, error)

// --- credentials.go (merged from internal/credentials) ---
// ResolveCredentialPath returns override if non-empty, else expands ~ in agentDefault.
func ResolveCredentialPath(agentDefault, override string) string
// EnsureCredentialDir creates the directory at path if it does not already exist.
func EnsureCredentialDir(path string) error

// --- portfinder.go (merged from internal/portfinder) ---
// FindFreePort iterates from constants.SSHPortStart upward and returns the
// first TCP port on 127.0.0.1 that is not already in use.
func FindFreePort() (int, error)
// IsPortFree reports whether the given port is available for binding on 127.0.0.1.
func IsPortFree(port int) bool
```

**Validates: Req 8.3, 8.4, 12.1, 12.2, 13.1, 13.4, 15.1–15.3, Req 28**

---

## PortFinder (merged into DataDir — Req 28)

> **Removed as a standalone package.** `FindFreePort` and `IsPortFree` now live in `datadir/portfinder.go`. The API is unchanged — only the import path changes from `portfinder.FindFreePort` to `datadir.FindFreePort`.

**Validates: Req 12.1, Req 28**

---

## PathUtil Package

`internal/pathutil` provides a single shared helper with zero internal dependencies (only stdlib). All packages that need to expand `~/` paths import this instead of defining their own local helper.

```go
package pathutil

import (
    "os"
    "path/filepath"
)

// ExpandHome expands a leading "~/" to the user's home directory.
func ExpandHome(p string) string {
    if len(p) >= 2 && p[:2] == "~/" {
        home, _ := os.UserHomeDir()
        return filepath.Join(home, p[2:])
    }
    return p
}
```

Used by: `naming`, `ssh`, `datadir`, `cmd`.

**Validates: Req 22**
