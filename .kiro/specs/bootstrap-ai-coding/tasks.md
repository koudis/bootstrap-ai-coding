# Implementation Plan: bootstrap-ai-coding

## Overview

Implement the `bootstrap-ai-coding` Go CLI tool in package dependency order: pure constants first, then leaf packages, then the agent interface and registry, then Docker helpers, then the Claude Code agent module, and finally the Cobra CLI orchestration and entry point. Each task builds directly on the previous ones. Integration tests are the final task.

The module path is `github.com/koudis/bootstrap-ai-coding`. All glossary-derived values must be referenced via `constants.*` — never hardcoded.

## Tasks

- [x] 1. Initialise Go module and constants package
  - Run `go mod init github.com/koudis/bootstrap-ai-coding` and add all required dependencies (`github.com/spf13/cobra`, `github.com/docker/docker`, `golang.org/x/crypto`, `pgregory.net/rapid`, `github.com/stretchr/testify`) to `go.mod`/`go.sum`
  - Create `internal/constants/constants.go` with every glossary-derived constant: `BaseContainerImage`, `ContainerUser`, `ContainerUserHome`, `WorkspaceMountPath`, `SSHPortStart`, `ContainerSSHPort`, `ToolDataDirRoot`, `ContainerNamePrefix`, `ContainerNameHashLen`, `ManifestFilePath`, `DefaultAgent`, `SSHHostKeyType`, `MinDockerVersion`, `ToolDataDirPerm`, `ToolDataFilePerm`
  - No other package may hardcode any of these values
  - _Requirements: Req 1–17, CC-1–CC-6, CLI-1–CLI-6 (all glossary-derived values)_

- [x] 2. Implement `internal/naming` package
  - [x] 2.1 Implement `naming/naming.go`
    - `ContainerName(projectPath string, existingNames []string) (string, error)` — resolves to absolute path, extracts dirname and parentdir, sanitizes each (lowercase; replace chars outside `[a-z0-9.-]` with `-`; collapse consecutive `-`; trim leading/trailing `-`; `_` is reserved as separator and excluded), then tries candidates in order against `existingNames` (only `bac-`-prefixed names):
      - Candidate 1: `bac-<dirname>`
      - Candidate 2: `bac-<parentdir>_<dirname>` (uses `"root"` if project is at filesystem root)
      - Candidate 3+: `bac-<parentdir>_<dirname>-2`, `-3`, … until free
    - `SanitizeNameComponent(s string) string` — pure helper implementing the sanitization rule above
    - _Requirements: Req 5.1_

  - [x] 2.2 Write property tests for `naming`
    - **Property 12: Container naming produces correct, collision-resistant names**
    - Test determinism: same path + same existingNames always returns same name
    - Test prefix: every name starts with `constants.ContainerNamePrefix`
    - Test conflict advancement: with level-1 name occupied, returns a different name
    - Test sanitization: `SanitizeNameComponent` only produces chars in `[a-z0-9.-]` (no `_`)
    - **Validates: Req 5.1**

- [x] 3. Implement `internal/portfinder` package
  - [x] 3.1 Implement `portfinder/portfinder.go`
    - `FindFreePort() (int, error)` — iterates from `constants.SSHPortStart` upward, returns first free TCP port
    - `IsPortFree(port int) bool` — attempts `net.Listen` on `127.0.0.1:<port>`, returns true if successful
    - _Requirements: Req 12.1_

  - [x] 3.2 Write property tests for `portfinder`
    - **Property 20: Port finder returns the first free port at or above constants.SSHPortStart**
    - Occupy 0–5 ports starting at `constants.SSHPortStart` using real listeners, assert `FindFreePort` returns the first unoccupied port
    - Assert returned port is always `>= constants.SSHPortStart`
    - Assert `IsPortFree` returns true for the returned port
    - **Validates: Req 12.1**

- [x] 4. Implement `internal/agent` interface and registry
  - [x] 4.1 Implement `agent/agent.go`
    - Define the `Agent` interface with all six methods: `ID() string`, `Install(b *docker.DockerfileBuilder)`, `CredentialStorePath() string`, `ContainerMountPath() string`, `HasCredentials(storePath string) (bool, error)`, `HealthCheck(ctx context.Context, containerID string) error`
    - _Requirements: Req 7.1_

  - [x] 4.2 Implement `agent/registry.go`
    - Package-level `registry map[string]Agent`
    - `Register(a Agent)` — panics on duplicate ID
    - `Lookup(id string) (Agent, error)` — returns descriptive error listing known IDs when not found
    - `All() []Agent` — returns all registered agents
    - `KnownIDs() []string` — returns sorted slice of all registered IDs
    - _Requirements: Req 7.2, 7.3_

  - [x] 4.3 Write unit tests for `agent` registry
    - Test `Lookup` returns error for unknown ID and includes available IDs in the message
    - Test `Register` panics on duplicate ID
    - Test `KnownIDs` returns sorted results
    - **Validates: Req 7.2, 7.3**

  - [x] 4.4 Write property test for unknown agent IDs
    - **Property 26: Unknown agent IDs always produce errors**
    - For any string not matching a registered ID, `Lookup` returns non-nil error
    - **Validates: Req 7.3**

