# Claude Code Agent Requirements

## Overview

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

1. THE Claude Code module SHALL contribute Dockerfile steps that install Node.js (LTS) as a runtime dependency, compatible with the Base_Container_Image. Note: when both Claude Code and Augment Code are enabled (the default), the agents share a single Node.js installation. Since Augment Code requires Node.js 22+ (see AC-2), the installed version must satisfy both agents' requirements. In practice, Node.js 22 is the current LTS and satisfies both constraints.
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
5. NOTE: Claude Code also stores onboarding state in `~/.claude.json` (outside the credential directory). See Requirement CC-8 for how this is handled via symlink and host-side synchronisation.

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

### Requirement CC-7: Headless Keyring for Credential Persistence

**User Story:** As a developer, I want Claude Code to be able to read and refresh its OAuth tokens inside the container without a graphical desktop, so I don't have to re-authenticate every time I connect.

#### Acceptance Criteria

1. THE container image SHALL include a D-Bus session bus and a Secret Service–compatible keyring daemon (gnome-keyring) capable of running without a graphical display.
2. THE keyring daemon SHALL be started automatically when the Container_User's SSH session begins, using an empty password to unlock the default keyring.
3. Claude Code (and any other tool using `libsecret` / D-Bus Secret Service API) SHALL be able to store and retrieve credentials via the running keyring daemon without user interaction.
4. THE `DBUS_SESSION_BUS_ADDRESS` environment variable SHALL be set correctly for the Container_User's session so that client applications can locate the session bus.
5. THE keyring setup SHALL NOT interfere with the existing SSH-based authentication or the bind-mounted `~/.claude` credential store.
6. THE keyring packages and startup configuration SHALL be installed as part of the base container image (in `DockerfileBuilder`), not in individual agent modules, since multiple agents and IDE extensions may benefit from it.

---

### Requirement CC-6: No Core Coupling

**User Story:** As a platform maintainer, I want the Claude Code module to be fully self-contained so that removing or replacing it requires no changes to core code.

#### Acceptance Criteria

1. THE Claude Code module SHALL NOT be referenced by name or identifier anywhere in the core application code.
2. THE Claude Code module SHALL register itself with the Agent_Registry without requiring any modification to core source files.
3. THE core application SHALL function correctly (with no enabled agents) if the Claude Code module is not compiled in.

---

### Requirement CC-8: Onboarding State Synchronisation

**User Story:** As a developer, I want my Claude Code onboarding state to persist across container recreations, so I am not prompted to complete the onboarding flow every time the container is rebuilt.

#### Acceptance Criteria

1. Claude Code stores its onboarding state (including `hasCompletedOnboarding`) in `~/.claude.json` on the Host — a file in the home directory root, separate from the `~/.claude/` credential directory.
2. THE Claude Code module SHALL create a symlink inside the Container at `<Container_User_Home>/.claude.json` pointing to `<Container_User_Home>/.claude/claude.json`, so that Claude Code reads and writes its onboarding state through the bind-mounted Credential_Volume.
3. THE Claude Code module SHALL implement the `CredentialPreparer` interface. Its `PrepareCredentials` method SHALL copy `~/.claude.json` from the Host home directory into the Credential_Store as `claude.json`, but only when the source file exists and is newer than the destination (or the destination is absent).
4. THE combination of the symlink (inside the container) and the host-side copy (before mount) SHALL ensure that a single bind-mount on `~/.claude/` persists both OAuth tokens and onboarding state across container rebuilds and restarts.
5. IF `~/.claude.json` does not exist on the Host (first-time user), THE `PrepareCredentials` method SHALL silently skip the copy without error.

---

## Cross-Cutting Concerns

This agent implements the general agent contract (Req 7 & 8) and the **Agent Summary Info** mechanism (SI-1–SI-7) defined in the [parent index](../requirements-agents.md).
