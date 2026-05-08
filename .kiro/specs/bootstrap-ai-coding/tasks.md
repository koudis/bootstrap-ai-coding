# Tasks — Module Consolidation (Req 28)

Merge `internal/credentials` and `internal/portfinder` into `internal/datadir`.

## 1. Merge credentials into datadir

- [x] 1.1 Create `internal/datadir/credentials.go` with `ResolveCredentialPath` and `EnsureCredentialDir` functions (same logic as `credentials.Resolve` and `credentials.EnsureDir`)
- [x] 1.2 Create `internal/datadir/credentials_test.go` — move all tests from `internal/credentials/store_test.go`, updating import paths from `credentials` to `datadir`
- [x] 1.3 Update `internal/cmd/root.go` — replace `credentials.Resolve` with `datadir.ResolveCredentialPath` and `credentials.EnsureDir` with `datadir.EnsureCredentialDir`; remove the `credentials` import
- [x] 1.4 Delete `internal/credentials/store.go` and `internal/credentials/store_test.go`
- [x] 1.5 Verify build passes: `go build ./...`

## 2. Merge portfinder into datadir

- [x] 2.1 Create `internal/datadir/portfinder.go` with `FindFreePort` and `IsPortFree` functions (same logic as `portfinder.FindFreePort` and `portfinder.IsPortFree`)
- [x] 2.2 Create `internal/datadir/portfinder_test.go` — move all tests from `internal/portfinder/portfinder_test.go`, updating import paths from `portfinder` to `datadir`
- [x] 2.3 Update `internal/cmd/root.go` — replace `portfinder.FindFreePort` with `datadir.FindFreePort` and `portfinder.IsPortFree` with `datadir.IsPortFree`; remove the `portfinder` import
- [x] 2.4 Delete `internal/portfinder/portfinder.go` and `internal/portfinder/portfinder_test.go`
- [x] 2.5 Verify build passes: `go build ./...`

## 3. Run full test suite

- [x] 3.1 Run `go test ./...` and confirm all unit and property-based tests pass
- [x] 3.2 Run `go vet ./...` and confirm no issues

---

# Tasks — Git Configuration Forwarding (Req 24)

Inject the host user's `~/.gitconfig` into the container image at build time as a read-only file.

## 4. Add GitConfigPerm constant

- [x] 4.1 Add `GitConfigPerm = 0o444` to `internal/constants/constants.go` with a comment referencing Req 24
- [x] 4.2 Verify build passes: `go build ./...`

## 5. Update DockerfileBuilder to accept and inject git config

- [x] 5.1 Add `gitConfig string` parameter to `NewDockerfileBuilder` in `internal/docker/builder.go`
- [x] 5.2 After the keyring setup step (step 10), add conditional logic: if `gitConfig != ""`, emit a `RUN` step that writes the content to `<info.HomeDir>/.gitconfig`, sets ownership to `info.Username:info.Username`, and sets permissions to `constants.GitConfigPerm` (`0444`)
- [x] 5.3 Update all existing callers of `NewDockerfileBuilder` to pass the new `gitConfig` parameter (empty string `""` for test helpers and integration tests that don't need git config)

## 6. Update cmd/root.go to read and pass git config

- [x] 6.1 In the image build section of `cmd/root.go`, before calling `NewDockerfileBuilder`, read `filepath.Join(info.HomeDir, ".gitconfig")` using `os.ReadFile`; if the file does not exist or is unreadable, set content to empty string (no error, no warning)
- [x] 6.2 Pass the git config content string to `NewDockerfileBuilder` as the new `gitConfig` parameter

## 7. Unit tests for git config injection

- [x] 7.1 In `internal/docker/builder_test.go`, add a test that passes non-empty git config content and asserts the generated Dockerfile contains a `RUN` line that writes to `<homeDir>/.gitconfig` with `chmod 0444` and correct `chown`
- [x] 7.2 In `internal/docker/builder_test.go`, add a test that passes empty string for git config and asserts no `.gitconfig`-related `RUN` line appears in the generated Dockerfile
- [x] 7.3 In `internal/docker/builder_test.go`, add a test that verifies git config content with special characters (quotes, newlines, backslashes) is correctly escaped in the generated Dockerfile step

## 8. Verify full build and test suite

- [x] 8.1 Run `go build ./...` and confirm no compilation errors
- [x] 8.2 Run `go test ./...` and confirm all unit and property-based tests pass
- [x] 8.3 Run `go vet ./...` and confirm no issues
