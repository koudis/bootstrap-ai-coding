# Implementation Plan: bootstrap-ai-coding

## Overview

Implement the `bootstrap-ai-coding` Go CLI tool in package dependency order: pure constants first, then leaf packages, then the agent interface and registry, then Docker helpers, then the Claude Code agent module, and finally the Cobra CLI orchestration and entry point. Each task builds directly on the previous ones. Integration tests are the final task.

The module path is `github.com/user/bootstrap-ai-coding`. All glossary-derived values must be referenced via `constants.*` — never hardcoded.

## Tasks

- [ ] 1. Initialise Go module and constants package
  - Run `go mod init github.com/user/bootstrap-ai-coding` and add all required dependencies (`github.com/spf13/cobra`, `github.com/docker/docker`, `golang.org/x/crypto`, `pgregory.net/rapid`, `github.com/stretchr/testify`) to `go.mod`/`go.sum`
  - Create `constants/constants.go` with every glossary-derived constant: `BaseContainerImage`, `ContainerUser`, `ContainerUserHome`, `WorkspaceMountPath`, `SSHPortStart`, `ContainerSSHPort`, `ToolDataDirRoot`, `ContainerNamePrefix`, `ContainerNameHashLen`, `ManifestFilePath`, `DefaultAgent`, `SSHHostKeyType`, `MinDockerVersion`, `ToolDataDirPerm`, `ToolDataFilePerm`
  - No other package may hardcode any of these values
  - _Requirements: Req 1–17, CC-1–CC-6, CLI-1–CLI-6 (all glossary-derived values)_

- [ ] 2. Implement `naming` package
  - [ ] 2.1 Implement `naming/naming.go`
    - `ContainerName(projectPath string) (string, error)` — resolves to absolute path, SHA-256 hashes it, returns `constants.ContainerNamePrefix + first (constants.ContainerNameHashLen/2) bytes as hex`
    - _Requirements: Req 5.1_

  - [ ]* 2.2 Write property tests for `naming`
    - **Property 12: Container naming is deterministic and collision-resistant**
    - Test determinism: same path always returns same name
    - Test collision resistance: two distinct paths return distinct names
    - Test prefix: every name starts with `constants.ContainerNamePrefix`
    - Test length: name is always `len(constants.ContainerNamePrefix) + constants.ContainerNameHashLen` characters
    - **Validates: Req 5.1**

- [ ] 3. Implement `portfinder` package
  - [ ] 3.1 Implement `portfinder/portfinder.go`
    - `FindFreePort() (int, error)` — iterates from `constants.SSHPortStart` upward, returns first free TCP port
    - `IsPortFree(port int) bool` — attempts `net.Listen` on `127.0.0.1:<port>`, returns true if successful
    - _Requirements: Req 12.1_

  - [ ]* 3.2 Write property tests for `portfinder`
    - **Property 20: Port finder returns the first free port at or above constants.SSHPortStart**
    - Occupy 0–5 ports starting at `constants.SSHPortStart` using real listeners, assert `FindFreePort` returns the first unoccupied port
    - Assert returned port is always `>= constants.SSHPortStart`
    - Assert `IsPortFree` returns true for the returned port
    - **Validates: Req 12.1**

- [ ] 4. Implement `agent` interface and registry
  - [ ] 4.1 Implement `agent/agent.go`
    - Define the `Agent` interface with all six methods: `ID() string`, `Install(b *docker.DockerfileBuilder)`, `CredentialStorePath() string`, `ContainerMountPath() string`, `HasCredentials(storePath string) (bool, error)`, `HealthCheck(ctx context.Context, containerID string) error`
    - _Requirements: Req 7.1_

  - [ ] 4.2 Implement `agent/registry.go`
    - Package-level `registry map[string]Agent`
    - `Register(a Agent)` — panics on duplicate ID
    - `Lookup(id string) (Agent, error)` — returns descriptive error listing known IDs when not found
    - `All() []Agent` — returns all registered agents
    - `KnownIDs() []string` — returns sorted slice of all registered IDs
    - _Requirements: Req 7.2, 7.3_

  - [ ]* 4.3 Write unit tests for `agent` registry
    - Test `Lookup` returns error for unknown ID and includes available IDs in the message
    - Test `Register` panics on duplicate ID
    - Test `KnownIDs` returns sorted results
    - **Validates: Req 7.2, 7.3**

  - [ ]* 4.4 Write property test for unknown agent IDs
    - **Property 26: Unknown agent IDs always produce errors**
    - For any string not matching a registered ID, `Lookup` returns non-nil error
    - **Validates: Req 7.3**

