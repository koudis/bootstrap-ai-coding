# Agent Summary Info Requirements

## Overview

Agent Summary Info is a mechanism that allows agent modules to contribute their own key:value pairs to the session summary printed by the core after a successful container start. This eliminates the need for the core to contain agent-specific logic (such as port discovery for Vibe Kanban) and restores the architectural rule that "core has zero knowledge of agents."

Each agent module implements a `SummaryInfo` method on the Agent_Interface. The core iterates over enabled agents, calls `SummaryInfo()` on each, and appends any returned key:value pairs to the session summary output. Agents that have nothing to report return nil.

## Glossary

- **Summary_Info**: A slice of key:value pairs returned by an agent's `SummaryInfo()` method, representing additional labelled lines to include in the session summary.
- **KeyValue**: A struct with `Key string` and `Value string` fields, representing a single labelled line in the session summary (e.g. Key=`"Vibe Kanban"`, Value=`"http://localhost:3000"`).

---

## Requirements

### Requirement SI-1: Agent_Interface Extension

**User Story:** As a platform maintainer, I want the Agent_Interface to include a `SummaryInfo` method so that agents can contribute information to the session summary without the core needing agent-specific logic.

#### Acceptance Criteria

1. THE Agent_Interface SHALL include a `SummaryInfo(ctx context.Context, c *docker.Client, containerID string) ([]KeyValue, error)` method.
2. THE `KeyValue` struct SHALL be defined in the `agent` package (`internal/agent/agent.go`) with two exported fields: `Key string` and `Value string`.
3. THE `SummaryInfo` method SHALL receive the same `context.Context`, `*docker.Client`, and `containerID string` parameters as `HealthCheck`, enabling agents to inspect the running container to gather information.
4. THE `SummaryInfo` method SHALL return a slice of `KeyValue` pairs and an error. A nil or empty slice indicates the agent has no information to contribute.

---

### Requirement SI-2: Core Iteration and Collection

**User Story:** As a developer, I want the core to automatically collect summary information from all enabled agents so that the session summary is always complete without hardcoded agent logic.

#### Acceptance Criteria

1. WHEN a Container is successfully started (or is already running), THE core SHALL call `SummaryInfo()` on each Enabled_Agent after health checks pass and before printing the session summary.
2. THE core SHALL iterate over Enabled_Agents in their declared order and collect all returned `KeyValue` pairs into a single ordered slice.
3. THE core SHALL append the collected `KeyValue` pairs to the session summary output after the standard fields (Data directory, Project directory, SSH port, SSH connect, Enabled agents).
4. THE core SHALL format each `KeyValue` pair as a labelled line with consistent alignment matching the existing session summary fields (left-aligned key followed by colon, padded with spaces to align values).

---

### Requirement SI-3: Graceful Error Handling

**User Story:** As a developer, I want the session to start successfully even if an agent's summary info gathering fails, so that a non-critical failure does not block my workflow.

#### Acceptance Criteria

1. IF an agent's `SummaryInfo()` returns a non-nil error, THEN THE core SHALL print a warning to stderr in the format `"warning: <agent-id> summary info: <error message>\n"` and continue processing remaining agents.
2. IF an agent's `SummaryInfo()` returns a non-nil error, THEN THE core SHALL NOT include any `KeyValue` pairs from that agent in the session summary.
3. IF an agent's `SummaryInfo()` returns a nil or empty slice with a nil error, THEN THE core SHALL NOT add any lines to the session summary for that agent.
4. THE overall startup process SHALL NOT fail due to a `SummaryInfo()` error from any agent.

---

### Requirement SI-4: Remove Hardcoded Vibe Kanban Logic from Core

**User Story:** As a platform maintainer, I want all Vibe Kanban–specific logic removed from the core so that the architectural rule "core has zero knowledge of agents" is restored.

#### Acceptance Criteria

1. THE `VibeKanbanURL` field SHALL be removed from the `SessionSummary` struct in `internal/cmd/root.go`.
2. THE `discoverVibeKanbanPort()` function SHALL be removed from `internal/cmd/root.go`.
3. THE `constants.VibeKanbanAgentName` reference SHALL be removed from `internal/cmd/root.go` — the core SHALL NOT reference any agent by name or identifier.
4. THE `FormatSessionSummary` function SHALL NOT contain any conditional logic specific to Vibe Kanban or any other individual agent.

---

### Requirement SI-5: Vibe Kanban SummaryInfo Implementation

**User Story:** As a developer, I want the Vibe Kanban agent to report its URL via the `SummaryInfo` mechanism so that the session summary still shows the Vibe Kanban URL without the core containing Vibe Kanban–specific code.

#### Acceptance Criteria

1. THE Vibe Kanban module SHALL implement `SummaryInfo()` by reading the port file (`/tmp/vibe-kanban.port`) written by the supervisor script after vibe-kanban starts and binds its auto-assigned port.
2. WHEN the Vibe Kanban port is successfully discovered, THE `SummaryInfo()` method SHALL return a single `KeyValue` with Key `"Vibe Kanban"` and Value `"http://localhost:<port>"`.
3. IF the Vibe Kanban port file cannot be read within 30 seconds (retrying every 2 seconds), THEN THE `SummaryInfo()` method SHALL return a non-nil error describing the timeout.
4. THE port discovery logic SHALL reside entirely in the Vibe Kanban agent package (`internal/agents/vibekanban/`) — the core SHALL NOT contain any port discovery code.

---

### Requirement SI-6: Other Agents Return Nil

**User Story:** As a platform maintainer, I want agents that have no summary information to return nil from `SummaryInfo()` so that the core handles them uniformly without special cases.

#### Acceptance Criteria

1. THE Claude Code module SHALL implement `SummaryInfo()` by returning `(nil, nil)`.
2. THE Augment Code module SHALL implement `SummaryInfo()` by returning `(nil, nil)`.
3. THE Build Resources module SHALL implement `SummaryInfo()` by returning `(nil, nil)`.

---

### Requirement SI-7: Session Summary Formatting with Agent Info

**User Story:** As a developer, I want agent-contributed information to appear in the session summary with the same formatting as built-in fields so that the output is consistent and easy to read.

#### Acceptance Criteria

1. THE `SessionSummary` struct SHALL include an `AgentInfo []KeyValue` field (using the `KeyValue` type from the `agent` package, or an equivalent type in the `cmd` package) to hold agent-contributed key:value pairs.
2. THE `FormatSessionSummary` function SHALL format each entry in `AgentInfo` as `"<Key>:<padding><Value>\n"` where `<padding>` is sufficient whitespace to align the value column with the existing session summary fields.
3. THE agent-contributed lines SHALL appear after the "Enabled agents" line and before any trailing newline.
4. WHEN `AgentInfo` is nil or empty, THE `FormatSessionSummary` function SHALL produce output identical to the current format (no extra blank lines or trailing content).
