# Implementation Plan: EnsureBaseImageAbsent integration test precondition

## Overview

Extract the base-image-absent precondition into a shared `testutil.EnsureBaseImageAbsent()` helper and call it from `TestMain` in every integration test package. This replaces the manual inspect-and-fail block in `internal/docker` and adds the check to the claude and augment packages. The `TestAFindConflictingUserPullsImageIfAbsent` test is simplified to remove its backup/restore cycle since the image is guaranteed absent by `TestMain`.

## Tasks

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