- [ ] 5. Implement `docker/builder.go` — DockerfileBuilder
  - [ ] 5.1 Implement `docker/builder.go`
    - `DockerfileBuilder` struct with `lines []string`
    - `NewDockerfileBuilder(uid, gid int, publicKey, hostKeyPriv, hostKeyPub string) *DockerfileBuilder` — pre-seeds with: `FROM constants.BaseContainerImage`, `openssh-server` + `sudo` install, `dev` user creation with matching UID/GID, passwordless sudo entry, `authorized_keys` injection, SSH host key injection at `/etc/ssh/ssh_host_ed25519_key{,.pub}`, `sshd_config` hardening (`PasswordAuthentication no`, `PermitRootLogin no`, `PubkeyAuthentication yes`), `mkdir -p /run/sshd`, `CMD ["/usr/sbin/sshd", "-D"]`
    - Methods: `From`, `Run`, `Env`, `Copy`, `Cmd`, `Build() string`, `Lines() []string`
    - _Requirements: Req 3.1, 4.2, 4.5, 9.1–9.3, 10.1–10.5, 13.2_

  - [ ]* 5.2 Write property tests for `DockerfileBuilder`
    - **Property 3: Generated Dockerfile always uses constants.BaseContainerImage as base image**
    - **Property 4: Generated Dockerfile always includes SSH server and Container_User**
    - **Property 5: Container_User UID and GID always match the host user**
    - **Property 6: Container_User always has passwordless sudo**
    - **Property 7: sshd_config always disables password authentication**
    - **Property 8: Public key is always injected into constants.ContainerUserHome/.ssh/authorized_keys**
    - **Property 10: SSH host key is always injected into the Dockerfile**
    - For each property, draw random UID/GID/key values and assert the invariant holds in `b.Build()`
    - **Validates: Req 3.1, 4.2, 4.5, 9.1–9.3, 10.1–10.5, 13.2**

- [ ] 6. Implement `docker/client.go` — Docker SDK wrapper and prerequisite checks
  - [ ] 6.1 Implement `docker/client.go`
    - `Client` interface (or struct wrapping `*dockerclient.Client`) exposing: `Ping`, `ServerVersion`, `ImageInspect`, `ImageBuild`, `ContainerInspect`, `ContainerCreate`, `ContainerStart`, `ContainerStop`, `ContainerRemove`, `ContainerList`, `ImageList`, `ImageRemove`
    - `NewClient() (*Client, error)` — creates Docker SDK client, calls `Ping`, returns descriptive error if daemon unreachable
    - `CheckVersion(c *Client) error` — parses server version, returns error if `< constants.MinDockerVersion`
    - `IsVersionCompatible(version string) bool` — pure helper for version comparison (testable without daemon)
    - _Requirements: Req 6.1, 6.2, 6.3, 6.4_

  - [ ]* 6.2 Write property test for Docker version comparison
    - **Property 13: Docker version comparison is correct**
    - For any `(major, minor, patch)` triple, `IsVersionCompatible` returns true iff `major > 20 || (major == 20 && minor >= 10)`
    - **Validates: Req 6.3, 6.4**

  - [ ]* 6.3 Write unit tests for Docker client error paths
    - Test `NewClient` returns error when daemon is unreachable (mock or stub)
    - Test `CheckVersion` returns error with detected and required versions when version is too old
    - **Validates: Req 6.2, 6.4**

- [ ] 7. Implement `docker/runner.go` — container lifecycle helpers
  - Implement `docker/runner.go` with helpers: `BuildImage(ctx, client, spec)`, `CreateContainer(ctx, client, spec)`, `StartContainer(ctx, client, name)`, `StopContainer(ctx, client, name)`, `RemoveContainer(ctx, client, name)`, `InspectContainer(ctx, client, name)`, `WaitForSSH(ctx, host, port, timeout)` (TCP health-check polling), `ListBACContainers(ctx, client)`, `ListBACImages(ctx, client)`
  - All container names and image tags must be derived from `constants.ContainerNamePrefix`
  - `WaitForSSH` polls `host:port` with TCP dial until success or timeout (10 s)
  - _Requirements: Req 3.3, 5.2, 5.3, 5.4, 14.1, 14.5, 14.6, 16.2, 16.4_

