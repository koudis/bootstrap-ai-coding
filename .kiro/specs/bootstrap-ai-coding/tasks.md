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

### VS Code Server Persistence Volume (Req 22)

Implement a named Docker volume mounted at `/home/dev/.vscode-server` to persist VS Code Remote-SSH server binaries across container restarts and rebuilds.

- [x] 7. Implement VS Code Server persistence volume
  - [x] 7.1 Add constants to `internal/constants/constants.go`
    - Add `VSCodeServerPath = ContainerUserHome + "/.vscode-server"`
    - Add `VSCodeServerVolumeSuffix = "-vscode-server"`
  - [x] 7.2 Add `VolumeMount` type and `Volumes` field to `ContainerSpec` in `internal/docker/runner.go`
    - Add `VolumeMount` struct with `Name string` and `ContainerPath string` fields
    - Add `Volumes []VolumeMount` field to `ContainerSpec`
    - Update `CreateContainer` to iterate `spec.Volumes` and append `mount.Mount{Type: mount.TypeVolume, Source: v.Name, Target: v.ContainerPath}` to the mounts slice
  - [x] 7.3 Add `RemoveBACVolumes` helper to `internal/docker/runner.go`
    - List all Docker volumes, filter those whose name starts with `constants.ContainerNamePrefix` and ends with `constants.VSCodeServerVolumeSuffix`
    - Remove each matching volume
    - Return the list of removed volume names
  - [x] 7.4 Update `runStart` in `internal/cmd/root.go` to attach the volume
    - After assembling bind mounts, create a `[]dockerpkg.VolumeMount` with one entry: `{Name: containerName + constants.VSCodeServerVolumeSuffix, ContainerPath: constants.VSCodeServerPath}`
    - Pass it in the `ContainerSpec.Volumes` field when creating the container
  - [x] 7.5 Update `runPurge` in `internal/cmd/root.go` to remove volumes
    - Call `dockerpkg.RemoveBACVolumes(ctx, c)` after removing containers and images
    - Include removed volume names in the purge confirmation summary
  - [x] 7.6 Add unit tests
    - Test that `CreateContainer` includes a volume mount of type `TypeVolume` when `Volumes` is populated
    - Test that `RemoveBACVolumes` correctly filters volumes by prefix and suffix
  - [x] 7.7 Verify build and tests pass
    - Run `go build ./...` — must compile cleanly
    - Run `go vet ./...` — must pass
    - Run `go test ./...` — all unit and PBT tests must pass
