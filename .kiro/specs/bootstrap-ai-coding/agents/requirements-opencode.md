# OpenCode Agent Requirements

## Overview

OpenCode is an open-source AI coding agent for the terminal, developed by Anomaly (anomalyco). It is distributed as the `opencode-ai` npm package and invoked via the `opencode` command. It requires Node.js 18 or later (the project installs Node.js 22, which satisfies this). Authentication credentials are stored in `~/.local/share/opencode/auth.json` on the Host, and provider configuration lives in `~/.config/opencode/` on the Host. Unlike the default agents, OpenCode is **opt-in only** — users must explicitly include it via `--agents open-code`.

### Glossary

- **OpenCode_CLI**: The OpenCode terminal AI coding agent, distributed as the `opencode-ai` npm package, providing the `opencode` binary. Requires Node.js 18 or later.
- **OpenCode_Auth_Store**: The host-side directory `~/.local/share/opencode/` where OpenCode persists authentication credentials (`auth.json`).
- **OpenCode_Config_Store**: The host-side directory `~/.config/opencode/` where OpenCode persists provider configuration and session data.

---

### Requirement OC-1: Agent Identity

**User Story:** As the core system, I need the OpenCode module to declare a stable, unique identifier so it can be selected via the `--agents` flag.

#### Acceptance Criteria

1. THE OpenCode module SHALL declare the Agent_ID `"open-code"`.
2. THE Agent_ID SHALL be stable across versions of the module and SHALL NOT change.

---

### Requirement OC-2: Installation

**User Story:** As a developer, I want OpenCode to be pre-installed in the container image so I can run it immediately after connecting via SSH.

#### Acceptance Criteria

1. THE OpenCode module SHALL contribute Dockerfile steps that install Node.js 22 as a runtime dependency, using the shared `IsNodeInstalled()`/`MarkNodeInstalled()` deduplication pattern.
2. THE OpenCode module SHALL contribute Dockerfile steps that install the `opencode-ai` npm package globally via `npm install -g --no-fund --no-audit opencode-ai`.
3. WHEN the container image is built with the OpenCode agent enabled, the `opencode` command SHALL be available on the default `PATH` inside the Container for the Container_User.
4. THE installation steps SHALL NOT require any manual intervention or environment variable configuration after the container starts.

---

### Requirement OC-3: Credential Store

**User Story:** As a developer, I want my OpenCode authentication and configuration to persist across sessions so I only need to log in once.

#### Acceptance Criteria

1. THE OpenCode module SHALL declare `~/.local/share/opencode` as its primary Credential_Store path on the Host (OpenCode_Auth_Store).
2. THE OpenCode module SHALL declare `<Container_User_Home>/.local/share/opencode` as the primary Credential_Volume mount path inside the Container.
3. THE OpenCode module SHALL declare `~/.config/opencode` as its secondary Credential_Store path on the Host (OpenCode_Config_Store) via the `AdditionalMounter` interface.
4. THE OpenCode module SHALL declare `<Container_User_Home>/.config/opencode` as the secondary Credential_Volume mount path inside the Container.
5. EACH Credential_Volume SHALL be a bind-mount so that authentication tokens and configuration written inside the Container are immediately persisted to the respective Host directory.
6. Authentication tokens persisted in the Host OpenCode_Auth_Store SHALL be available in future Sessions without re-authentication.
7. Provider configuration persisted in the Host OpenCode_Config_Store SHALL be available in future Sessions without reconfiguration.
8. IF the Host OpenCode_Auth_Store or OpenCode_Config_Store directory does not exist at container start time, THEN THE core SHALL create it with permissions `0700` before mounting.

---

### Requirement OC-4: Credential Presence Check

**User Story:** As the core system, I need to know whether the user has already authenticated OpenCode so I can inform them if they haven't.

#### Acceptance Criteria

1. THE OpenCode module SHALL implement a credential presence check that inspects the OpenCode_Auth_Store directory for the `auth.json` file.
2. THE credential presence check SHALL return `true` when `auth.json` exists in the OpenCode_Auth_Store and has a file size greater than 0 bytes.
3. THE credential presence check SHALL return `false` with a nil error when `auth.json` is absent, has a file size of 0 bytes, or the OpenCode_Auth_Store directory does not exist.
4. IF a filesystem error other than file-not-found or directory-not-found occurs, THE credential presence check SHALL return `false` and a non-nil error describing the failure.
5. WHEN the credential presence check returns `false`, THE core SHALL print a message to stdout instructing the user to run `opencode` inside the Container and complete the authentication flow.

---

### Requirement OC-5: Readiness Health Check

**User Story:** As the core system, I need to verify that the OpenCode CLI is correctly installed inside a running container before reporting it as ready.

#### Acceptance Criteria

1. THE OpenCode module SHALL implement a Health_Check that executes `opencode --version` inside the Container and verifies exit code 0.
2. THE Health_Check SHALL be invoked by the core after the Container starts.
3. IF `opencode --version` returns a non-zero exit code, THE OpenCode module SHALL return an error indicating that the health check failed, including the exit code.
4. IF the Health_Check fails, THE core SHALL report the failure to the user with a descriptive error message identifying the OpenCode agent.

---

### Requirement OC-6: No Core Coupling

**User Story:** As a platform maintainer, I want the OpenCode module to be fully self-contained so that removing or replacing it requires no changes to core code.

#### Acceptance Criteria

1. THE OpenCode module SHALL NOT be referenced by name or identifier anywhere in the core application code.
2. THE OpenCode module SHALL register itself with the Agent_Registry without requiring any modification to core source files.
3. THE core application SHALL function correctly (with no enabled agents) if the OpenCode module is not compiled in.
4. THE string literal `"open-code"` SHALL NOT appear in any source file under `internal/cmd/`, `internal/naming/`, `internal/docker/`, `internal/ssh/`, `internal/datadir/`, `internal/pathutil/`, `internal/hostinfo/`, or `internal/agent/`.

> **Note:** The OpenCode agent is NOT included in `constants.DefaultAgents`. It is opt-in only — users must explicitly pass `--agents open-code` or `--agents claude-code,open-code` to include it.

---

## Cross-Cutting Concerns

This agent implements the general agent contract (Req 7 & 8) and the **Agent Summary Info** mechanism (SI-1–SI-7) defined in the [parent index](../requirements-agents.md).