- [x] 5. Implement `internal/docker/builder.go` — DockerfileBuilder with UserStrategy support
  - [x] 5.1 Update `internal/docker/builder.go` for Req 10a
    - Add `UserStrategy` type with `UserStrategyCreate` and `UserStrategyRename` constants
    - Update `NewDockerfileBuilder` signature to `(uid, gid int, publicKey, hostKeyPriv, hostKeyPub string, strategy UserStrategy, conflictingUser string) *DockerfileBuilder`
    - `UserStrategyCreate`: use `groupadd` + `useradd` (existing behaviour)
    - `UserStrategyRename`: use `usermod -l` + `usermod -d -m` + `groupmod -n` to rename `conflictingUser` to `constants.ContainerUser`
    - All callers in `internal/cmd/root.go` must be updated to pass `UserStrategyCreate, ""`
    - Methods unchanged: `From`, `Run`, `Env`, `Copy`, `Cmd`, `Build() string`, `Lines() []string`
    - _Requirements: Req 10.1–10.5, Req 10a.4_

  - [x] 5.2 Implement `FindConflictingUser` in `internal/docker/client.go`
    - Add `ImageUser` struct: `Username string`, `UID int`, `GID int`
    - `FindConflictingUser(ctx context.Context, client *Client, uid, gid int) (*ImageUser, error)` — runs a short-lived `docker run --rm constants.BaseContainerImage getent passwd`, parses `/etc/passwd` format, returns first entry whose UID or GID matches; returns `(nil, nil)` if no conflict
    - _Requirements: Req 10a.1, 10a.2_

  - [x] 5.3 Wire UID/GID conflict check into `internal/cmd/root.go` start flow
    - Before building the image, call `FindConflictingUser(ctx, dockerClient, uid, gid)`
    - If conflict found: print message identifying conflicting username and UID/GID, prompt `[y/N]`
    - If user confirms: pass `UserStrategyRename, conflictingUser.Username` to `NewDockerfileBuilder`
    - If user declines: print error to stderr and return exit 1
    - If no conflict: pass `UserStrategyCreate, ""` to `NewDockerfileBuilder` (existing path)
    - _Requirements: Req 10a.3, 10a.4, 10a.5, 10a.6_

  - [x] 5.4 Write property tests for `DockerfileBuilder`
    - **Property 3: Generated Dockerfile always uses constants.BaseContainerImage as base image**
    - **Property 4: Generated Dockerfile always includes SSH server and Container_User**
    - **Property 5: Container_User UID and GID always match the host user**
    - **Property 5a: UserStrategyRename uses usermod -l, UserStrategyCreate uses useradd**
    - **Property 5b: No conflict returns nil from FindConflictingUser**
    - For any UID/GID pair not present in the parsed `/etc/passwd` output, `FindConflictingUser` returns `(nil, nil)`
    - **Property 6: Container_User always has passwordless sudo**
    - **Property 7: sshd_config always disables password authentication**
    - **Property 8: Public key is always injected into constants.ContainerUserHome/.ssh/authorized_keys**
    - **Property 10: SSH host key is always injected into the Dockerfile**
    - For each property, draw random UID/GID/key values and assert the invariant holds in `b.Build()`; test both `UserStrategyCreate` and `UserStrategyRename` paths for Properties 5 and 5a
    - **Validates: Req 3.1, 4.2, 4.5, 9.1–9.3, 10.1–10.5, 10a.2, 10a.4, 13.2**

- [x] 6. Implement `internal/docker/client.go` — Docker SDK wrapper and prerequisite checks
  - [x] 6.1 Implement `internal/docker/client.go`
    - `Client` interface (or struct wrapping `*dockerclient.Client`) exposing: `Ping`, `ServerVersion`, `ImageInspect`, `ImageBuild`, `ContainerInspect`, `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerRemove`, `ContainerList`, `ImageList`, `ImageRemove`
    - `NewClient() (*Client, error)` — creates Docker SDK client, calls `Ping`, returns descriptive error if daemon unreachable
    - `CheckVersion(c *Client) error` — parses server version, returns error if `< constants.MinDockerVersion`
    - `IsVersionCompatible(version string) bool` — pure helper for version comparison (testable without daemon)
    - _Requirements: Req 6.1, 6.2, 6.3, 6.4_

  - [x] 6.2 Write property test for Docker version comparison
    - **Property 13: Docker version comparison is correct**
    - For any `(major, minor, patch)` triple, `IsVersionCompatible` returns true iff `major > 20 || (major == 20 && minor >= 10)`
    - **Validates: Req 6.3, 6.4**

  - [x] 6.3 Write unit tests for Docker client error paths
    - Test `NewClient` returns error when daemon is unreachable (mock or stub)
    - Test `CheckVersion` returns error with detected and required versions when version is too old
    - **Validates: Req 6.2, 6.4**

- [x] 7. Implement `internal/docker/runner.go` — container lifecycle helpers
  - Implement `internal/docker/runner.go` with helpers: `BuildImage(ctx, client, spec)`, `CreateContainer(ctx, client, spec)`, `StartContainer(ctx, client, name)`, `StopContainer(ctx, client, name)`, `RemoveContainer(ctx, client, name)`, `InspectContainer(ctx, client, name)`, `WaitForSSH(ctx, host, port, timeout)` (TCP health-check polling), `ListBACContainers(ctx, client)`, `ListBACImages(ctx, client)`
  - All container names and image tags must be derived from `constants.ContainerNamePrefix`
  - `WaitForSSH` polls `host:port` with TCP dial until success or timeout (10 s)
  - _Requirements: Req 3.3, 5.2, 5.3, 5.4, 14.1, 14.5, 14.6, 16.2, 16.4_

