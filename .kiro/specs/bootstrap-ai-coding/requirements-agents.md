# Agent Module Requirements

## Introduction

This document defines the requirements for AI coding agent modules that plug into the `bootstrap-ai-coding` core. Each agent module is a self-contained implementation of the Agent_Interface defined by the core. The core does not need to be modified to add a new agent — only a new module conforming to this specification is required.

This document currently covers **Claude Code** as the reference implementation. Future agents (e.g. Codex, Gemini Code Assist, Aider) would each have their own section following the same structure.

> **Related documents:**
> - `requirements-core.md` — core application requirements including the Agent_Interface contract
> - `requirements-cli-combinations.md` — formal rules for valid and invalid CLI flag combinations

## Glossary

- **Agent_Interface**: The contract defined by the core application that every agent module must implement. See `requirements-core.md` for the full definition.
- **Agent_ID**: The unique, stable string identifier for an agent module (e.g. `"claude-code"`). This is the value users supply to the `--agents` flag.
- **Base_Container_Image**: Defined in `requirements-core.md`. The base Docker image (`ubuntu:26.04`) on which all Container_Images are built.
- **Container_User**: Defined in `requirements-core.md`. The non-root user (`dev`) inside the Container whose UID/GID match the Host_User.
- **Container_User_Home**: Defined in `requirements-core.md`. The home directory of the Container_User inside the Container (`/home/<Container_User>`).
- **Credential_Store**: The directory on the Host where the agent's authentication tokens are persisted. The agent module declares its own default path.
- **Credential_Volume**: The Docker bind-mount that makes the Credential_Store accessible inside the Container at the path the agent expects.
- **Health_Check**: A verification step run after container start to confirm the agent is installed and ready to use.

---

## Claude Code Agent

### Overview

Claude Code is Anthropic's AI coding agent. It is the first and default agent module for `bootstrap-ai-coding`. It is installed via npm, stores its authentication tokens in `~/.claude` on the Host, and is invoked inside the Container via the `claude` command.

---

### Requirement CC-1: Agent Identity

**User Story:** As the core system, I need the Claude Code module to declare a stable, unique identifier so it can be selected via the `--agents` flag.

#### Acceptance Criteria

1. THE Claude Code module SHALL declare the Agent_ID `"claude-code"`.
2. THE Agent_ID SHALL be stable across versions of the module and SHALL NOT change.

---

### Requirement CC-2: Installation

**User Story:** As a developer, I want Claude Code to be pre-installed in the container image so I can run it immediately after connecting via SSH.

#### Acceptance Criteria

1. THE Claude Code module SHALL contribute Dockerfile steps that install Node.js (LTS) as a runtime dependency, compatible with the Base_Container_Image.
2. THE Claude Code module SHALL contribute Dockerfile steps that install the `@anthropic-ai/claude-code` npm package globally.
3. WHEN the container image is built with Claude Code enabled, the `claude` command SHALL be available on the default `PATH` inside the Container for the Container_User.
4. THE installation steps SHALL NOT require any manual intervention after the container starts.

---

### Requirement CC-3: Credential Store

**User Story:** As a developer, I want my Claude Code authentication to persist across sessions so I only need to log in once.

#### Acceptance Criteria

1. THE Claude Code module SHALL declare `~/.claude` as its default Credential_Store path on the Host.
2. THE Claude Code module SHALL declare `<Container_User_Home>/.claude` as its Credential_Volume mount path inside the Container.
3. THE Credential_Volume SHALL be a bind-mount so that authentication tokens written inside the Container are immediately persisted to the Host Credential_Store.
4. Authentication tokens persisted in the Host Credential_Store SHALL be available in future Sessions without re-authentication.

---

### Requirement CC-4: Credential Presence Check

**User Story:** As the core system, I need to know whether the user has already authenticated Claude Code so I can inform them if they haven't.

#### Acceptance Criteria

1. THE Claude Code module SHALL implement a credential presence check that inspects the Credential_Store directory for existing authentication tokens.
2. THE credential presence check SHALL return `false` when the Credential_Store is empty or contains no recognisable Claude Code authentication tokens.
3. THE credential presence check SHALL return `true` when valid authentication tokens are present in the Credential_Store.
4. WHEN the credential presence check returns `false`, THE core SHALL print a message to stdout instructing the user to run `claude` inside the Container and complete the login flow.

---

### Requirement CC-5: Readiness Health Check

**User Story:** As the core system, I need to verify that Claude Code is correctly installed inside a running container before reporting it as ready.

#### Acceptance Criteria

1. THE Claude Code module SHALL implement a Health_Check that verifies the `claude` binary is present and executable inside the Container.
2. THE Health_Check SHALL be invoked by the core after the Container starts.
3. IF the Health_Check fails, THE core SHALL report the failure to the user with a descriptive error message identifying the Claude Code agent.

---

### Requirement CC-6: No Core Coupling

**User Story:** As a platform maintainer, I want the Claude Code module to be fully self-contained so that removing or replacing it requires no changes to core code.

#### Acceptance Criteria

1. THE Claude Code module SHALL NOT be referenced by name or identifier anywhere in the core application code.
2. THE Claude Code module SHALL register itself with the Agent_Registry without requiring any modification to core source files.
3. THE core application SHALL function correctly (with no enabled agents) if the Claude Code module is not compiled in.
