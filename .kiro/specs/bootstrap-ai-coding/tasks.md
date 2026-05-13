# Implementation Plan: Agent Summary Info

## Overview

Refactor the session summary mechanism so that agent modules contribute their own key:value pairs via a generic `SummaryInfo` method on the `Agent` interface. This removes all Vibe Kanban–specific logic from the core (`internal/cmd/root.go`), restoring the architectural rule that "core has zero knowledge of agents."

## Tasks

- [x] 1. Extend the Agent interface and add KeyValue type
  - [x] 1.1 Add `KeyValue` struct and `SummaryInfo` method to the Agent interface
    - Add `KeyValue` struct with `Key string` and `Value string` fields to `internal/agent/agent.go`
    - Add `SummaryInfo(ctx context.Context, c *docker.Client, containerID string) ([]KeyValue, error)` to the `Agent` interface
    - _Requirements: SI-1.1, SI-1.2, SI-1.3, SI-1.4_

- [x] 2. Implement no-op SummaryInfo in existing agents
  - [x] 2.1 Implement `SummaryInfo` returning `(nil, nil)` in the Claude Code agent
    - Add `SummaryInfo` method to `claudeAgent` in `internal/agents/claude/claude.go`
    - Add necessary imports (`context`, `agent` package reference for `KeyValue`)
    - _Requirements: SI-6.1_

  - [x] 2.2 Implement `SummaryInfo` returning `(nil, nil)` in the Augment Code agent
    - Add `SummaryInfo` method to `augmentAgent` in `internal/agents/augment/augment.go`
    - _Requirements: SI-6.2_

  - [x] 2.3 Implement `SummaryInfo` returning `(nil, nil)` in the Build Resources agent
    - Add `SummaryInfo` method to `buildResourcesAgent` in `internal/agents/buildresources/buildresources.go`
    - _Requirements: SI-6.3_

- [x] 3. Implement SummaryInfo in the Vibe Kanban agent
  - [x] 3.1 Move port discovery logic into `vibekanban.SummaryInfo()`
    - Add `portRegexp` variable to `internal/agents/vibekanban/vibekanban.go`
    - Implement `SummaryInfo` method with the same retry logic (30s deadline, 2s intervals, `ss -tlnp` parsing) currently in `discoverVibeKanbanPort()`
    - Return `[]agent.KeyValue{{Key: "Vibe Kanban", Value: "http://localhost:<port>"}}` on success
    - Return error on timeout
    - Add `"regexp"` and `"strconv"` imports to `vibekanban.go`
    - _Requirements: SI-5.1, SI-5.2, SI-5.3, SI-5.4_

- [x] 4. Checkpoint - Verify compilation
  - Ensure all tests pass (`go build ./...`), ask the user if questions arise.

- [x] 5. Update SessionSummary and FormatSessionSummary
  - [x] 5.1 Replace `VibeKanbanURL` with `AgentInfo` in `SessionSummary` and update `FormatSessionSummary`
    - Remove `VibeKanbanURL string` field from `SessionSummary` struct in `internal/cmd/root.go`
    - Add `AgentInfo []agent.KeyValue` field to `SessionSummary` struct
    - Replace the Vibe Kanban conditional in `FormatSessionSummary` with a generic loop: `fmt.Fprintf(&sb, "%-17s%s\n", kv.Key+":", kv.Value)` for each entry in `AgentInfo`
    - _Requirements: SI-4.1, SI-4.4, SI-7.1, SI-7.2, SI-7.3, SI-7.4_

- [x] 6. Update core collection logic and remove VK-specific code from root.go
  - [x] 6.1 Add generic agent info collection loop and update `printSessionSummary`
    - Add a collection loop in `runStart()` (both reconnect and fresh-start paths) that iterates over `enabledAgents`, calls `SummaryInfo()`, collects `[]agent.KeyValue`, and prints warnings on error
    - Update `printSessionSummary` signature: replace `vibeKanbanURL string` parameter with `agentInfo []agent.KeyValue`
    - Pass collected `agentInfo` to `printSessionSummary` and store in `SessionSummary.AgentInfo`
    - _Requirements: SI-2.1, SI-2.2, SI-2.3, SI-2.4, SI-3.1, SI-3.2, SI-3.3, SI-3.4_

  - [x] 6.2 Remove all Vibe Kanban–specific code from root.go
    - Delete `discoverVibeKanbanPort()` function
    - Delete `portRegexp` package-level variable
    - Remove `constants.VibeKanbanAgentName` reference and the two VK discovery blocks (reconnect path + fresh start path)
    - Remove unused imports (`"regexp"`, `"strconv"`) from `root.go`
    - _Requirements: SI-4.2, SI-4.3_

