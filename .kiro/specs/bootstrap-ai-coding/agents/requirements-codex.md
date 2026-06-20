# Codex Agent Requirements

## Overview

Codex is OpenAI's AI coding agent, distributed as the `@openai/codex` npm package and invoked via the `codex` command. It requires Node.js 22 or later. Authentication tokens are stored in `~/.codex/auth.json` on the Host. Unlike the default agents, Codex is **opt-in only** — users must explicitly include it via `--agents codex`.

### Glossary

- **Codex_CLI**: The OpenAI Codex CLI tool, distributed as the `@openai/codex` npm package, providing an AI coding assistant powered by OpenAI models.

---

### Requirement CX-1: Agent Identity

**User Story:** As the core system, I need the Codex module to declare a stable, unique identifier so it can be selected via the `--agents` flag.

#### Acceptance Criteria

1. THE Codex module SHALL declare the Agent_ID `"codex"`.
2. THE Agent_ID SHALL be stable across versions of the module and SHALL NOT change.

---

### Requirement CX-2: Installation

**User Story:** As a developer, I want the Codex CLI to be pre-installed in the container image so I can run it immediately after connecting via SSH.

#### Acceptance Criteria

1. THE Codex module SHALL contribute Dockerfile steps that install Node.js 22 as a runtime dependency, using the shared `IsNodeInstalled()`/`MarkNodeInstalled()` deduplication pattern.
2. THE Codex module SHALL contribute Dockerfile steps that install the `@openai/codex` npm package globally via `npm install -g --no-fund --no-audit @openai/codex`.
3. WHEN the container image is built with the Codex agent enabled, the `codex` command SHALL be available on the default `PATH` inside the Container for the Container_User.
4. THE installation steps SHALL NOT require any manual intervention after the container starts.

---

### Requirement CX-3: Credential Store

**User Story:** As a developer, I want my Codex authentication to persist across sessions so I only need to log in once.

#### Acceptance Criteria

1. THE Codex module SHALL declare `~/.codex` as its default Credential_Store path on the Host.
2. THE Codex module SHALL declare `<Container_User_Home>/.codex` as its Credential_Volume mount path inside the Container.
3. THE Credential_Volume SHALL be a bind-mount so that authentication tokens written inside the Container are immediately persisted to the Host Credential_Store.
4. Authentication tokens persisted in the Host Credential_Store SHALL be available in future Sessions without re-authentication.

---

### Requirement CX-4: Credential Presence Check

**User Story:** As the core system, I need to know whether the user has already authenticated Codex so I can inform them if they haven't.

#### Acceptance Criteria

1. THE Codex module SHALL implement a credential presence check that inspects the Credential_Store directory for the `auth.json` file.
2. THE credential presence check SHALL return `true` when `auth.json` exists in the Credential_Store.
3. THE credential presence check SHALL return `false` when `auth.json` is absent or the Credential_Store directory does not exist.
4. WHEN the credential presence check returns `false`, THE core SHALL print a message to stdout instructing the user to run `codex` inside the Container and complete the login flow.

---

### Requirement CX-5: Readiness Health Check

**User Story:** As the core system, I need to verify that the Codex CLI is correctly installed inside a running container before reporting it as ready.

#### Acceptance Criteria

1. THE Codex module SHALL implement a Health_Check that executes `codex --version` inside the Container and verifies exit code 0.
2. THE Health_Check SHALL be invoked by the core after the Container starts.
3. IF the Health_Check fails, THE core SHALL report the failure to the user with a descriptive error message identifying the Codex agent.

---

### Requirement CX-6: No Core Coupling

**User Story:** As a platform maintainer, I want the Codex module to be fully self-contained so that removing or replacing it requires no changes to core code.

#### Acceptance Criteria

1. THE Codex module SHALL NOT be referenced by name or identifier anywhere in the core application code.
2. THE Codex module SHALL register itself with the Agent_Registry without requiring any modification to core source files.
3. THE core application SHALL function correctly (with no enabled agents) if the Codex module is not compiled in.

> **Note:** The Codex agent is NOT included in `constants.DefaultAgents`. It is opt-in only — users must explicitly pass `--agents codex` or `--agents claude-code,codex` to include it.

---

## Cross-Cutting Concerns

This agent implements the general agent contract (Req 7 & 8) and the **Agent Summary Info** mechanism (SI-1–SI-7) defined in the [parent index](../requirements-agents.md).
