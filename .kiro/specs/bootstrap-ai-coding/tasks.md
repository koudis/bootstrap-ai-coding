# Tasks: Container Restart Policy (Req 25, CLI-7)

## Task Dependency Graph

```
Task 1 (constants) → Task 2 (ContainerSpec) → Task 3 (runner) → Task 4 (cmd flags) → Task 5 (tests)
```

---

## Task 1: Add `DefaultRestartPolicy` constant

- [x] Add `DefaultRestartPolicy = "unless-stopped"` to `internal/constants/constants.go`

### Files to modify
- `internal/constants/constants.go`

### Acceptance criteria
- `constants.DefaultRestartPolicy` exists and equals `"unless-stopped"`
- No other package hardcodes the default restart policy string

---

## Task 2: Add `RestartPolicy` field to `ContainerSpec`

- [x] Add `RestartPolicy string` field to the `ContainerSpec` struct in `internal/docker/runner.go`

### Files to modify
- `internal/docker/runner.go`

### Acceptance criteria
- `ContainerSpec` has a `RestartPolicy string` field
- Existing code that constructs `ContainerSpec` still compiles (field is zero-value safe)

---

## Task 3: Apply restart policy in `CreateContainer`

- [x] In `CreateContainer` (`internal/docker/runner.go`), set `HostConfig.RestartPolicy` from `spec.RestartPolicy`
- [x] If `spec.RestartPolicy` is empty, default to `constants.DefaultRestartPolicy`

### Files to modify
- `internal/docker/runner.go`

### Acceptance criteria
- Containers created via `CreateContainer` have the Docker restart policy set
- When `RestartPolicy` is empty string, `unless-stopped` is used as fallback
- When `RestartPolicy` is explicitly set, that value is used

---

## Task 4: Add `--docker-restart-policy` flag and validation in CLI

- [x] Add `flagDockerRestartPolicy string` variable
- [x] Register `--docker-restart-policy` flag in `init()` with default `constants.DefaultRestartPolicy`
- [x] Add `"docker-restart-policy"` to the `startOnly` map in `ValidateStartOnlyFlags`
- [x] Add validation: reject values not in `{no, always, unless-stopped, on-failure}`
- [x] Pass the flag value to `ContainerSpec.RestartPolicy` when creating the container in `runStart`

### Files to modify
- `internal/cmd/root.go`

### Acceptance criteria
- `--docker-restart-policy` flag is registered with default `"unless-stopped"`
- Invalid values produce a descriptive error listing valid options (exit 1)
- Flag is rejected in STOP and PURGE modes (CLI-3)
- The value is threaded through to `ContainerSpec.RestartPolicy` in `runStart`

---

## Task 5: Add unit and property-based tests

- [x] Add `TestRestartPolicyDefaultIsUnlessStopped` — verify flag default
- [x] Add `TestRestartPolicyInvalidValueRejected` — verify invalid values produce errors
- [x] Add `TestRestartPolicyFlagWithStopRejected` — verify CLI-3 for STOP mode
- [x] Add `TestRestartPolicyFlagWithPurgeRejected` — verify CLI-3 for PURGE mode
- [x] Add `TestRestartPolicyAppliedToContainerSpec` — verify the value reaches `HostConfig`
- [x] Add property test (Property 55): for any string, validation accepts iff it's in the valid set
- [x] Add property test (Property 56): for any valid policy, ContainerSpec.RestartPolicy matches

### Files to modify
- `internal/cmd/root_test.go`
- `internal/docker/runner_test.go` (or new file `internal/docker/runner_restart_test.go`)

### Acceptance criteria
- All new tests pass with `go test ./...`
- Property tests use `pgregory.net/rapid` with minimum 100 iterations
- Property test tag format: `// Feature: bootstrap-ai-coding, Property N: <description>`
