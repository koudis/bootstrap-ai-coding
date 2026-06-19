# Implementation Plan: Build Resources — Add tree, btop, and graphify

## Overview

Update the Build Resources agent module to install three new tools: `tree` (directory listing), `btop` (terminal resource monitor), and `graphify` (knowledge graph skill for AI coding assistants). This involves modifying the apt packages list, adding new RUN steps for graphify, extending health checks, and updating both unit and integration tests.

## Tasks

- [x] 1. Add tree, btop, and graphify installation steps to buildresources.go
  - [x] 1.1 Add "tree" and "btop" to the aptPackages slice
    - Add `"tree"` and `"btop"` to the `aptPackages` variable in `internal/agents/buildresources/buildresources.go`
    - Place them in a new comment group `// Terminal and directory utilities` or alongside existing terminal utilities
    - _Requirements: BR-2 AC-9_

  - [x] 1.2 Add graphify installation RUN steps
    - After the uv installation `b.Run(...)` call, add: `b.Run("UV_TOOL_BIN_DIR=/usr/local/bin uv tool install graphifyy")`
    - After the graphify tool install, add: `b.Run("graphify install")`
    - _Requirements: BR-2 AC-10_

  - [x] 1.3 Add health checks for graphify, tree, and btop
    - Add three new entries to the `checks` slice in `HealthCheck()`:
      - `{[]string{"graphify", "--version"}, "graphify"}`
      - `{[]string{"tree", "--version"}, "tree"}`
      - `{[]string{"btop", "--version"}, "btop"}`
    - _Requirements: BR-4 AC-1_

- [x] 2. Update unit tests to verify new packages and install steps
  - [x] 2.1 Update TestInstallAppendsExpectedPackages to include new packages
    - Add `"tree"` and `"btop"` to the `expectedPackages` slice in `TestInstallAppendsExpectedPackages`
    - Add assertions verifying the graphify install commands appear in the generated Dockerfile:
      - `require.Contains(t, content, "UV_TOOL_BIN_DIR=/usr/local/bin uv tool install graphifyy")`
      - `require.Contains(t, content, "graphify install")`
    - _Requirements: BR-2 AC-9, BR-2 AC-10_

- [x] 3. Checkpoint - Ensure unit tests pass
  - Ensure all tests pass with `go test ./internal/agents/buildresources/...`, ask the user if questions arise.

- [x] 4. Update integration tests to verify new tools are available
  - [x] 4.1 Add integration test for tree availability
    - Add `TestTreeAvailable` in `integration_test.go` following existing patterns (exec `tree --version` in the shared container, assert exit code 0)
    - Add comment `// Validates: BR-2.9`
    - _Requirements: BR-2 AC-9_

  - [x] 4.2 Add integration test for btop availability
    - Add `TestBtopAvailable` in `integration_test.go` following existing patterns (exec `btop --version` in the shared container, assert exit code 0)
    - Add comment `// Validates: BR-2.9`
    - _Requirements: BR-2 AC-9_

  - [x] 4.3 Add integration test for graphify availability
    - Add `TestGraphifyAvailable` in `integration_test.go` following existing patterns (exec `graphify --version` in the shared container, assert exit code 0)
    - Add comment `// Validates: BR-2.10`
    - _Requirements: BR-2 AC-10_

- [x] 5. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass with `go test ./internal/agents/buildresources/...`, ask the user if questions arise.
  - Integration tests can be run with `BAC_INTEGRATION_CONSENT=yes go test -tags integration -timeout 30m -p 1 ./internal/agents/buildresources/...`

## Notes

- The design specifies Go as the implementation language — all code is in Go.
- No property-based tests are needed for this change — the additions are deterministic install steps verified by example-based unit tests and integration tests.
- The existing `TestBuildResourcesHealthCheck` integration test will implicitly validate the new health check entries once the implementation is in place.
- The graphify package name on PyPI is `graphifyy` (note the double-y), as specified in the design document.
- `graphify install` must run after `uv tool install graphifyy` since it depends on the graphify executable being available.
- Each task references specific requirements for traceability.
- Checkpoints ensure incremental validation.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2"] },
    { "id": 1, "tasks": ["1.3", "2.1"] },
    { "id": 2, "tasks": ["4.1", "4.2", "4.3"] }
  ]
}
```