- [ ] 8. Implement `ssh` package — public key discovery
  - [ ] 8.1 Implement `ssh/keys.go`
    - `DiscoverPublicKey(sshKeyFlag string) (string, error)` — tries `sshKeyFlag` (if non-empty), then `~/.ssh/id_ed25519.pub`, then `~/.ssh/id_rsa.pub`; returns file contents of first found; returns descriptive error if none found
    - `GenerateHostKeyPair() (priv, pub string, err error)` — generates an ed25519 host key pair using `golang.org/x/crypto/ssh`; returns PEM-encoded private key and authorised-keys-format public key
    - _Requirements: Req 4.1, 4.4, 13.1_

  - [ ]* 8.2 Write property test for SSH key discovery precedence
    - **Property 9: Public key discovery respects precedence order**
    - Simulate presence/absence of each candidate file and assert `DiscoverPublicKey` returns the highest-precedence available key
    - **Validates: Req 4.1**

  - [ ]* 8.3 Write unit tests for SSH key error paths
    - Test `DiscoverPublicKey` returns descriptive error when no key file exists and no flag is set
    - **Validates: Req 4.4**

- [ ] 9. Implement `credentials` package
  - [ ] 9.1 Implement `credentials/store.go`
    - `Resolve(agentDefault, override string) string` — returns `override` if non-empty, else expands `~` in `agentDefault`
    - `EnsureDir(path string) error` — `os.MkdirAll(path, constants.ToolDataDirPerm)`
    - _Requirements: Req 8.3, 8.4_

  - [ ]* 9.2 Write property tests for `credentials`
    - **Property 14: Credential store path resolution respects override precedence**
    - For any `(agentDefault, override)` pair, `Resolve` returns override when non-empty, expanded default when override is empty
    - **Property 15: Credential store directory is always created before mounting**
    - `EnsureDir` on a non-existent path creates the directory with `constants.ToolDataDirPerm`
    - **Validates: Req 8.3, 8.4**

- [ ] 10. Implement `datadir` package
  - [ ] 10.1 Implement `datadir/datadir.go`
    - `DataDir` struct with `path string`
    - `New(containerName string) (*DataDir, error)` — expands `constants.ToolDataDirRoot`, joins with `containerName`, calls `os.MkdirAll` with `constants.ToolDataDirPerm`
    - `Path() string`
    - `ReadPort() (int, error)` / `WritePort(port int) error` — reads/writes `port` file with `constants.ToolDataFilePerm`
    - `ReadHostKey() (priv, pub string, err error)` / `WriteHostKey(priv, pub string) error` — reads/writes `ssh_host_ed25519_key` and `ssh_host_ed25519_key.pub` with `constants.ToolDataFilePerm`
    - `ReadManifest() ([]string, error)` / `WriteManifest(agentIDs []string) error` — JSON-encodes/decodes agent ID list, writes with `constants.ToolDataFilePerm`
    - `PurgeRoot() error` — `os.RemoveAll` on expanded `constants.ToolDataDirRoot`
    - _Requirements: Req 12.2, 13.1, 13.4, 14.2, 15.1–15.3_

  - [ ]* 10.2 Write property tests for `datadir`
    - **Property 11: SSH host key is stable across rebuilds**
    - Write a key pair, read it back multiple times, assert identical result each time
    - **Property 21: Persisted port round-trips correctly**
    - For any valid port number, `WritePort` then `ReadPort` returns the same value
    - **Property 24: Tool_Data_Dir is created with constants.ToolDataDirPerm permissions**
    - `New` on a non-existent path creates directory with mode `constants.ToolDataDirPerm`
    - **Validates: Req 12.2, 13.1, 13.3, 13.4, 15.2, 15.3**

  - [ ]* 10.3 Write unit tests for `datadir` file permissions
    - Assert all files written by `WritePort`, `WriteHostKey`, `WriteManifest` have mode `constants.ToolDataFilePerm`
    - **Validates: Req 13.4, 15.3**

- [ ] 11. Checkpoint — core packages complete
  - Ensure `go build ./...` and `go test ./...` pass for all packages implemented so far (`constants`, `naming`, `portfinder`, `agent`, `docker/builder.go`, `docker/client.go`, `ssh`, `credentials`, `datadir`)
  - Ensure all tests pass; ask the user if questions arise.

