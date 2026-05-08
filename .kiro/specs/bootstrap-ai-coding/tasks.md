# Tasks: Build Resources Agent

## Task 1: Add `RunAsUser` method to DockerfileBuilder

- [x] Add `RunAsUser(cmd string)` method to `internal/docker/builder.go`
  - [x] Emit `USER <username>` (from `b.info.Username`)
  - [x] Emit `RUN <cmd>`
  - [x] Emit `USER root` to restore root context for subsequent instructions
- [x] Add unit test in `internal/docker/builder_test.go` verifying `RunAsUser` emits correct USER/RUN/USER sequence

**File:** `internal/docker/builder.go`, `internal/docker/builder_test.go`
**Validates:** Design section "DockerfileBuilder Extension: RunAsUser"

---

## Task 2: Implement the `buildresources` agent package

- [x] Create `internal/agents/buildresources/buildresources.go`
  - [x] Define `buildResourcesAgent` struct
  - [x] Implement `init()` calling `agent.Register(&buildResourcesAgent{})`
  - [x] Implement `ID()` returning `constants.BuildResourcesAgentName`
  - [x] Implement `Install(b *docker.DockerfileBuilder)`:
    - [x] Define local `aptPackages []string` slice listing all apt packages (grouped by category: Python, C/C++, Java, common deps, utilities)
    - [x] Single `apt-get install` using `strings.Join(aptPackages, " ")`
    - [x] Go tarball download and extraction to `/usr/local/go` (architecture-aware via `dpkg --print-architecture`)
    - [x] `/etc/profile.d/golang.sh` for system-wide Go PATH
    - [x] `b.RunAsUser(...)` for uv installation via official installer
    - [x] `b.RunAsUser(...)` for `~/.bashrc` PATH entry (`$HOME/.local/bin`)
  - [x] Implement `CredentialStorePath()` returning `""`
  - [x] Implement `ContainerMountPath(homeDir string)` returning `""`
  - [x] Implement `HasCredentials(storePath string)` returning `(true, nil)`
  - [x] Implement `HealthCheck(ctx, c, containerID)` checking: `python3 --version`, `bash -lc "uv --version"`, `cmake --version`, `javac -version`, `bash -lc "go version"`

**File:** `internal/agents/buildresources/buildresources.go`
**Validates:** BR-1, BR-2, BR-3, BR-4, BR-5

**Depends on:** Task 1

---

## Task 3: Wire `buildresources` into `main.go`

- [x] Add blank import `_ "github.com/koudis/bootstrap-ai-coding/internal/agents/buildresources"` to `main.go`

**File:** `main.go`
**Validates:** BR-5, BR-6

**Depends on:** Task 2

---

## Task 4: Unit tests for `buildresources` agent

- [x] Create `internal/agents/buildresources/buildresources_test.go`
  - [x] Test `ID()` returns `"build-resources"`
  - [x] Test `CredentialStorePath()` returns `""`
  - [x] Test `ContainerMountPath("")` returns `""`
  - [x] Test `HasCredentials("")` returns `(true, nil)`
  - [x] Test `Install()` appends expected RUN lines (python3, cmake, build-essential, default-jdk, go tarball, uv)
  - [x] Test `Install()` uses `RunAsUser` for uv steps (verify USER directives in output)

**File:** `internal/agents/buildresources/buildresources_test.go`
**Validates:** BR-1, BR-2, BR-3

**Depends on:** Task 2

---

## Task 5: Update existing tests that depend on `DefaultAgents`

- [x] Check `internal/cmd/root_test.go` for tests that assert on default agent list — update expected values to include `"build-resources"`
- [x] Check `internal/agent/registry_test.go` for any hardcoded agent count assertions — update if needed
- [x] Run `go test ./...` and fix any failures caused by the new default agent

**File:** `internal/cmd/root_test.go`, `internal/agent/registry_test.go`
**Validates:** BR-6

**Depends on:** Task 3

---

## Task 6: Integration test for `buildresources` agent

- [x] Create `internal/agents/buildresources/integration_test.go`
  - [x] Gate with `//go:build integration`
  - [x] Add `TestMain` with consent gate and `EnsureBaseImageAbsent()`
  - [x] Test that container built with `build-resources` has all tools available:
    - [x] `python3 --version` exits 0
    - [x] `bash -lc "uv --version"` exits 0 (as Container_User)
    - [x] `cmake --version` exits 0
    - [x] `javac -version` exits 0
    - [x] `bash -lc "go version"` exits 0
  - [x] Clean up container in `t.Cleanup()`

**File:** `internal/agents/buildresources/integration_test.go`
**Validates:** BR-2, BR-4

**Depends on:** Task 3

---

## Task Dependency Graph

```
Task 1 (RunAsUser)
    └── Task 2 (buildresources package)
            ├── Task 3 (wire main.go)
            │       ├── Task 5 (update existing tests)
            │       └── Task 6 (integration test)
            └── Task 4 (unit tests)
```
