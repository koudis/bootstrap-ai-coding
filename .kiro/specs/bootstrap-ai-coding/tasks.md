# Implementation Plan: Vibe Kanban Agent Module

## Overview

Implement the Vibe Kanban agent module (`internal/agents/vibekanban/`) — a web-based project management tool that runs as a background service inside the container. The implementation adds the agent constant, extends the DockerfileBuilder with an `Entrypoint()` method, adds `ExecInContainerWithOutput()` to the runner, creates the agent module with auto-start via ENTRYPOINT wrapper + supervisor script, extends the session summary with the Vibe Kanban URL, and integrates port discovery into `runStart()`.

## Tasks

- [ ] 1. Add Vibe Kanban constant and update DefaultAgents
  - [ ] 1.1 Add `VibeKanbanAgentName` constant and update `DefaultAgents` in `internal/constants/constants.go`
    - Add `VibeKanbanAgentName = "vibe-kanban"` constant with comment referencing VK-1
    - Update `DefaultAgents` to append `"," + VibeKanbanAgentName` to the existing value
    - _Requirements: VK-1.1, VK-7.1_

- [ ] 2. Extend DockerfileBuilder with Entrypoint method
  - [ ] 2.1 Add `Entrypoint()` method to `DockerfileBuilder` in `internal/docker/builder.go`
    - Implement `Entrypoint(args ...string)` that appends an `ENTRYPOINT [...]` instruction in exec form
    - Each arg is quoted with `fmt.Sprintf("%q", a)` and joined with `", "`
    - _Requirements: VK-3.1_

  - [ ]* 2.2 Write unit tests for `Entrypoint()` method in `internal/docker/builder_test.go`
    - Test single-arg entrypoint produces correct exec-form instruction
    - Test multi-arg entrypoint produces correct exec-form instruction
    - _Requirements: VK-3.1_

- [ ] 3. Add ExecInContainerWithOutput helper to runner
  - [ ] 3.1 Add `ExecInContainerWithOutput()` function to `internal/docker/runner.go`
    - Implement function that runs a command inside a container and returns `(exitCode int, stdout string, err error)`
    - Use `ContainerExecCreate` with `AttachStdout: true, AttachStderr: true`
    - Use `ContainerExecAttach` and `stdcopy.StdCopy` to separate stdout from stderr
    - Use `ContainerExecInspect` to get the exit code
    - Add necessary imports: `"github.com/docker/docker/pkg/stdcopy"`
    - _Requirements: VK-8.2_

- [ ] 4. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 5. Create the Vibe Kanban agent module
  - [ ] 5.1 Create `internal/agents/vibekanban/vibekanban.go` with full agent implementation
    - Implement `vibeKanbanAgent` struct with `init()` calling `agent.Register()`
    - Implement `ID()` returning `constants.VibeKanbanAgentName`
    - Implement `Install(b *docker.DockerfileBuilder)`:
      - Conditional Node.js installation (check `b.IsNodeInstalled()`, install Node.js 22 if not, call `b.MarkNodeInstalled()`)
      - `npm install -g --no-fund --no-audit vibe-kanban`
      - Generate supervisor script with crash recovery (MAX_RESTARTS=5, WINDOW_SECONDS=60, DELAY_SECONDS=5), replacing `__USERNAME__` with `b.Username()`
      - Generate entrypoint wrapper script (`bac-entrypoint.sh`) that starts supervisor in background then `exec "$@"`
      - Call `b.Entrypoint("/usr/local/bin/bac-entrypoint.sh")`
    - Implement `CredentialStorePath()` returning `""`
    - Implement `ContainerMountPath(homeDir string)` returning `""`
    - Implement `HasCredentials(storePath string)` returning `(true, nil)`
    - Implement `HealthCheck(ctx, c, containerID)`:
      - Check 1: `vibe-kanban --version` exits 0 (binary presence)
      - Check 2: `pgrep -f vibe-kanban` with up to 5 retries at 2-second intervals (process running)
    - _Requirements: VK-1.1, VK-1.3, VK-2.1, VK-2.2, VK-2.4, VK-3.1, VK-3.2, VK-3.5, VK-3.6, VK-4.1, VK-4.2, VK-4.3, VK-5.1, VK-5.2_

  - [ ] 5.2 Add blank import in `main.go` for the vibekanban package
    - Add `_ "github.com/koudis/bootstrap-ai-coding/internal/agents/vibekanban"` to the import block
    - _Requirements: VK-1.3, VK-6.2_