- [x] 7. Checkpoint - Verify compilation and existing tests
  - Ensure all tests pass (`go test ./...`), ask the user if questions arise.

- [x] 8. Update existing tests
  - [x] 8.1 Update `root_test.go` to use `AgentInfo` instead of `VibeKanbanURL`
    - Update `TestFormatSessionSummaryWithVibeKanban` to use `AgentInfo: []agent.KeyValue{{Key: "Vibe Kanban", Value: "http://localhost:3000"}}`
    - Update `TestFormatSessionSummaryWithoutVibeKanban` to use `AgentInfo: nil`
    - Update `TestPropertySessionSummaryIncludesVibeKanbanURL` to use `AgentInfo` field and assert generic formatting behaviour
    - Import `agent` package in `root_test.go`
    - _Requirements: SI-7.2, SI-7.3, SI-7.4_

  - [x] 8.2 Add unit tests for no-op `SummaryInfo` in agent test files
    - Add `TestSummaryInfoReturnsNil` to `internal/agents/claude/claude_test.go`
    - Add `TestSummaryInfoReturnsNil` to `internal/agents/augment/augment_test.go`
    - Add `TestSummaryInfoReturnsNil` to `internal/agents/buildresources/buildresources_test.go`
    - _Requirements: SI-6.1, SI-6.2, SI-6.3_

- [x] 9. Write property tests for collection logic and formatting
  - [x] 9.1 Write property test: Collection preserves order and excludes errors
    - **Property 1: Collection preserves order and excludes errors**
    - **Validates: Requirements SI-2.2, SI-3.2, SI-3.3**
    - Create a helper function `CollectAgentInfo` (exported for testability) that takes a slice of `([]KeyValue, error)` results and returns the collected `[]KeyValue`
    - Write `TestPropertyCollectionPreservesOrderAndExcludesErrors` in `internal/cmd/root_test.go` using `rapid`
    - Generate random slices of `([]KeyValue, error)` tuples; assert collected output matches expected filtered/ordered result

  - [x] 9.2 Write property test: Session summary formatting includes all agent info after standard fields
    - **Property 2: Session summary formatting includes all agent info after standard fields**
    - **Validates: Requirements SI-2.3, SI-2.4, SI-7.2, SI-7.3, SI-7.4**
    - Write `TestPropertyFormatSessionSummaryAgentInfo` in `internal/cmd/root_test.go` using `rapid`
    - Generate random `SessionSummary` with random `AgentInfo`; assert all keys/values present, after "Enabled agents" line, no extras when empty

  - [x] 9.3 Write property test: Vibe Kanban URL format
    - **Property 3: Vibe Kanban URL format**
    - **Validates: Requirements SI-5.2**
    - Write `TestPropertyVibeKanbanURLFormat` in `internal/agents/vibekanban/vibekanban_test.go` using `rapid`
    - Generate random port in 1–65535; assert URL matches `"http://localhost:<port>"` exactly

- [x] 10. Final checkpoint - Ensure all tests pass
  - Ensure all tests pass (`go test ./...`), ask the user if questions arise.

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation
- Property tests validate universal correctness properties from the design document
- Unit tests validate specific examples and edge cases
- The Go compiler enforces interface compliance — once `SummaryInfo` is added to the interface, all implementations must exist for the project to compile (tasks 1–3 must be done together or in quick succession)

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1"] },
    { "id": 1, "tasks": ["2.1", "2.2", "2.3", "3.1"] },
    { "id": 2, "tasks": ["5.1"] },
    { "id": 3, "tasks": ["6.1", "6.2"] },
    { "id": 4, "tasks": ["8.1", "8.2"] },
    { "id": 5, "tasks": ["9.1", "9.2", "9.3"] }
  ]
}
```
