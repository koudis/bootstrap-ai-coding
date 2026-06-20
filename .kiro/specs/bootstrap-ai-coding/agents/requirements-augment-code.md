# Augment Code Agent Requirements

## Overview

Augment Code is an AI coding agent by Augment (augmentcode.com). Its CLI tool is called **Auggie** and is distributed as the `@augmentcode/auggie` npm package, providing the `auggie` binary. Auggie requires Node.js 22 or later. Authentication tokens and settings are stored in `~/.augment` on the Host. The agent is invoked inside the Container via the `auggie` command.

### Glossary

- **Auggie**: The Augment Code CLI tool, installed as the `auggie` binary via the `@augmentcode/auggie` npm package. Requires Node.js 22 or later.

---

### Requirement AC-1: Agent Identity

**User Story:** As the core system, I need the Augment Code module to declare a stable, unique identifier so it can be selected via the `--agents` flag.

#### Acceptance Criteria

1. THE Augment Code module SHALL declare the Agent_ID `"augment-code"`.
2. THE Agent_ID SHALL be stable across versions of the module and SHALL NOT change.

---

### Requirement AC-2: Installation

**User Story:** As a developer, I want Auggie to be pre-installed in the container image so I can run it immediately after connecting via SSH.

#### Acceptance Criteria

1. THE Augment Code module SHALL contribute Dockerfile steps that install Node.js 22 or later as a runtime dependency, compatible with the Base_Container_Image. Note: when both Claude Code and Augment Code are enabled (the default), the agents share a single Node.js installation. The Node.js version installed must be >= 22 to satisfy this requirement; since Node.js 22 is the current LTS, it also satisfies Claude Code's LTS requirement (see CC-2).
2. THE Augment Code module SHALL contribute Dockerfile steps that install the `@augmentcode/auggie` npm package globally.
3. WHEN the container image is built with the Augment Code agent enabled, the `auggie` command SHALL be available on the default `PATH` inside the Container for the Container_User.
4. THE installation steps SHALL NOT require any manual intervention after the container starts.

---

### Requirement AC-3: Credential Store

**User Story:** As a developer, I want my Augment Code authentication to persist across sessions so I only need to log in once.

#### Acceptance Criteria

1. THE Augment Code module SHALL declare `~/.augment` as its default Credential_Store path on the Host.
2. THE Augment Code module SHALL declare `<Container_User_Home>/.augment` as its Credential_Volume mount path inside the Container.
3. THE Credential_Volume SHALL be a bind-mount so that authentication tokens written inside the Container are immediately persisted to the Host Credential_Store.
4. Authentication tokens persisted in the Host Credential_Store SHALL be available in future Sessions without re-authentication.

---

### Requirement AC-4: Credential Presence Check

**User Story:** As the core system, I need to know whether the user has already authenticated Augment Code so I can inform them if they haven't.

#### Acceptance Criteria

1. THE Augment Code module SHALL implement a credential presence check that inspects the Credential_Store directory for existing authentication tokens.
2. THE credential presence check SHALL return `false` when the Credential_Store is empty or contains no recognisable Augment Code authentication tokens.
3. THE credential presence check SHALL return `true` when valid authentication tokens are present in the Credential_Store.
4. WHEN the credential presence check returns `false`, THE core SHALL print a message to stdout instructing the user to run `auggie login` inside the Container and complete the login flow.

---

### Requirement AC-5: Readiness Health Check

**User Story:** As the core system, I need to verify that Auggie is correctly installed inside a running container before reporting it as ready.

#### Acceptance Criteria

1. THE Augment Code module SHALL implement a Health_Check that verifies the `auggie` binary is present and executable inside the Container.
2. THE Health_Check SHALL be invoked by the core after the Container starts.
3. IF the Health_Check fails, THE core SHALL report the failure to the user with a descriptive error message identifying the Augment Code agent.

---

### Requirement AC-6: No Core Coupling

**User Story:** As a platform maintainer, I want the Augment Code module to be fully self-contained so that removing or replacing it requires no changes to core code.

#### Acceptance Criteria

1. THE Augment Code module SHALL NOT be referenced by name or identifier anywhere in the core application code.
2. THE Augment Code module SHALL register itself with the Agent_Registry without requiring any modification to core source files.
3. THE core application SHALL function correctly (with no enabled agents) if the Augment Code module is not compiled in.

---

## Cross-Cutting Concerns

This agent implements the general agent contract (Req 7 & 8) and the **Agent Summary Info** mechanism (SI-1–SI-7) defined in the [parent index](../requirements-agents.md).