- [x] 8. Implement `internal/ssh` package — public key discovery
  - [x] 8.1 Implement `ssh/keys.go`
    - `DiscoverPublicKey(sshKeyFlag string) (string, error)` — tries `sshKeyFlag` (if non-empty), then `~/.ssh/id_ed25519.pub`, then `~/.ssh/id_rsa.pub`; returns file contents of first found; returns descriptive error if none found
    - `GenerateHostKeyPair() (priv, pub string, err error)` — generates an ed25519 host key pair using `golang.org/x/crypto/ssh`; returns PEM-encoded private key and authorised-keys-format public key
    - _Requirements: Req 4.1, 4.4, 13.1_

  - [x] 8.2 Write property test for SSH key discovery precedence
    - **Property 9: Public key discovery respects precedence order**
    - Simulate presence/absence of each candidate file and assert `DiscoverPublicKey` returns the highest-precedence available key
    - **Validates: Req 4.1**

  - [x] 8.3 Write unit tests for SSH key error paths
    - Test `DiscoverPublicKey` returns descriptive error when no key file exists and no flag is set
    - **Validates: Req 4.4**

- [x] 9. Implement `internal/credentials` package
  - [x] 9.1 Implement `credentials/store.go`
    - `Resolve(agentDefault, override string) string` — returns `override` if non-empty, else expands `~` in `agentDefault`
    - `EnsureDir(path string) error` — `os.MkdirAll(path, constants.ToolDataDirPerm)`
    - _Requirements: Req 8.3, 8.4_

  - [x] 9.2 Write property tests for `credentials`
    - **Property 14: Credential store path resolution respects override precedence**
    - For any `(agentDefault, override)` pair, `Resolve` returns override when non-empty, expanded default when override is empty
    - **Property 15: Credential store directory is always created before mounting**
    - `EnsureDir` on a non-existent path creates the directory with `constants.ToolDataDirPerm`
    - **Validates: Req 8.3, 8.4**

- [x] 10. Implement `internal/datadir` package
  - [x] 10.1 Implement `datadir/datadir.go`
    - `DataDir` struct with `path string`
    - `New(containerName string) (*DataDir, error)` — expands `constants.ToolDataDirRoot`, joins with `containerName`, calls `os.MkdirAll` with `constants.ToolDataDirPerm`
    - `Path() string`
    - `ReadPort() (int, error)` / `WritePort(port int) error` — reads/writes `port` file with `constants.ToolDataFilePerm`
    - `ReadHostKey() (priv, pub string, err error)` / `WriteHostKey(priv, pub string) error` — reads/writes `ssh_host_ed25519_key` and `ssh_host_ed25519_key.pub` with `constants.ToolDataFilePerm`
    - `ReadManifest() ([]string, error)` / `WriteManifest(agentIDs []string) error` — JSON-encodes/decodes agent ID list, writes with `constants.ToolDataFilePerm`
    - `PurgeRoot() error` — `os.RemoveAll` on expanded `constants.ToolDataDirRoot`
    - _Requirements: Req 12.2, 13.1, 13.4, 14.2, 15.1–15.3_

  - [x] 10.2 Write property tests for `datadir`
    - **Property 11: SSH host key is stable across rebuilds**
    - Write a key pair, read it back multiple times, assert identical result each time
    - **Property 21: Persisted port round-trips correctly**
    - For any valid port number, `WritePort` then `ReadPort` returns the same value
    - **Property 24: Tool_Data_Dir is created with constants.ToolDataDirPerm permissions**
    - `New` on a non-existent path creates directory with mode `constants.ToolDataDirPerm`
    - **Validates: Req 12.2, 13.1, 13.3, 13.4, 15.2, 15.3**

  - [x] 10.3 Write unit tests for `datadir` file permissions
    - Assert all files written by `WritePort`, `WriteHostKey`, `WriteManifest` have mode `constants.ToolDataFilePerm`
    - **Validates: Req 13.4, 15.3**

- [x] 11. Checkpoint — core packages complete
  - Ensure `go build ./...` and `go test ./...` pass for all packages implemented so far (`internal/constants`, `internal/naming`, `internal/portfinder`, `internal/agent`, `internal/docker/builder.go`, `internal/docker/client.go`, `internal/ssh`, `internal/credentials`, `internal/datadir`)
  - Ensure all tests pass; ask the user if questions arise.