- [ ] 6. Extend SessionSummary and integrate port discovery
  - [ ] 6.1 Add `VibeKanbanURL` field to `SessionSummary` struct and update `FormatSessionSummary` in `internal/cmd/root.go`
    - Add `VibeKanbanURL string` field to `SessionSummary`
    - Update `FormatSessionSummary` to use `strings.Builder` and conditionally include "Vibe Kanban:" line when `VibeKanbanURL` is non-empty
    - _Requirements: VK-8.3, VK-8.4_

  - [ ] 6.2 Add `discoverVibeKanbanPort()` function and integrate into `runStart()` in `internal/cmd/root.go`
    - Implement `discoverVibeKanbanPort(ctx, c, containerID)` that:
      - Executes `ss -tlnp` inside the container via `dockerpkg.ExecInContainerWithOutput`
      - Greps for `vibe-kanban` and parses the port number
      - Retries for up to 30 seconds with 2-second intervals
      - Returns `(port int, err error)`
    - In `runStart()`, after SSH health check passes, check if `vibe-kanban` is in enabled agents
    - If enabled, call `discoverVibeKanbanPort()` and set `vibeKanbanURL`
    - On failure, print warning to stderr and continue (graceful degradation)
    - Pass `VibeKanbanURL` to `printSessionSummary` / `SessionSummary`
    - Update `printSessionSummary` to accept and pass through the URL
    - _Requirements: VK-8.2, VK-8.3, VK-8.4_

- [ ] 7. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 8. Write unit tests for the Vibe Kanban agent module
  - [ ] 8.1 Create `internal/agents/vibekanban/vibekanban_test.go` with unit tests
    - `TestID` — returns `constants.VibeKanbanAgentName`
    - `TestInstallNodeAlreadyInstalled` — skips Node.js when `IsNodeInstalled()` is true
    - `TestInstallNodeNotInstalled` — installs Node.js when `IsNodeInstalled()` is false
    - `TestInstallContainsNpmPackage` — output contains `npm install -g` with `vibe-kanban`
    - `TestInstallContainsEntrypoint` — output contains ENTRYPOINT instruction
    - `TestInstallContainsSupervisor` — output contains supervisor script with crash recovery params
    - `TestInstallDoesNotContainCMD` — output does NOT contain CMD instruction
    - `TestInstallNoRustNoPnpm` — output does NOT contain rust/pnpm references
    - `TestCredentialStorePath` — returns empty string
    - `TestContainerMountPath` — returns empty string for various homeDir values
    - `TestHasCredentials` — returns `(true, nil)`
    - `TestHealthCheckBinaryFailure` — error message identifies binary check
    - `TestHealthCheckProcessFailure` — error message identifies process check
    - _Requirements: VK-1.1, VK-2.1, VK-2.2, VK-2.4, VK-3.1, VK-4.1, VK-4.2, VK-4.3, VK-5.1, VK-5.2_

  - [ ]* 8.2 Write unit tests for `FormatSessionSummary` with Vibe Kanban URL in `internal/cmd/root_test.go`
    - `TestFormatSessionSummaryWithVibeKanban` — URL line present when VibeKanbanURL is set
    - `TestFormatSessionSummaryWithoutVibeKanban` — URL line absent when VibeKanbanURL is empty
    - _Requirements: VK-8.3, VK-8.4_

