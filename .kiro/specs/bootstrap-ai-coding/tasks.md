# Implementation Tasks

## Completed

### EnsureBaseImageAbsent integration test precondition

Extract the base-image-absent precondition into a shared `testutil.EnsureBaseImageAbsent()` helper and call it from `TestMain` in every integration test package.

## Tasks

### Deduplicate Node.js installation across agent modules

When both Claude Code and Augment Code are enabled (the default), both agents independently install Node.js — Claude via `setup_lts.x` and Augment via `setup_22.x`. This results in duplicate `apt-get update`, duplicate NodeSource setup, and duplicate `apt-get install nodejs` steps in the Dockerfile. Since Node.js 22 is the current LTS and satisfies both agents' requirements, the installation should happen once.

**Approach:** Each agent's `Install()` method should skip the Node.js installation if Node.js is already being installed by a prior agent. The simplest mechanism is to have the `DockerfileBuilder` track whether a Node.js install step has already been appended, and expose a method agents can check.

- [x] 6. Deduplicate Node.js installation when multiple agents are enabled
  - [x] 6.1 Add Node.js tracking to `DockerfileBuilder`
    - Add a `nodeInstalled bool` field to `DockerfileBuilder`
    - Add `MarkNodeInstalled()` method that sets the flag
    - Add `IsNodeInstalled() bool` method that returns the flag
  - [x] 6.2 Update Claude Code `Install()` to use the tracking
    - Check `b.IsNodeInstalled()` before appending Node.js install steps
    - If not installed: install Node.js 22 (changed from `setup_lts.x` to `setup_22.x` to satisfy AC-2's Node.js 22+ requirement), then call `b.MarkNodeInstalled()`
    - If already installed: skip the `curl` + `apt-get install nodejs` steps, only install the npm package
    - Keep the `apt-get install curl ca-certificates git` step (idempotent, needed regardless)
  - [x] 6.3 Update Augment Code `Install()` to use the tracking
    - Check `b.IsNodeInstalled()` before appending Node.js install steps
    - If not installed: install Node.js 22 via `setup_22.x`, then call `b.MarkNodeInstalled()`
    - If already installed: skip the `curl` + `apt-get install nodejs` steps, only install the npm package
    - Keep the `apt-get install curl ca-certificates git` step (idempotent, needed regardless)
  - [x] 6.4 Update unit tests for both agents
    - Test that when `IsNodeInstalled()` returns false, the agent appends Node.js install steps
    - Test that when `IsNodeInstalled()` returns true, the agent skips Node.js install steps but still installs its npm package
    - Test that `MarkNodeInstalled()` is called after Node.js installation
  - [x] 6.5 Verify build and tests pass
    - Run `go build ./...` — must compile cleanly
    - Run `go vet ./...` — must pass
    - Run `go test ./...` — all unit and PBT tests must pass

- [x] 1. Add `EnsureBaseImageAbsent()` to `internal/testutil/consent.go`
  - [x] 1.1 Add `EnsureBaseImageAbsent()` function (gated by `//go:build integration`)
    - Connect to Docker via `docker.NewClient()`
    - Inspect `constants.BaseContainerImage`; if not present, return nil (nothing to do)
    - If present, remove it with `client.ImageRemove(ctx, id, force=true)`
    - Return error only if the removal itself fails
    - Print a short notice to stderr when removing: `"Removing cached base image %s to ensure clean test environment\n"`

- [x] 2. Update `internal/docker/integration_test.go`
  - [x] 2.1 Replace the manual base-image inspect-and-fail block in `TestMain` with `testutil.EnsureBaseImageAbsent()`
    - Remove the `ctx`, `client`, `ImageInspectWithRaw`, and the `INTEGRATION TEST ENVIRONMENT ERROR` block
    - Call `testutil.EnsureBaseImageAbsent()` after `testutil.RequireIntegrationConsent()`; if it returns an error, print it to stderr and `os.Exit(1)`
  - [x] 2.2 Simplify `TestAFindConflictingUserPullsImageIfAbsent`
    - Remove the backup tag (`bac-test-backup:findconflict`), the `tagImage` calls, the manual image removal loop, and the `t.Cleanup` restore block
    - The test body becomes: call `FindConflictingUser(ctx, client, uid, gid)`, assert no error, assert the image is now present locally
    - Remove the `tagImage` helper function if no other test uses it

- [x] 3. Update `internal/agents/claude/integration_test.go`
  - [x] 3.1 Add `testutil.EnsureBaseImageAbsent()` call in `TestMain` after `testutil.RequireIntegrationConsent()`
    - If it returns an error, print to stderr and `os.Exit(1)`

- [x] 4. Update `internal/agents/augment/integration_test.go`
  - [x] 4.1 Add `testutil.EnsureBaseImageAbsent()` call in `TestMain` after `testutil.RequireIntegrationConsent()`
    - If it returns an error, print to stderr and `os.Exit(1)`

- [x] 5. Verify
  - [x] 5.1 Run `go build ./...` — must compile cleanly
  - [x] 5.2 Run `go vet -tags integration ./...` — must pass with no warnings
  - [x] 5.3 Run `go test ./...` (unit + PBT only) — must pass (no regressions)

### Headless keyring for credential persistence (CC-7)

Install D-Bus and gnome-keyring-daemon in the base container image so that tools using libsecret (Claude Code, VS Code extensions) can store and refresh OAuth tokens without a graphical desktop.

- [x] 7. Add headless keyring support to the base container image
  - [x] 7.1 Add keyring constant to `internal/constants/constants.go`
    - Add `KeyringProfileScript = "/etc/profile.d/dbus-keyring.sh"` constant
  - [x] 7.2 Update `DockerfileBuilder` in `internal/docker/builder.go` to install keyring packages and startup script
    - After the `mkdir -p /run/sshd` step, add a RUN step that installs `dbus-x11`, `gnome-keyring`, and `libsecret-1-0`
    - Add a RUN step that creates `/etc/profile.d/dbus-keyring.sh` with the startup script content
    - The script must: start dbus-launch if `DBUS_SESSION_BUS_ADDRESS` is unset, export it, then unlock gnome-keyring-daemon with an empty password
    - Make the script executable (chmod +x)
  - [x] 7.3 Add unit/PBT tests for keyring in `internal/docker/builder_test.go`
    - Property 52: Verify the generated Dockerfile contains `dbus-x11`, `gnome-keyring`, and `libsecret-1-0` installation for any UID/GID
    - Property 53: Verify the generated Dockerfile contains the profile.d script creation at `constants.KeyringProfileScript` with `dbus-launch`, `gnome-keyring-daemon --unlock`, and `chmod +x`
    - Unit test: Verify keyring is present in UserStrategyRename as well
  - [x] 7.4 Verify build and tests pass
    - Run `go build ./...` — must compile cleanly
    - Run `go vet ./...` — must pass
    - Run `go test ./...` — all unit and PBT tests must pass

### Semantic refactoring (R1, R3, R4, R6, R7, R8)

Internal code quality improvements: consolidate duplicated helpers, fix misplaced responsibilities, clarify intent. No user-facing behaviour changes.

- [x] 8. Create `internal/pathutil` package and consolidate `expandHome` (R1)
  - [x] 8.1 Create `internal/pathutil/pathutil.go` with `ExpandHome` function
  - [x] 8.2 Create `internal/pathutil/pathutil_test.go` with property test (ExpandHome never returns ~/prefix) and unit tests
  - [x] 8.3 Update `internal/naming/naming.go` — remove local `expandHome`, import `pathutil`
  - [x] 8.4 Update `internal/ssh/keys.go` — remove local `expandHome`, import `pathutil`
  - [x] 8.5 Update `internal/credentials/store.go` — remove local `expandHome`, import `pathutil`
  - [x] 8.6 Update `internal/datadir/datadir.go` — remove local `expandHome`, import `pathutil`
  - [x] 8.7 Update `internal/cmd/root.go` — remove both `expandHome` and `ExpandHome`, import `pathutil`
  - [x] 8.8 Update `internal/cmd/root_test.go` — change `cmd.ExpandHome` references to `pathutil.ExpandHome`
  - [x] 8.9 Run `go build ./...` and `go test ./...` to verify no regressions
- [x] 9. Update `ExecInContainer` to accept `*Client` parameter (R3)
  - [x] 9.1 Change `Agent.HealthCheck` interface in `internal/agent/agent.go` to accept `*docker.Client`
  - [x] 9.2 Update `internal/docker/runner.go` `ExecInContainer` signature to accept `*Client`
  - [x] 9.3 Update `internal/agents/claude/claude.go` `HealthCheck` to match new interface
  - [x] 9.4 Update `internal/agents/augment/augment.go` `HealthCheck` to match new interface
  - [x] 9.5 Update `internal/agent/registry_test.go` stub to match new interface
  - [x] 9.6 Update any integration tests that call `HealthCheck` or `ExecInContainer`
  - [x] 9.7 Run `go build ./...` and `go test ./...` to verify no regressions
- [x] 10. Consolidate inline flag validation in `cmd/root.go` (R4)
  - [x] 10.1 Replace 7 individual `cmd.Flags().Changed(...)` blocks with `cmd.Flags().Visit` + `ValidateStartOnlyFlags` call
  - [x] 10.2 Remove private `stringSlicesEqual` wrapper (use `StringSlicesEqual` directly)
  - [x] 10.3 Run `go build ./...` and `go test ./...` to verify no regressions
- [x] 11. Split `ListBACImages` into explicit and fallback variants (R6)
  - [x] 11.1 Refactor `ListBACImages` in `internal/docker/runner.go` to return only labeled images
  - [x] 11.2 Create `ListBACImagesWithFallback` with the tag-prefix scan logic and doc comment
  - [x] 11.3 Update `runPurge` in `cmd/root.go` to call `ListBACImagesWithFallback`
  - [x] 11.4 Run `go build ./...` and `go test ./...` to verify no regressions
- [x] 12. Extract `HostBindIP` constant (R7)
  - [x] 12.1 Add `HostBindIP = "127.0.0.1"` to `internal/constants/constants.go`
  - [x] 12.2 Update `CreateContainer` in `internal/docker/runner.go` to use `constants.HostBindIP`
  - [x] 12.3 Update `WaitForSSH` call in `internal/cmd/root.go` to use `constants.HostBindIP`
  - [x] 12.4 Run `go build ./...` and `go test ./...` to verify no regressions
- [x] 13. Move `CredentialPreparer` to its own file (R8)
  - [x] 13.1 Create `internal/agent/preparer.go` with the `CredentialPreparer` interface
  - [x] 13.2 Remove `CredentialPreparer` from `internal/agent/agent.go`
  - [x] 13.3 Run `go build ./...` and `go test ./...` to verify no regressions