- [x] 12. Implement `internal/agents/claude` — Claude Code agent module
  - [x] 12.1 Implement `internal/agents/claude/claude.go`
    - Private `claudeAgent` struct implementing `agent.Agent`
    - `init()` calls `agent.Register(&claudeAgent{})`
    - `ID()` returns `constants.DefaultAgent` (`"claude-code"`)
    - `Install(b)` appends: `apt-get install curl ca-certificates git`, NodeSource LTS setup + `apt-get install nodejs`, `npm install -g @anthropic-ai/claude-code`
    - `CredentialStorePath()` returns `filepath.Join(os.UserHomeDir(), ".claude")`
    - `ContainerMountPath()` returns `filepath.Join(constants.ContainerUserHome, ".claude")`
    - `HasCredentials(storePath)` checks for `.credentials.json` in `storePath`
    - `HealthCheck(ctx, containerID)` runs `claude --version` via Docker exec
    - Must NOT import `cmd`, `naming`, `ssh`, `credentials`, `datadir`, `portfinder`, or `docker/runner`
    - _Requirements: CC-1, CC-2, CC-3, CC-4, CC-5, CC-6_

  - [x] 12.2 Write property tests for Claude Code agent
    - **Property 27: All registered agents satisfy the Agent interface**
    - After blank-importing `internal/agents/claude`, assert `agent.All()` returns at least one agent and each implements all six methods
    - **Property 28: Claude Code agent ID is stable**
    - `claudeAgent.ID()` always returns `constants.DefaultAgent`
    - **Property 29: Claude Code credential presence check is consistent**
    - For any temp dir, `HasCredentials` returns true iff `.credentials.json` exists
    - **Property 30: Claude Code container mount path is always constants.ContainerUserHome/.claude**
    - `ContainerMountPath()` always returns `constants.ContainerUserHome + "/.claude"`
    - **Property 31: Claude Code Dockerfile steps include Node.js and claude-code package**
    - After `claudeAgent.Install(b)`, `b.Build()` contains `nodejs` and `@anthropic-ai/claude-code`
    - **Validates: CC-1, CC-2, CC-3, CC-4, CC-5, CC-6**

  - [x] 12.3 Write unit tests for Claude Code agent
    - `TestClaudeAgentRegistered` — after blank import, `agent.Lookup(constants.DefaultAgent)` succeeds
    - `TestClaudeInstallStepsPresent` — `Install` adds Node.js and `@anthropic-ai/claude-code` RUN steps
    - `TestClaudeCredentialPaths` — `CredentialStorePath` ends with `.claude`
    - `TestClaudeContainerMountPath` — `ContainerMountPath` equals `constants.ContainerUserHome + "/.claude"`
    - `TestClaudeHasCredentialsEmpty` — returns `(false, nil)` for empty dir
    - `TestClaudeHasCredentialsPresent` — returns `(true, nil)` when `.credentials.json` exists
    - **Validates: CC-1, CC-2, CC-3, CC-4, CC-6**

