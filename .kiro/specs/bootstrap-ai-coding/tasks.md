# Tasks: Two-Layer Image Architecture

> Implements `requirements-two-layer-image.md` (TL-1 through TL-11) and the "Two-Layer Image Architecture" section of `design-architecture.md`.

## Task 1: Add `BaseImageName` constant (TL-11)

- [x] Add `BaseImageName = "bac-base"` to `internal/constants/constants.go`
- [x] Add `BaseImageTag = BaseImageName + ":latest"` derived constant (or compute inline — decide based on Go const rules since string concat is allowed)

## Task 2: Split `DockerfileBuilder` into base and instance builders (TL-1, TL-2)

- [x] Create `NewBaseImageBuilder(info *hostinfo.Info, strategy UserStrategy, conflictingUser string, gitConfig string) *DockerfileBuilder` in `internal/docker/builder.go`
  - FROM constants.BaseContainerImage
  - openssh-server + sudo
  - Container_User (create or rename)
  - sudoers
  - D-Bus + gnome-keyring + profile script
  - gitconfig (if non-empty)
  - Does NOT add SSH host keys, authorized_keys, sshd hardening, /run/sshd, or CMD
- [x] Create `NewInstanceImageBuilder(info *hostinfo.Info, publicKey, hostKeyPriv, hostKeyPub string) *DockerfileBuilder` in `internal/docker/builder.go`
  - FROM bac-base:latest (use `constants.BaseImageName + ":latest"`)
  - SSH host key injection
  - authorized_keys
  - sshd_config hardening
  - mkdir /run/sshd
  - CMD via Finalize()
- [x] Keep `NewDockerfileBuilder` temporarily (or remove and update all callers in one go — see Task 4)

## Task 3: Add `determineBuilds` function (TL-3, TL-4, TL-8)

- [x] Create `func determineBuilds(ctx, c, enabledIDs, containerName, rebuild) (needBase, needInstance bool, err error)` in `internal/docker/` (or `internal/cmd/`)
  - If `rebuild` → return true, true
  - Inspect `bac-base:latest` — if absent → true, true
  - Check `bac.manifest` label — if absent/invalid JSON → true, true
  - If manifest mismatch → return sentinel `ErrManifestMismatch`
  - Inspect `<containerName>:latest` — if absent → false, true
  - Otherwise → false, false
- [x] Define `var ErrManifestMismatch = errors.New("agent configuration changed")` sentinel

## Task 4: Refactor `runStart` to use two-layer build (TL-1 through TL-5, TL-10)

- [x] Replace the single `needBuild` logic with a call to `determineBuilds`
- [x] Handle `ErrManifestMismatch` — print message, exit 0 (existing UX)
- [x] If `needBase`:
  - Run UID/GID conflict check (existing code, unchanged)
  - Call `NewBaseImageBuilder` + agent `Install()` loops + manifest RUN
  - Build with `BuildImage(ctx, c, baseSpec, flagVerbose)` — print "Building base image..."
  - Tag as `bac-base:latest`, labels: `bac.managed=true`, `bac.manifest=<json>`
  - If `--rebuild`, use `NoCache: true`
- [x] If `needInstance`:
  - Call `NewInstanceImageBuilder` + `Finalize()`
  - Build with `BuildImage(ctx, c, instanceSpec, flagVerbose)` — print "Building instance image..."
  - Tag as `<containerName>:latest`, labels: `bac.managed=true`, `bac.container=<name>`
- [x] Remove old `NewDockerfileBuilder` call and single-image build path

## Task 5: Update `--rebuild` to rebuild both layers (TL-5)

- [x] Ensure `determineBuilds` returns `(true, true, nil)` when `rebuild == true`
- [x] Ensure base build uses `NoCache: true`
- [x] Instance build does NOT need `NoCache` (it inherits fresh base via FROM)
- [x] Existing container stop/remove logic before rebuild remains unchanged

## Task 6: Verify `--stop-and-remove` does not touch images (TL-6)

- [x] Confirm `runStop` does not call `ImageRemove` — no code change expected, just verify
- [x] Add a unit test asserting no image removal happens during stop-and-remove

## Task 7: Update `--purge` to remove base image (TL-7)

- [x] `ListBACImagesWithFallback` already finds images by `bac.managed` label — `bac-base:latest` will have this label, so it should be picked up automatically
- [x] Verify with a test that purge removes both `bac-base:latest` and instance images
- [x] No code change expected if labels are set correctly in Task 4

## Task 8: Unit tests for builder split (TL-1, TL-2)

- [x] Test `NewBaseImageBuilder` output:
  - Starts with `FROM ubuntu:26.04`
  - Contains useradd/usermod
  - Contains gnome-keyring
  - Does NOT contain SSH host key, authorized_keys, sshd_config, CMD
- [x] Test `NewInstanceImageBuilder` output:
  - Starts with `FROM bac-base:latest`
  - Contains SSH host key injection
  - Contains authorized_keys
  - Contains sshd_config hardening
  - Ends with CMD after Finalize()
- [x] Test gitconfig skip when empty string passed

## Task 9: Unit tests for `determineBuilds` (TL-3, TL-4, TL-8)

- [x] Test: rebuild=true → (true, true, nil)
- [x] Test: base absent → (true, true, nil)
- [x] Test: base present, no label → (true, true, nil)
- [x] Test: base present, invalid JSON label → (true, true, nil)
- [x] Test: base present, manifest mismatch → ErrManifestMismatch
- [x] Test: base present, manifest match, instance absent → (false, true, nil)
- [x] Test: base present, manifest match, instance present → (false, false, nil)

## Task 10: Property-based tests (TL-1, TL-2, TL-11)

- [x] Property: Base image Dockerfile always starts with `FROM constants.BaseContainerImage` for any valid hostinfo
- [x] Property: Instance image Dockerfile always starts with `FROM bac-base:latest` for any valid inputs
- [x] Property: Base image Dockerfile never contains `CMD` or SSH host key content
- [x] Property: Instance image Dockerfile always ends with CMD after Finalize()
- [x] Property: `constants.BaseImageName + ":latest"` equals `"bac-base:latest"`

## Task 11: Integration test — full two-layer build cycle

- [x] Build base image, verify it exists with correct labels
- [x] Build instance image FROM base, verify it exists with correct labels
- [x] Start container from instance image, verify SSH connectivity
- [x] Stop and remove container — verify both images still exist
- [x] Rebuild (--rebuild equivalent) — verify both images are recreated

## Task Dependency Graph

```
Task 1 (constant)
  └─► Task 2 (builder split)
        └─► Task 3 (determineBuilds)
              └─► Task 4 (runStart refactor)
                    ├─► Task 5 (--rebuild)
                    ├─► Task 6 (--stop-and-remove verify)
                    └─► Task 7 (--purge verify)
Task 2 ─► Task 8 (unit tests for builders)
Task 3 ─► Task 9 (unit tests for determineBuilds)
Task 2 ─► Task 10 (property tests)
Task 4 ─► Task 11 (integration test)
```
