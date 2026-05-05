# Testing Guide

## Test Types

### Unit Tests (example-based)
- No build tag — run with `go test ./...`
- Cover specific scenarios, error conditions, and edge cases
- Use `testify/require` for assertions
- Mock Docker interactions using the `docker.Client` interface

### Property-Based Tests (PBT)
- No build tag — run with `go test ./...`
- Use `pgregory.net/rapid` library
- Minimum **100 iterations** per property
- Focus on pure functions: naming, Dockerfile generation, port finding, path resolution, flag parsing

### Integration Tests
- Gated by `//go:build integration`
- Run with `go test -tags integration -timeout 30m ./...`
- Require a live Docker daemon
- Cover the full happy path, SSH connectivity, volume sync, credential persistence

#### Consent gate

Every integration test package has a `TestMain` that prompts for explicit consent before running, because the tests interact with the local Docker daemon and may pull, build, delete, and update Docker images and containers.

When `BAC_INTEGRATION_CONSENT` is **not** set to `yes`, the suite prints a warning and aborts:

```
WARNING: Integration tests interact with the local Docker daemon.
They may pull, build, delete, and update Docker images and containers.

To run these tests, set the environment variable:
  BAC_INTEGRATION_CONSENT=yes go test -tags integration ./...

Aborted — no consent given.
```

**To run integration tests:**

```bash
BAC_INTEGRATION_CONSENT=yes go test -tags integration -timeout 30m ./...
```

#### Base image precondition: automatic removal

Every integration test package calls `testutil.EnsureBaseImageAbsent()` in `TestMain` (after the consent gate). This helper removes `constants.BaseContainerImage` from the local Docker store if present, so the suite always starts from a clean slate.

- The first test that builds a container triggers a fresh pull of the base image
- All subsequent tests in the suite reuse the now-cached image
- No manual `docker rmi` step is needed before running tests

In `internal/docker`, `TestAFindConflictingUserPullsImageIfAbsent` (named with `A` prefix so it runs first alphabetically) specifically validates the auto-pull path: it calls `FindConflictingUser` when the image is absent and asserts the function succeeds.

**Running integration tests:**

```bash
BAC_INTEGRATION_CONSENT=yes go test -tags integration -timeout 30m ./...
```

---

## Property-Based Test Format

Every property test must have a tag comment on the line immediately above the function:

```go
// Feature: bootstrap-ai-coding, Property N: <property description>
func TestPropertyName(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        // draw inputs
        // exercise the function
        // assert the property holds
    })
}
```

The property number `N` must match the property number in `design.md`.

---

## Writing a Property Test

```go
import (
    "testing"
    "pgregory.net/rapid"
    "github.com/stretchr/testify/require"
    "github.com/koudis/bootstrap-ai-coding/internal/constants"
    "github.com/koudis/bootstrap-ai-coding/internal/naming"
)

// Feature: bootstrap-ai-coding, Property 12: Container naming is deterministic and collision-resistant
func TestContainerNameDeterminism(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        path := rapid.StringMatching(`/[a-z/]+`).Draw(t, "path")
        name1, err1 := naming.ContainerName(path)
        name2, err2 := naming.ContainerName(path)
        require.NoError(t, err1)
        require.NoError(t, err2)
        require.Equal(t, name1, name2, "same input must produce same name")
    })
}
```

### Useful rapid generators

```go
rapid.StringMatching(`/[a-zA-Z0-9_/.-]+`)  // absolute path
rapid.IntRange(1000, 65000)                  // UID/GID
rapid.IntRange(1024, 65535)                  // port number
rapid.IntRange(0, 30)                        // version major
rapid.SliceOfN(rapid.String(), 1, 5)         // non-empty slice
rapid.Bool()                                 // true/false
rapid.OneOf(rapid.Just(""), rapid.String())  // optional string
```

---

## What to Test with PBT vs Unit Tests

| Scenario | Test type |
|---|---|
| Container name is deterministic for any path | PBT |
| Container name has correct prefix and length | PBT |
| `/workspace` mount always present for any project path | PBT |
| Dockerfile always starts with `constants.BaseContainerImage` | PBT |
| Dev user UID/GID always match host for any UID/GID | PBT |
| Port finder returns first free port ≥ `constants.SSHPortStart` | PBT |
| Port round-trips through datadir correctly | PBT |
| `--agents` flag parses any comma-separated list correctly | PBT |
| Session summary always contains all required fields | PBT |
| No args → usage on stderr, exit 1 | Unit |
| Docker daemon unreachable → error on stderr, exit 1 | Unit |
| Root UID → error on stderr, exit 1 | Unit |
| Unknown agent ID → error on stderr, exit 1 | Unit |
| Manifest mismatch → instruct rebuild, exit 0 | Unit |
| `--stop-and-remove` with no container → message, exit 0 | Unit |
| `--purge` declined → nothing deleted, exit 0 | Unit |

---

## Integration Test Conventions

```go
//go:build integration

package docker_test

import (
    "testing"
    "github.com/stretchr/testify/require"
)

func TestContainerStartsAndSSHConnects(t *testing.T) {
    // setup: create a temp project dir
    // run: start container
    // assert: SSH connection succeeds
    // teardown: --stop-and-remove
}
```

- Always clean up containers in `t.Cleanup()` or `defer`
- Use `t.TempDir()` for temporary project directories
- Skip gracefully if Docker is not available:
  ```go
  if _, err := exec.LookPath("docker"); err != nil {
      t.Skip("docker not available")
  }
  ```

---

## Coverage Target

≥ 80% line coverage on all non-integration packages:
- `internal/constants/`, `internal/naming/`, `internal/ssh/`, `internal/credentials/`, `internal/datadir/`, `internal/portfinder/`, `internal/agent/`, `internal/docker/builder.go`, `internal/agents/claude/`

Check coverage:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```