- [x] 13. Implement `internal/cmd/root.go` — Cobra CLI orchestration
  - [x] 13.1 Define Cobra root command, all flags, and mode resolution
    - Positional arg: `<project-path>`
    - Flags: `--agents` (default `constants.DefaultAgent`), `--port` (default 0 = auto), `--ssh-key`, `--rebuild`, `--stop-and-remove`, `--purge`
    - Implement `Mode` type (`ModeStart`, `ModeStop`, `ModePurge`) in `cmd/root.go`
    - Implement `ResolveMode(stopAndRemove, purge bool) (Mode, error)` — returns error for `S ∧ U` (CLI-1)
    - **Flag combination validation is the very first step** before root check or any other logic:
      - Reject `--stop-and-remove` + `--purge` together (CLI-1)
      - Reject missing `<project-path>` in START/STOP mode (CLI-2)
      - Reject `<project-path>` in PURGE mode (CLI-2)
      - Reject `--agents`, `--port`, `--ssh-key`, `--rebuild` in STOP or PURGE mode (CLI-3)
      - Reject `--port` outside 1024–65535 (CLI-5)
      - Reject `--agents` that parses to empty or contains unknown IDs (CLI-6)
    - Export `ParseAgentsFlag(s string) []string` — splits on comma, trims whitespace, drops empty strings
    - _Requirements: Req 1.1, 1.3, 7.4, 7.5, 12.3, 14.4, CLI-1–CLI-6_

  - [x] 13.2 Implement runtime validation and prerequisite checks
    - (Flag combination validation is already done in 13.1 — this step covers runtime checks only)
    - Check effective UID != 0 (root prevention); print error + exit 1 if root
    - Validate project path exists; print error + exit 1 if not
    - Call `docker.NewClient()` + `docker.CheckVersion()`; print error + exit 1 on failure
    - _Requirements: Req 1.4, 6.1–6.4, 11.1–11.3_

  - [x] 13.3 Implement `--stop-and-remove` flow
    - Derive container name from project path via `naming.ContainerName`
    - Inspect container; if found: stop + remove, print confirmation; if not found: print informational message
    - Exit 0 in both cases
    - _Requirements: Req 5.3, 5.4_

  - [x] 13.4 Implement `--purge` flow
    - Collect all bac-managed containers (`ContainerNamePrefix` label filter) and images
    - Print summary of what will be deleted
    - Prompt for confirmation; if declined, exit 0 without deleting
    - If confirmed: stop+remove all containers, remove all images, call `datadir.PurgeRoot()`
    - Print confirmation of what was removed; exit 0
    - _Requirements: Req 16.1–16.6_

  - [x] 13.5 Implement SSH key and Tool_Data_Dir initialisation
    - Call `ssh.DiscoverPublicKey(sshKeyFlag)`; exit 1 if not found
    - Call `naming.ContainerName(projectPath)` to get container name
    - Call `datadir.New(containerName)` to initialise Tool_Data_Dir
    - Load or auto-select SSH port: if `--port` given use it; else `datadir.ReadPort()`; else `portfinder.FindFreePort()`; persist via `datadir.WritePort`
    - Load or generate SSH host key pair: `datadir.ReadHostKey()`; if empty, `ssh.GenerateHostKeyPair()` + `datadir.WriteHostKey()`
    - _Requirements: Req 4.1, 4.4, 12.1–12.3, 13.1, 13.4, 15.1–15.3_

  - [x] 13.6 Implement credential store setup for enabled agents
    - For each enabled agent: `credentials.Resolve(agent.CredentialStorePath(), override)`, `credentials.EnsureDir(resolvedPath)`, `agent.HasCredentials(resolvedPath)` — record agents needing auth
    - _Requirements: Req 8.3, 8.4, 8.5_

  - [x] 13.7 Implement image build and manifest check
    - Inspect existing image for `constants.ManifestFilePath`; compare manifest agent IDs to enabled agents
    - If mismatch and no `--rebuild`: print "agent config changed — run with --rebuild" to stdout; exit 0
    - If no image or `--rebuild`: build image via `docker.BuildImage` using `DockerfileBuilder` (base + dev user + sshd + host key + enabled agents' `Install()` steps + manifest COPY); print "building image" to stdout
    - If build fails: print build output to stderr; exit 1
    - _Requirements: Req 8.1, 8.2, 14.1–14.6_

  - [x] 13.8 Fix session summary label in `FormatSessionSummary`
    - Change `"Project:"` label to `"Project directory:"` to match Req 17.2
    - Verify all five labels match exactly: `"Data directory:"`, `"Project directory:"`, `"SSH port:"`, `"SSH connect:"`, `"Enabled agents:"`
    - _Requirements: Req 17.2_

  - [x] 13.11 Implement `known_hosts` sync in `internal/ssh/known_hosts.go`
    - `SyncKnownHosts(port int, hostPubKey string, noUpdate bool) error` — orchestrates the full sync flow for a given SSH_Port:
      - If `noUpdate` is true: print notice to stdout that `known_hosts` management is disabled and return nil
      - Check `~/.ssh/known_hosts` for entries matching `[localhost]:<port>` and `127.0.0.1:<port>`
      - If neither exists: append both entries derived from `hostPubKey`; create the file with `0600` if absent
      - If both exist and match `hostPubKey`: no-op
      - If entries exist but do NOT match: prompt user `"known_hosts entries for port <port> do not match the stored host key. Replace them? [y/N]"`; if confirmed: remove stale entries and append correct ones, print confirmation to stdout; if declined: print warning to stdout and return nil
    - `RemoveKnownHostsEntries(port int) error` — removes all lines matching `[localhost]:<port>` and `127.0.0.1:<port>` from `~/.ssh/known_hosts`; no-op if file does not exist
    - Both functions must not modify any other lines in `~/.ssh/known_hosts`
    - Wire `SyncKnownHosts` into the START flow in `cmd/root.go` after the container is confirmed ready (after `WaitForSSH` succeeds), passing the `--no-update-known-hosts` flag value
    - _Requirements: Req 18.1–18.9_

  - [x] 13.12 Write property tests for `known_hosts` management
    - **Property 36: SyncKnownHosts never modifies unrelated known_hosts entries**
    - For any pre-existing file with N unrelated entries, after `SyncKnownHosts` the file still contains all N original entries unchanged
    - **Property 37: RemoveKnownHostsEntries only removes entries for the given port**
    - For any file with entries for multiple ports, `RemoveKnownHostsEntries(p)` removes only entries matching port `p`
    - **Property 38: SyncKnownHosts is idempotent when key matches**
    - Calling `SyncKnownHosts` twice with the same key and port produces the same file as calling it once
    - **Validates: Req 18.1–18.6**

  - [x] 13.9 Write property tests for `cmd` pure functions
    - **Property 16: --agents flag parsing produces correct agent ID slices**
    - For any comma-separated string of IDs (including whitespace variants), `ParseAgentsFlag` returns trimmed, non-empty IDs in order
    - **Property 25: Session summary always contains all required fields**
    - For any valid `SessionSummary`, `FormatSessionSummary` output contains all five labelled lines
    - **Property 2: Project path always produces a constants.WorkspaceMountPath bind mount**
    - For any absolute project path, the assembled mount list contains exactly one mount with `ContainerPath == constants.WorkspaceMountPath`
    - **Property 22: SSH port is always bound to the selected host port**
    - For any valid port, the container spec port binding maps `constants.ContainerSSHPort/tcp` to that port
    - **Property 23: Manifest is written for exactly the enabled agents**
    - For any set of enabled agent IDs, the Dockerfile contains a step writing `constants.ManifestFilePath` with exactly those IDs
    - **Property 17: Dockerfile contains install steps for exactly the enabled agents**
    - For any subset of registered agents, the assembled Dockerfile contains each enabled agent's install marker and no disabled agent's marker
    - **Property 18: Every enabled agent's credential store is bind-mounted**
    - For any set of enabled agents, the mount list contains one bind mount per agent at `agent.ContainerMountPath()`
    - **Property 19: Auth warning is printed for every agent with empty credentials**
    - For any subset of agents with empty stores, the CLI output contains one warning per agent in that subset
    - **Property 32: S ∧ U is always rejected**
    - For any invocation with both `--stop-and-remove` and `--purge`, `ResolveMode` returns non-nil error
    - **Property 33: Mode is always exactly one of START, STOP, PURGE**
    - For any valid `(stopAndRemove, purge)` pair (excluding `true ∧ true`), `ResolveMode` returns exactly one mode
    - **Property 34: START-only flags in STOP or PURGE mode always produce errors**
    - For any invocation in STOP or PURGE mode where any of `--agents`, `--port`, `--ssh-key`, `--rebuild`, `--no-update-known-hosts`, or `--no-update-ssh-config` is set, the CLI returns a non-nil error identifying the incompatible flag(s)
    - **Property 35: --port is always within 1024–65535 when provided**
    - For any integer, the port validator accepts it iff `1024 ≤ port ≤ 65535`
    - **Property 36: --agents always resolves to a non-empty list of known IDs**
    - For any comma-separated `--agents` string, the CLI rejects it if the parsed list is empty or contains any ID not in the AgentRegistry
    - **Validates: Req 2.1, 7.4, 8.1–8.5, 12.4, 14.2, 17.1–17.4, CLI-1, CLI-3, CLI-5, CLI-6**

  - [x] 13.10 Write unit tests for `cmd` error paths
    - `TestNoArgsShowsUsage` — no positional arg → usage on stderr, exit 1
    - `TestInvalidProjectPathError` — non-existent path → descriptive error, exit 1
    - `TestRootExecutionPrevented` — UID 0 → error message + suggestion, exit 1
    - `TestDockerDaemonUnreachable` — daemon unreachable → "start Docker" message, exit 1
    - `TestIncompatibleDockerVersion` — version < `constants.MinDockerVersion` → detected + required version, exit 1
    - `TestUnknownAgentIDError` — unknown `--agents` value → error listing available IDs, exit 1
    - `TestDefaultAgentsUsedWhenFlagOmitted` — no `--agents` flag → `constants.DefaultAgent` used
    - `TestExistingContainerReturnsSessionSummary` — already-running container → session summary, exit 0
    - `TestStopAndRemoveNonExistentContainer` — `--stop-and-remove` with no container → message, exit 0
    - `TestManifestMismatchInstructsRebuild` — manifest mismatch → "run with --rebuild", exit 0
    - `TestRebuildFlagForcesRebuild` — `--rebuild` bypasses manifest check
    - `TestPurgeConfirmationPrompt` — `--purge` prints summary before acting
    - `TestPurgeDeclinedDoesNothing` — declined confirmation → nothing deleted, exit 0
    - `TestStopAndPurgeTogetherRejected` — `--stop-and-remove` + `--purge` → error, exit 1 (CLI-1)
    - `TestPurgeWithProjectPathRejected` — `--purge` + `<project-path>` → error, exit 1 (CLI-2)
    - `TestStopWithoutProjectPathRejected` — `--stop-and-remove` without path → usage, exit 1 (CLI-2)
    - `TestAgentsFlagWithStopRejected` — `--agents` + `--stop-and-remove` → error naming flag, exit 1 (CLI-3)
    - `TestPortFlagWithPurgeRejected` — `--port` + `--purge` → error naming flag, exit 1 (CLI-3)
    - `TestRebuildFlagWithStopRejected` — `--rebuild` + `--stop-and-remove` → error naming flag, exit 1 (CLI-3)
    - `TestPortBelowRangeRejected` — `--port 80` → error, exit 1 (CLI-5)
    - `TestPortAboveRangeRejected` — `--port 99999` → error, exit 1 (CLI-5)
    - `TestEmptyAgentsFlagRejected` — `--agents ""` → error, exit 1 (CLI-6)
    - `TestNoUpdateKnownHostsFlagWithStopRejected` — `--no-update-known-hosts` + `--stop-and-remove` → error naming flag, exit 1 (CLI-3)
    - `TestNoUpdateKnownHostsFlagWithPurgeRejected` — `--no-update-known-hosts` + `--purge` → error naming flag, exit 1 (CLI-3)
    - `TestKnownHostsEntryAddedOnStart` — fresh `known_hosts` → both `localhost` and `127.0.0.1` entries appended after container start
    - `TestKnownHostsNoChangeWhenKeyMatches` — existing matching entries → file unchanged
    - `TestKnownHostsStaleEntryPromptsUser` — mismatched entry → user prompted; confirmed → entries replaced; declined → warning printed, file unchanged
    - `TestKnownHostsEntryRemovedOnStopAndRemove` — after `--stop-and-remove`, both entries for the project port are gone
    - `TestKnownHostsSkippedWithNoUpdateFlag` — `--no-update-known-hosts` → notice printed, file not touched
    - **Validates: Req 1.3, 1.4, 5.2, 5.4, 6.2, 6.4, 7.3, 7.5, 11.1–11.3, 14.3, 14.4, 16.5, 18.1–18.9, CLI-1–CLI-3, CLI-5, CLI-6**

- [x] 14. Implement `main.go` (entry point) — entry point
  - Create `main.go` with `package main`, `func main()` calling `cmd.Execute()`
  - Add blank import `_ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"` to wire the Claude Code agent via `init()`
  - No other logic in `main.go`
  - _Requirements: Req 7.2, CC-6_

- [x] 15. Checkpoint — full build and unit/property test suite
  - Run `go build ./...` — must succeed with no errors
  - Run `go vet ./...` — must produce no warnings
  - Run `go test ./...` — all unit and property-based tests must pass
  - Ensure all tests pass; ask the user if questions arise.

- [x] 16. Write integration tests
  - [x] 16.1 Write `TestContainerStartsAndSSHConnects`
    - `//go:build integration`; use `t.TempDir()` for project dir; start container; assert TCP connection to SSH port succeeds; clean up in `t.Cleanup()`
    - **Validates: Req 3.3, 4.3**

  - [x] 16.2 Write `TestWorkspaceMountLiveSync`
    - Write a file to the host project dir; assert it appears at `constants.WorkspaceMountPath/<file>` inside the container
    - **Validates: Req 2.3**

  - [x] 16.3 Write `TestFileOwnershipMatchesHostUser`
    - Create a file inside the container at `/workspace/`; assert its UID/GID on the host matches the invoking user
    - **Validates: Req 10.6**

  - [x] 16.4 Write `TestCredentialVolumePersistedAcrossRestart`
    - Write a file to the credential store inside the container; stop and restart; assert file is still present
    - **Validates: Req 8.6**

  - [x] 16.5 Write `TestSSHPortPersistenceAcrossRestarts`
    - Start container, record SSH port; stop and restart; assert same port is used
    - **Validates: Req 12.2**

  - [x] 16.6 Write `TestSSHHostKeyStableAcrossRebuild`
    - Build image, record host key fingerprint; rebuild with `--rebuild`; assert fingerprint unchanged
    - **Validates: Req 13.3**

  - [x] 16.7 Write `TestPurgeRemovesContainersAndImages`
    - Start a container; run `--purge` with confirmation; assert container, image, and Tool_Data_Dir are all gone
    - **Validates: Req 16.2, 16.4**

  - [x] 16.8 Write `TestClaudeAvailableInContainer`
    - Start container with `--agents claude-code`; exec `claude --version` inside; assert exit 0
    - **Validates: CC-2.3**

  - [x] 16.9 Write `TestClaudeHealthCheck`
    - Start container; call `claudeAgent.HealthCheck(ctx, containerID)`; assert no error
    - **Validates: CC-5**

  - [x] 16.10 Write `TestKnownHostsEntriesLifecycle`
    - Start container; assert both `[localhost]:<port>` and `127.0.0.1:<port>` entries are present in `~/.ssh/known_hosts`; run `--stop-and-remove`; assert both entries are gone
    - **Validates: Req 18.1–18.2, 18.7**

  - [x] 16.11 Write `TestSSHConfigEntryLifecycle`
    - Start container; assert a `Host bac-<dirname>` stanza is present in `~/.ssh/config` with correct `HostName`, `Port`, `User`; run `--stop-and-remove`; assert the stanza is gone
    - **Validates: Req 19.1–19.2, 19.7**

- [x] 17. Add `SSHConfigFile` constant and implement `internal/ssh/ssh_config.go`
  - [x] 17.1 Add `SSHConfigFile = "~/.ssh/config"` constant to `internal/constants/constants.go`
    - Follow the same pattern as `KnownHostsFile`; add a doc comment referencing the SSH_Config_File glossary term and Req 19
    - _Requirements: Req 19 (glossary-derived value)_

  - [x] 17.2 Implement `internal/ssh/ssh_config.go`
    - `SyncSSHConfig(containerName string, port int, noUpdate bool) error`
      - If `noUpdate` is true: print notice to stdout that SSH config management is disabled and return nil
      - Read `constants.SSHConfigFile` (expand `~/`); create with `0600` if absent
      - Search for a `Host <containerName>` stanza
      - If absent: append a new stanza with `HostName localhost`, `Port <port>`, `User constants.ContainerUser`, `StrictHostKeyChecking yes`
      - If present and all fields match: no-op
      - If present but fields differ: replace the stale stanza and print confirmation to stdout
      - Must never modify any stanza whose `Host` value does not equal `containerName`
    - `RemoveSSHConfigEntry(containerName string) error`
      - Remove the `Host <containerName>` stanza from `~/.ssh/config`; no-op if file or stanza absent
    - `RemoveAllBACSSHConfigEntries() error`
      - Remove all stanzas whose `Host` value starts with `constants.ContainerNamePrefix` (`bac-`); no-op if file absent
    - Parsing: read line-by-line; a stanza runs from `Host <name>` to the next `Host` line or EOF; preserve all other stanzas verbatim
    - _Requirements: Req 19.1–19.9_

  - [x] 17.3 Add `--no-update-ssh-config` flag and wire SSH config sync into `internal/cmd/root.go`
    - Add `flagNoUpdateSSHConfig bool` and register `--no-update-ssh-config` flag (analogous to `--no-update-known-hosts`)
    - Reject `--no-update-ssh-config` in STOP or PURGE mode (CLI-3) — same pattern as `--no-update-known-hosts`
    - In `runStart`: after `SyncKnownHosts`, call `sshpkg.SyncSSHConfig(containerName, sshPort, flagNoUpdateSSHConfig)` — both in the "already running" branch and the "new container" branch
    - In `runStop`: after `RemoveKnownHostsEntries`, call `sshpkg.RemoveSSHConfigEntry(containerName)`
    - In `runPurge`: after `datadir.PurgeRoot()`, call `sshpkg.RemoveAllBACSSHConfigEntries()`; include SSH config entries in the purge summary and confirmation output
    - _Requirements: Req 19.1–19.9, CLI-3_

  - [x] 17.4 Write property tests for `ssh_config` management
    - **Property 39: SyncSSHConfig never modifies unrelated SSH config entries**
    - For any pre-existing file with N stanzas whose `Host` values do not start with `constants.ContainerNamePrefix`, after `SyncSSHConfig` the file still contains all N original stanzas unchanged
    - **Property 40: RemoveSSHConfigEntry only removes the entry for the given container name**
    - For any file with entries for multiple container names, `RemoveSSHConfigEntry(name)` removes only the stanza whose `Host` equals `name`
    - **Property 41: SyncSSHConfig is idempotent when entry matches**
    - Calling `SyncSSHConfig` twice with the same container name and port produces the same file as calling it once
    - **Property 42: RemoveAllBACSSHConfigEntries only removes bac- prefixed entries**
    - For any file with a mix of bac-prefixed and non-bac entries, `RemoveAllBACSSHConfigEntries` removes all and only the bac- entries
    - **Validates: Req 19.3, 19.6, 19.7, 19.8**

  - [x] 17.5 Write unit tests for `ssh_config` scenarios
    - `TestSSHConfigEntryAddedOnStart` — no existing entry → stanza appended with correct fields
    - `TestSSHConfigNoChangeWhenEntryMatches` — existing matching stanza → file unchanged
    - `TestSSHConfigStaleEntryReplaced` — existing stanza with wrong port → replaced, confirmation printed
    - `TestSSHConfigEntryRemovedOnStopAndRemove` — after `RemoveSSHConfigEntry`, stanza is gone
    - `TestSSHConfigSkippedWithNoUpdateFlag` — `noUpdate=true` → notice printed, file not touched
    - `TestNoUpdateSSHConfigFlagWithStopRejected` — `--no-update-ssh-config` + `--stop-and-remove` → error naming flag, exit 1 (CLI-3)
    - `TestNoUpdateSSHConfigFlagWithPurgeRejected` — `--no-update-ssh-config` + `--purge` → error naming flag, exit 1 (CLI-3)
    - **Validates: Req 19.1–19.4, 19.9, CLI-3**

- [x] 18. Fix missing known_hosts removal in `--purge` flow (Req 18.8)
  - In `runPurge` in `internal/cmd/root.go`, after `datadir.PurgeRoot()`, iterate over all bac-managed containers' data directories to collect their persisted SSH ports, then call `sshpkg.RemoveKnownHostsEntries(port)` for each
  - Since the Tool_Data_Dir is deleted by `PurgeRoot()`, ports must be read **before** calling `PurgeRoot()`
  - Implementation: call `datadir.ListAll()` (or equivalent — read all subdirs of `constants.ToolDataDirRoot`) before deletion, collect ports, delete data dir, then remove known_hosts entries for each collected port
  - If `datadir` does not yet expose a `ListAll()` helper, add `ListContainerNames() ([]string, error)` to `datadir/datadir.go` that returns the names of all subdirectories under `constants.ToolDataDirRoot`
  - _Requirements: Req 18.8_

- [x] 19. Fix missing port conflict detection for persisted ports (Req 12.5)
  - In `runStart` in `internal/cmd/root.go`, after loading the persisted SSH port (via `dd.ReadPort()`), check whether that port is free using `portfinder.IsPortFree(sshPort)`
  - If the port is **not** free and the container for this project is **not** already running (i.e. it's in use by a different process), print a descriptive error to stderr identifying the port conflict and exit with a non-zero exit code
  - If the port is not free but the container **is** already running (the container itself holds the port), this is the normal reconnect path — no error
  - The check must happen after `dd.ReadPort()` returns a non-zero value and before `dd.WritePort()` is called for a new port
  - _Requirements: Req 12.5_

- [x] 20. Update `requirements-cli-combinations.md` to include `--no-update-ssh-config` flag
  - Add symbol `C` for `--no-update-ssh-config` to the notation table
  - Update CLI-3 formal rule to include `C`: `(S ∨ U) → ¬(A ∨ T ∨ K ∨ R ∨ N ∨ C)`
  - Add rows to the valid combination summary table for `C` in START mode (valid) and `C` with `S` or `U` (invalid, CLI-3)
  - _Requirements: Req 19.9, CLI-3_

- [x] 21. Update container naming implementation to human-readable scheme
  - [x] 21.1 Update `internal/constants/constants.go`
    - Remove `ContainerNameHashLen` (no longer used)
    - Add `ContainerNameParentSep = "_"` — separator between `<parentdir>` and `<dirname>` in level-2+ names
    - Add `ContainerNameCounterSep = "-"` — separator before the numeric counter suffix in level-3+ names
    - _Requirements: Req 5.1_

  - [x] 21.2 Rewrite `internal/naming/naming.go`
    - Replace the SHA-256 implementation with the 3-level algorithm (see Req 5.1 and design-architecture.md Naming Package section)
    - `ContainerName(projectPath string, existingNames []string) (string, error)` — new signature; `existingNames` is the list of `bac-`-prefixed container names already on the host
    - `SanitizeNameComponent(s string) string` — new exported pure helper
    - Update all callers in `internal/cmd/root.go` to pass the list of existing bac-container names (obtained via `docker.ListBACContainers`) as the second argument
    - _Requirements: Req 5.1_

  - [x] 21.3 Update `internal/cmd/root.go` to pass existing container names to `ContainerName`
    - Before calling `naming.ContainerName`, call `docker.ListBACContainers(ctx, client)` to get the current list of `bac-`-prefixed container names on the host
    - Pass the resulting slice as the `existingNames` argument
    - This applies to all three modes (START, STOP, PURGE) wherever `ContainerName` is called
    - _Requirements: Req 5.1_

- [x] 22. Update `SSHConnect` field in `FormatSessionSummary` to use SSH config alias
  - In `internal/cmd/root.go`, update the `SessionSummary` construction to set `SSHConnect` to `"ssh " + containerName` (e.g. `"ssh bac-my-project"`) instead of `fmt.Sprintf("ssh -p %d %s@localhost", sshPort, constants.ContainerUser)`
  - The alias is always valid at this point because `SyncSSHConfig` (Req 19) runs before the session summary is printed
  - Update any unit tests in `internal/cmd/root_test.go` that assert the old `ssh -p <port> dev@localhost` format to expect the new `ssh bac-<name>` format
  - _Requirements: Req 17.2, Req 19_

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- All packages must import glossary values from `constants.*` — never hardcode them
- Property tests use `pgregory.net/rapid` with minimum 100 iterations per property
- Integration tests require `//go:build integration` and a live Docker daemon; skip gracefully with `t.Skip` if Docker is unavailable
- Each property test must have the tag comment `// Feature: bootstrap-ai-coding, Property N: <text>` immediately above the function
- The `internal/agents/claude` package must not import `cmd`, `naming`, `ssh`, `credentials`, `datadir`, `portfinder`, or `docker/runner`
- `main.go` is the only file that blank-imports agent modules
- Flag combination validation (CLI-1–CLI-6) is always the **first** check in `cmd/root.go`, before root UID check or Docker checks


