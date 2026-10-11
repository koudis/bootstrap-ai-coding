# Kimi Code Agent Requirements

## Overview

Kimi Code is Moonshot AI's open-source terminal AI coding agent. It reads and edits code, executes shell commands, searches files, and plans its next step from user feedback. Unlike the other Node.js-based agents in this project, Kimi Code CLI is distributed as a **self-contained single binary** — no Node.js installation is required.

Authentication uses OAuth or an API key configured via the `/login` command inside the TUI. All credentials and configuration are stored under `~/.kimi-code/` on the host and are bind-mounted into the Container, so a login performed once on the Host persists across sessions and rebuilds without re-authentication.

Kimi Code is **opt-in only** — not included in `constants.DefaultAgents`. Users enable it explicitly via `--agents kimi-code`.

> **Sources:** [Kimi Code GitHub (MoonshotAI/kimi-code)](https://github.com/MoonshotAI/kimi-code), [Kimi Code data locations docs](https://www.kimi.com/code/docs/en/kimi-code-cli/configuration/data-locations.html), [Kimi API Platform integration guide](https://platform.kimi.ai/docs/guide/kimi-code-support). Content was rephrased for compliance with licensing restrictions.

### Glossary

- **Kimi_CLI**: The Kimi Code command-line AI coding agent provided by Moonshot AI, installed via `curl -fsSL https://code.kimi.com/kimi-code/install.sh | bash`. Provides the `kimi` binary. Does not require Node.js.
- **Kimi_Data_Dir**: The host-side directory `~/.kimi-code/` where Kimi Code CLI persists configuration (`config.toml`), OAuth credentials (`credentials/`), session history, logs, and all other runtime data.

---

### Requirement KM-1: Agent Identity

**User Story:** As the core system, I need the Kimi Code module to declare a stable, unique identifier so it can be selected via the `--agents` flag.

#### Acceptance Criteria

1. THE Kimi Code module SHALL declare the Agent_ID `"kimi-code"`.
2. THE Agent_ID SHALL be stable across versions of the module and SHALL NOT change.
3. THE Agent_ID `"kimi-code"` SHALL be unique among all registered agent modules.
4. THE Agent_ID SHALL be defined as `KimiCodeAgentName = "kimi-code"` in `internal/constants/constants.go` and referenced via `constants.KimiCodeAgentName` — no string literal hardcoding elsewhere.

---

### Requirement KM-2: Installation

**User Story:** As a developer, I want the Kimi Code CLI to be pre-installed in the container image so I can run it immediately after connecting via SSH.

#### Acceptance Criteria

1. THE Kimi Code module SHALL contribute Dockerfile steps that install the Kimi Code CLI via the official installer: `curl -fsSL https://code.kimi.com/kimi-code/install.sh | bash`.
2. THE installation SHALL run non-interactively and SHALL NOT trigger any browser-based authentication flow or interactive prompt during image build.
3. WHEN the container image is built with the Kimi Code agent enabled, the `kimi` command SHALL be available on the default `PATH` inside the Container for the Container_User.
4. THE installation SHALL NOT require Node.js, since Kimi Code CLI is distributed as a self-contained binary.
5. IF the Kimi Code installer requires OS packages (e.g. `curl`, `ca-certificates`), THE Kimi Code module SHALL install them as part of its Dockerfile steps.
6. THE installation steps SHALL NOT require any manual intervention after the container starts.

---

### Requirement KM-3: Credential & Configuration Store

**User Story:** As a developer, I want my Kimi Code authentication and configuration to persist across sessions and rebuilds so I only need to authenticate once.

#### Acceptance Criteria

1. THE Kimi Code module SHALL declare `~/.kimi-code` as its Credential_Store path on the host (the Kimi_Data_Dir).
2. THE Kimi Code module SHALL declare `<Container_User_Home>/.kimi-code` as its Credential_Volume mount path inside the Container.
3. THE Credential_Volume SHALL be a bind-mount so that credentials and configuration written inside the Container are immediately persisted to the host Kimi_Data_Dir.
4. IF the host Kimi_Data_Dir does not exist at container start time, THEN THE core SHALL create it with permissions `0700` before mounting.
5. Credentials and configuration persisted in the host Kimi_Data_Dir SHALL be available in future Sessions without re-authentication.
6. THE Credential_Volume bind-mount SHALL be read-write (`ReadOnly: false`) so that the Kimi Code CLI can write session history, logs, cache, and updated credentials into the Kimi_Data_Dir during a session.

---

### Requirement KM-4: Credential Presence Check

**User Story:** As the core system, I need to know whether the user has authenticated Kimi Code so I can inform them if they haven't.

#### Acceptance Criteria

1. THE Kimi Code module SHALL implement a credential presence check that inspects the Kimi_Data_Dir (`~/.kimi-code`) for recognisable Kimi Code authentication state.
2. THE credential presence check SHALL return `true` when the `credentials/` subdirectory exists inside Kimi_Data_Dir and contains at least one non-empty file.
3. THE credential presence check SHALL return `false` with a nil error when the Kimi_Data_Dir is absent, the `credentials/` subdirectory is absent, or no non-empty credential file exists.
4. IF a filesystem error other than file-not-found or directory-not-found occurs while inspecting the Kimi_Data_Dir, THE credential presence check SHALL return `false` and a non-nil error describing the failure.
5. WHEN the credential presence check returns `false`, THE core SHALL print a message instructing the user to run `kimi` inside the Container and complete the `/login` flow.

---

### Requirement KM-5: Readiness Health Check

**User Story:** As the core system, I need to verify that the Kimi Code CLI is correctly installed inside a running container before reporting it as ready.

#### Acceptance Criteria

1. THE Kimi Code module SHALL implement a Health_Check that executes `kimi --version` inside the Container as the Container_User and verifies exit code 0.
2. THE Health_Check SHALL be invoked by the core after the Container starts, running as the Container_User.
3. IF `kimi --version` returns a non-zero exit code, THE Kimi Code module SHALL return a non-nil error that includes the exit code.
4. IF the Health_Check fails, THE core SHALL report the failure with a descriptive error message identifying the Kimi Code agent.

---

### Requirement KM-6: No Core Coupling

**User Story:** As a platform maintainer, I want the Kimi Code module to be fully self-contained so removing or replacing it requires no changes to core code.

#### Acceptance Criteria

1. THE Kimi Code module SHALL NOT be referenced by name or identifier in any source file under `internal/cmd/`, `internal/naming/`, `internal/docker/`, `internal/ssh/`, `internal/datadir/`, `internal/pathutil/`, `internal/hostinfo/`, or `internal/agent/`.
2. THE Kimi Code module SHALL register itself with the Agent_Registry via `init()` without requiring modification of any core source file.
3. THE core application SHALL function correctly if the Kimi Code module is not compiled in.
4. THE only files that change when adding the Kimi Code agent SHALL be `main.go` (blank import) and `internal/constants/constants.go` (`KimiCodeAgentName` constant).
5. THE Agent_ID `"kimi-code"` SHALL NOT appear in `constants.DefaultAgents`. The agent is opt-in only.

---

### Requirement KM-7: Integration Tests

**User Story:** As a maintainer, I want automated integration tests for the Kimi Code agent so I can verify the binary installs and the Health_Check passes.

#### Acceptance Criteria

1. THE Kimi Code agent package SHALL include an integration test file gated by the `//go:build integration` build tag.
2. THE integration test SHALL build a container image with the `kimi-code` agent enabled.
3. THE integration test SHALL verify that the `kimi` binary is present on `PATH` inside the Container.
4. THE integration test SHALL invoke the Kimi Code module's `HealthCheck` method and verify it returns nil.
5. THE integration test SHALL clean up all containers it creates in a `t.Cleanup()` or `defer` block.
6. THE integration test SHALL skip gracefully if Docker is not available on the host running the test.

---

### Requirement KM-8: Session Summary

**User Story:** As a developer, I want any relevant Kimi Code agent information to appear in the session summary.

#### Acceptance Criteria

1. THE Kimi Code module SHALL implement the `SummaryInfo` method of the Agent_Interface.
2. WHEN there is no additional agent-specific information to report, THE `SummaryInfo` method SHALL return `(nil, nil)`.
3. IF the `SummaryInfo` method returns non-nil key:value pairs, THE core SHALL include them in the session summary output following the session summary format defined in core Req 9.

---

### Requirement KM-9: Read-Write Data Directory Pre-Creation

**User Story:** As a developer, I want the container image to pre-create `~/.kimi-code` with correct ownership so that the bind-mounted Kimi_Data_Dir is immediately writable by the Container_User without root-owned intermediate directories blocking writes.

#### Acceptance Criteria

1. THE Kimi Code `Install` method SHALL include a Dockerfile `RUN` step that creates `<Container_User_Home>/.kimi-code` during image build.
2. THE created directory SHALL be owned by the Container_User (username and UID/GID matching the host user), using `chown -R <username>:<username>`.
3. THE pre-creation step SHALL use `mkdir -p` so it is idempotent and does not fail if the directory already exists.
4. THE pre-creation ensures that when the core bind-mounts the host Kimi_Data_Dir at `<Container_User_Home>/.kimi-code`, no intermediate path is created as root, preserving the Container_User's ability to write credentials, session history, logs, and cache into the directory.
5. THE pre-creation step SHALL be the last `RUN` instruction contributed by the Kimi Code `Install` method.

---

## Cross-Cutting Concerns

This agent implements the general agent contract (Req 7 & 8) and the **Agent Summary Info** mechanism (SI-1–SI-7) defined in the [parent index](../requirements-agents.md). Its `SummaryInfo` MAY contribute a key:value line to the session output; returning `(nil, nil)` is acceptable if there is nothing additional to report.