- [ ] 12. Implement `agents/claude` — Claude Code agent module
  - [ ] 12.1 Implement `agents/claude/claude.go`
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

  - [ ]* 12.2 Write property tests for Claude Code agent
    - **Property 27: All registered agents satisfy the Agent interface**
    - After blank-importing `agents/claude`, assert `agent.All()` returns at least one agent and each implements all six methods
    - **Property 28: Claude Code agent ID is stable**
    - `claudeAgent.ID()` always returns `constants.DefaultAgent`
    - **Property 29: Claude Code credential presence check is consistent**
    - For any temp dir, `HasCredentials` returns true iff `.credentials.json` exists
    - **Property 30: Claude Code container mount path is always constants.ContainerUserHome/.claude**
    - `ContainerMountPath()` always returns `constants.ContainerUserHome + "/.claude"`
    - **Property 31: Claude Code Dockerfile steps include Node.js and claude-code package**
    - After `claudeAgent.Install(b)`, `b.Build()` contains `nodejs` and `@anthropic-ai/claude-code`
    - **Validates: CC-1, CC-2, CC-3, CC-4, CC-5, CC-6**

  - [ ]* 12.3 Write unit tests for Claude Code agent
    - `TestClaudeAgentRegistered` — after blank import, `agent.Lookup(constants.DefaultAgent)` succeeds
    - `TestClaudeInstallStepsPresent` — `Install` adds Node.js and `@anthropic-ai/claude-code` RUN steps
    - `TestClaudeCredentialPaths` — `CredentialStorePath` ends with `.claude`
    - `TestClaudeContainerMountPath` — `ContainerMountPath` equals `constants.ContainerUserHome + "/.claude"`
    - `TestClaudeHasCredentialsEmpty` — returns `(false, nil)` for empty dir
    - `TestClaudeHasCredentialsPresent` — returns `(true, nil)` when `.credentials.json` exists
    - **Validates: CC-1, CC-2, CC-3, CC-4, CC-6**