- [ ] 9. Write property-based tests for the Vibe Kanban agent module
  - [ ]* 9.1 Write property test: Node.js conditional installation invariant
    - **Property 1: Node.js conditional installation invariant**
    - For any DockerfileBuilder state, calling Install() results in at most one Node.js installation block and `IsNodeInstalled()` returns true after
    - Use `rapid.Bool()` to draw whether Node.js is pre-installed
    - **Validates: Requirements VK-2.1**

  - [ ]* 9.2 Write property test: Install does not emit CMD
    - **Property 2: Install does not emit CMD**
    - For any DockerfileBuilder state, calling Install() does NOT append any line starting with `CMD`
    - Use `rapid.Bool()` to draw whether Node.js is pre-installed
    - **Validates: Requirements VK-3.1**

  - [ ]* 9.3 Write property test: No-credential-store invariant
    - **Property 3: No-credential-store invariant**
    - For any string homeDir, `ContainerMountPath(homeDir)` returns empty; for any storePath, `HasCredentials(storePath)` returns `(true, nil)`
    - Use `rapid.String()` to draw arbitrary homeDir and storePath values
    - **Validates: Requirements VK-4.2, VK-4.3**

  - [ ]* 9.4 Write property test: Session summary includes Vibe Kanban URL for any valid port
    - **Property 4: Session summary includes Vibe Kanban URL for any valid port**
    - For any valid TCP port (1-65535), `FormatSessionSummary()` with `VibeKanbanURL` set includes the URL; when empty, output does NOT contain "Vibe Kanban:"
    - Use `rapid.IntRange(1, 65535)` to draw port numbers
    - **Validates: Requirements VK-8.3**

  - [ ]* 9.5 Write property test: Supervisor script contains correct backoff parameters
    - **Property 5: Supervisor script contains correct backoff parameters**
    - For any valid Linux username, the supervisor script generated by Install() contains `MAX_RESTARTS=5`, `WINDOW_SECONDS=60`, and `DELAY_SECONDS=5`
    - Use `rapid.StringMatching(`[a-z_][a-z0-9_-]*`)` to draw usernames
    - **Validates: Requirements VK-3.5**

- [ ] 10. Write integration tests for the Vibe Kanban agent module
  - [ ]* 10.1 Create `internal/agents/vibekanban/integration_test.go` with integration tests
    - Gated by `//go:build integration`
    - Include `TestMain` with consent gate and base image removal
    - `TestVibeKanbanInstallsAndRuns` — full image build, binary present (`which vibe-kanban` exits 0), process running
    - `TestVibeKanbanHealthCheck` — HealthCheck passes on a live container
    - `TestVibeKanbanPortDiscovery` — port is discoverable via ss after startup
    - `TestVibeKanbanCrashRecovery` — process restarts after being killed (kill + wait + verify running)
    - `TestVibeKanbanAccessibleFromHost` — HTTP GET to localhost:port returns 2xx (host network mode)
    - _Requirements: VK-2.3, VK-3.1, VK-3.3, VK-3.5, VK-5.1, VK-5.2, VK-8.1, VK-8.2_

- [ ] 11. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document
- Unit tests validate specific examples and edge cases
- The design uses Go — all code examples and implementations use Go
- The agent module follows the same pattern as `internal/agents/buildresources/` and `internal/agents/augment/`
- `ExecInContainerWithOutput` is needed for port discovery (capturing stdout from `ss -tlnp`)
- The `Entrypoint()` builder method is generic and reusable by future agents needing initialization before CMD

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "2.1", "3.1"] },
    { "id": 1, "tasks": ["2.2", "5.1"] },
    { "id": 2, "tasks": ["5.2", "6.1"] },
    { "id": 3, "tasks": ["6.2"] },
    { "id": 4, "tasks": ["8.1", "8.2", "9.1", "9.2", "9.3", "9.4", "9.5"] },
    { "id": 5, "tasks": ["10.1"] }
  ]
}
```