- [ ] 13. Implement `cmd/root.go` — Cobra CLI orchestration
  - [ ] 13.1 Define Cobra root command, all flags, and mode resolution
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

  - [ ] 13.2 Implement runtime validation and prerequisite checks
    - (Flag combination validation is already done in 13.1 — this step covers runtime checks only)
    - Check effective UID != 0 (root prevention); print error + exit 1 if root
    - Validate project path exists; print error + exit 1 if not
    - Call `docker.NewClient()` + `docker.CheckVersion()`; print error + exit 1 on failure
    - _Requirements: Req 1.4, 6.1–6.4, 11.1–11.3_

  - [ ] 13.3 Implement `--stop-and-remove` flow
    - Derive container name from project path via `naming.ContainerName`
    - Inspect container; if found: stop + remove, print confirmation; if not found: print informational message
    - Exit 0 in both cases
    - _Requirements: Req 5.3, 5.4_

  - [ ] 13.4 Implement `--purge` flow
    - Collect all bac-managed containers (`ContainerNamePrefix` label filter) and images
    - Print summary of what will be deleted
    - Prompt for confirmation; if declined, exit 0 without deleting
    - If confirmed: stop+remove all containers, remove all images, call `datadir.PurgeRoot()`
    - Print confirmation of what was removed; exit 0
    - _Requirements: Req 16.1–16.6_

  - [ ] 13.5 Implement SSH key and Tool_Data_Dir initialisation
    - Call `ssh.DiscoverPublicKey(sshKeyFlag)`; exit 1 if not found
    - Call `naming.ContainerName(projectPath)` to get container name
    - Call `datadir.New(containerName)` to initialise Tool_Data_Dir
    - Load or auto-select SSH port: if `--port` given use it; else `datadir.ReadPort()`; else `portfinder.FindFreePort()`; persist via `datadir.WritePort`
    - Load or generate SSH host key pair: `datadir.ReadHostKey()`; if empty, `ssh.GenerateHostKeyPair()` + `datadir.WriteHostKey()`
    - _Requirements: Req 4.1, 4.4, 12.1–12.3, 13.1, 13.4, 15.1–15.3_

  - [ ] 13.6 Implement credential store setup for enabled agents
    - For each enabled agent: `credentials.Resolve(agent.CredentialStorePath(), override)`, `credentials.EnsureDir(resolvedPath)`, `agent.HasCredentials(resolvedPath)` — record agents needing auth
    - _Requirements: Req 8.3, 8.4, 8.5_

  - [ ] 13.7 Implement image build and manifest check
    - Inspect existing image for `constants.ManifestFilePath`; compare manifest agent IDs to enabled agents
    - If mismatch and no `--rebuild`: print "agent config changed — run with --rebuild" to stdout; exit 0
    - If no image or `--rebuild`: build image via `docker.BuildImage` using `DockerfileBuilder` (base + dev user + sshd + host key + enabled agents' `Install()` steps + manifest COPY); print "building image" to stdout
    - If build fails: print build output to stderr; exit 1
    - _Requirements: Req 8.1, 8.2, 14.1–14.6_

  - [ ] 13.8 Implement container start and session summary
    - Inspect container by name; if already running: print session summary + exit 0
    - Create container with all bind mounts (`/workspace` + per-agent credential volumes), port binding (`constants.ContainerSSHPort` → SSH port), labels
    - Start container; call `docker.WaitForSSH`; if timeout: stop+remove container, print error + exit 1
    - Print auth warnings for agents needing credentials
    - Export `FormatSessionSummary(s SessionSummary) string` — formats all five labelled fields
    - Print session summary to stdout; exit 0
    - _Requirements: Req 2.1–2.3, 3.1–3.3, 5.2, 8.5, 12.4, 12.5, 12.6, 17.1–17.4_

  - [ ]* 13.9 Write property tests for `cmd` pure functions
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
    - **Property 35: --port is always within 1024–65535 when provided**
    - For any integer, the port validator accepts it iff `1024 ≤ port ≤ 65535`
    - **Validates: Req 2.1, 7.4, 8.1–8.5, 12.4, 14.2, 17.1–17.4, CLI-1, CLI-3, CLI-5**

  - [ ]* 13.10 Write unit tests for `cmd` error paths
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
    - **Validates: Req 1.3, 1.4, 5.2, 5.4, 6.2, 6.4, 7.3, 7.5, 11.1–11.3, 14.3, 14.4, 16.5, CLI-1–CLI-3, CLI-5, CLI-6**

- [ ] 14. Implement `main.go` — entry point
  - Create `main.go` with `package main`, `func main()` calling `cmd.Execute()`
  - Add blank import `_ "github.com/user/bootstrap-ai-coding/agents/claude"` to wire the Claude Code agent via `init()`
  - No other logic in `main.go`
  - _Requirements: Req 7.2, CC-6_

- [ ] 15. Checkpoint — full build and unit/property test suite
  - Run `go build ./...` — must succeed with no errors
  - Run `go vet ./...` — must produce no warnings
  - Run `go test ./...` — all unit and property-based tests must pass
  - Ensure all tests pass; ask the user if questions arise.

- [ ] 16. Write integration tests
  - [ ]* 16.1 Write `TestContainerStartsAndSSHConnects`
    - `//go:build integration`; use `t.TempDir()` for project dir; start container; assert TCP connection to SSH port succeeds; clean up in `t.Cleanup()`
    - **Validates: Req 3.3, 4.3**

  - [ ]* 16.2 Write `TestWorkspaceMountLiveSync`
    - Write a file to the host project dir; assert it appears at `constants.WorkspaceMountPath/<file>` inside the container
    - **Validates: Req 2.3**

  - [ ]* 16.3 Write `TestFileOwnershipMatchesHostUser`
    - Create a file inside the container at `/workspace/`; assert its UID/GID on the host matches the invoking user
    - **Validates: Req 10.6**

  - [ ]* 16.4 Write `TestCredentialVolumePersistedAcrossRestart`
    - Write a file to the credential store inside the container; stop and restart; assert file is still present
    - **Validates: Req 8.6**

  - [ ]* 16.5 Write `TestSSHPortPersistenceAcrossRestarts`
    - Start container, record SSH port; stop and restart; assert same port is used
    - **Validates: Req 12.2**

  - [ ]* 16.6 Write `TestSSHHostKeyStableAcrossRebuild`
    - Build image, record host key fingerprint; rebuild with `--rebuild`; assert fingerprint unchanged
    - **Validates: Req 13.3**

  - [ ]* 16.7 Write `TestPurgeRemovesContainersAndImages`
    - Start a container; run `--purge` with confirmation; assert container, image, and Tool_Data_Dir are all gone
    - **Validates: Req 16.2, 16.4**

  - [ ]* 16.8 Write `TestClaudeAvailableInContainer`
    - Start container with `--agents claude-code`; exec `claude --version` inside; assert exit 0
    - **Validates: CC-2.3**

  - [ ]* 16.9 Write `TestClaudeHealthCheck`
    - Start container; call `claudeAgent.HealthCheck(ctx, containerID)`; assert no error
    - **Validates: CC-5**

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- All packages must import glossary values from `constants.*` — never hardcode them
- Property tests use `pgregory.net/rapid` with minimum 100 iterations per property
- Integration tests require `//go:build integration` and a live Docker daemon; skip gracefully with `t.Skip` if Docker is unavailable
- Each property test must have the tag comment `// Feature: bootstrap-ai-coding, Property N: <text>` immediately above the function
- The `agents/claude` package must not import `cmd`, `naming`, `ssh`, `credentials`, `datadir`, `portfinder`, or `docker/runner`
- `main.go` is the only file that blank-imports agent modules
- Flag combination validation (CLI-1–CLI-6) is always the **first** check in `cmd/root.go`, before root UID check or Docker checks
