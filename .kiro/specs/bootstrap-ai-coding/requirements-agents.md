# Agent Module Requirements

## Introduction

This document defines the requirements for AI coding agent modules that plug into the `bootstrap-ai-coding` core. Each agent module is a self-contained implementation of the Agent_Interface defined by the core. The core does not need to be modified to add a new agent — only a new module conforming to this specification is required.

This document currently covers **Claude Code** as the reference implementation, **Augment Code** as the second agent module, **Build Resources** as a pseudo-agent that installs common build toolchains, and **Vibe Kanban** as a web-based project management tool for AI coding agents. Future agents (e.g. Codex, Gemini Code Assist, Aider) would each have their own section following the same structure.

> **Related documents:**
> - `requirements-core.md` — core application requirements including the Agent_Interface contract
> - `requirements-cli-combinations.md` — formal rules for valid and invalid CLI flag combinations

## Glossary

> Terms defined in `requirements-core.md` (Agent_Interface, Base_Container_Image, Container_User, Container_User_Home, Credential_Store, Credential_Volume) are not repeated here. See the core glossary for their definitions.
>
> **Note:** `Container_User` and `Container_User_Home` match the Host_User's username and home directory path (see core Requirement 22). Agent modules that reference these values (e.g. for credential mount paths) receive them via the Agent_Interface contract.

- **Agent_ID**: The unique, stable string identifier for an agent module (e.g. `"claude-code"`). This is the value users supply to the `--agents` flag.
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

## Augment Code Agent

### Overview

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

## Build Resources Agent

### Overview

Build Resources is a pseudo-agent that does not provide an AI coding tool. Instead, it installs common build toolchains and language runtimes into the container so that the development environment is ready for compilation, packaging, and general-purpose development tasks out of the box. It follows the standard agent module pattern (self-registers via `init()`, contributes Dockerfile steps, included in `DefaultAgents`) for architectural simplicity.

### Glossary

- **Build_Resources**: The set of system packages and language runtimes installed by this module: Python 3 (complete with setuptools/wheel), Python uv (system-wide via `UV_INSTALL_DIR`), CMake, build-essential, OpenJDK, Go, and common build dependencies (pkg-config, libssl-dev, libffi-dev, unzip, wget).

---

### Requirement BR-1: Agent Identity

**User Story:** As the core system, I need the Build Resources module to declare a stable, unique identifier so it can be selected via the `--agents` flag.

#### Acceptance Criteria

1. THE Build Resources module SHALL declare the Agent_ID `"build-resources"`.
2. THE Agent_ID SHALL be stable across versions of the module and SHALL NOT change.

---

### Requirement BR-2: Installation

**User Story:** As a developer, I want common build toolchains and language runtimes pre-installed in the container so I can compile and build projects immediately after connecting via SSH.

#### Acceptance Criteria

1. THE Build Resources module SHALL contribute Dockerfile steps that install **Python 3** (complete): `python3`, `python3-pip`, `python3-venv`, `python3-dev`, `python3-setuptools`, `python3-wheel`.
2. THE Build Resources module SHALL contribute Dockerfile steps that install **Python uv** via the official installer (`curl -LsSf https://astral.sh/uv/install.sh | UV_INSTALL_DIR=/usr/local/bin sh`), installed system-wide to `/usr/local/bin`.
3. THE Build Resources module SHALL ensure `uv` is available on the default system `PATH` (via `/usr/local/bin`) without any additional shell profile configuration.
4. THE Build Resources module SHALL contribute Dockerfile steps that install **CMake**: `cmake`.
5. THE Build Resources module SHALL contribute Dockerfile steps that install **build-essential**: `build-essential` (provides gcc, g++, make, libc-dev).
6. THE Build Resources module SHALL contribute Dockerfile steps that install **OpenJDK**: `default-jdk` (provides both JDK and JRE).
7. THE Build Resources module SHALL contribute Dockerfile steps that install **Go** (latest stable) via the official tarball from `https://go.dev/dl/`, extracted to `/usr/local/go`, with `/usr/local/go/bin` added to the system-wide `PATH`.
8. THE Build Resources module SHALL contribute Dockerfile steps that install **common build dependencies**: `pkg-config`, `libssl-dev`, `libffi-dev`, `unzip`, `wget`. These are transitive dependencies commonly required when building Python, Go, and C/C++ packages from source.
9. ALL packages and runtimes SHALL be installed globally (system-wide), including uv which uses `UV_INSTALL_DIR=/usr/local/bin`.
10. ALL installed tools SHALL be available to the Container_User without manual intervention after the container starts.

---

### Requirement BR-3: No Credential Store

**User Story:** As the core system, I need the Build Resources module to conform to the Agent_Interface even though it has no credentials to manage.

#### Acceptance Criteria

1. THE Build Resources module SHALL return an empty string from `CredentialStorePath()` indicating no host-side credential directory.
2. THE Build Resources module SHALL return an empty string from `ContainerMountPath()` indicating no bind-mount is needed.
3. THE Build Resources module SHALL always return `(true, nil)` from `HasCredentials()` — there are no credentials to check.

---

### Requirement BR-4: Readiness Health Check

**User Story:** As the core system, I need to verify that all build toolchains are correctly installed inside a running container before reporting the agent as ready.

#### Acceptance Criteria

1. THE Build Resources module SHALL implement a Health_Check that verifies the following commands exit with code 0 inside the Container:
   - `python3 --version`
   - `uv --version`
   - `cmake --version`
   - `javac -version`
   - `go version` (executed via `bash -lc` to pick up `/etc/profile.d/golang.sh`)
2. THE Health_Check SHALL be invoked by the core after the Container starts.
3. IF any Health_Check command fails, THE core SHALL report the failure to the user with a descriptive error message identifying the Build Resources agent and the specific tool that failed.

---

### Requirement BR-5: No Core Coupling

**User Story:** As a platform maintainer, I want the Build Resources module to be fully self-contained so that removing or replacing it requires no changes to core code.

#### Acceptance Criteria

1. THE Build Resources module SHALL NOT be referenced by name or identifier anywhere in the core application code.
2. THE Build Resources module SHALL register itself with the Agent_Registry without requiring any modification to core source files.
3. THE core application SHALL function correctly (with no enabled agents) if the Build Resources module is not compiled in.

---

### Requirement BR-6: Default Inclusion

**User Story:** As a developer, I want build toolchains installed by default so that the container is ready for development without needing to explicitly request them.

#### Acceptance Criteria

1. THE `constants.DefaultAgents` value SHALL include `"build-resources"` so that the module is enabled by default when the `--agents` flag is omitted.
2. THE user SHALL be able to exclude Build Resources by specifying `--agents` without `build-resources` in the list.


---

## Vibe Kanban Agent

### Overview

Vibe Kanban is a web-based project management tool designed for AI coding agents. It provides a kanban board, task management, and workspace management interface accessible via a web browser. It is distributed as the `vibe-kanban` npm package (GitHub: BloopAI/vibe-kanban, Apache-2.0 license) and run via `npx vibe-kanban`. Unlike other agent modules that are CLI tools invoked on demand, Vibe Kanban is a **web application** that must be running as a background service after container start so the user can access it from their host browser.

The container uses host network mode (Req 26) by default, so Vibe Kanban's auto-assigned port is directly accessible from the host browser without additional port forwarding.

### Glossary

- **Vibe_Kanban**: The web-based project management application for AI coding agents, run via `npx vibe-kanban`. Serves a combined frontend and backend on a single auto-assigned port.
- **Vibe_Kanban_Port**: The TCP port on which the Vibe Kanban server listens. Auto-assigned at startup (Vibe Kanban selects a free port). Accessible from the host browser via host network mode.

---

### Requirement VK-1: Agent Identity

**User Story:** As the core system, I need the Vibe Kanban module to declare a stable, unique identifier so it can be selected via the `--agents` flag.

#### Acceptance Criteria

1. THE Vibe Kanban module SHALL declare the Agent_ID `"vibe-kanban"` by returning that exact string from its `ID()` method, sourced from `constants.VibeKanbanAgentName`.
2. THE Agent_ID SHALL be stable across versions of the module and SHALL NOT change.
3. WHEN the module package is imported, THE Vibe Kanban module SHALL self-register with the global agent registry via its `init()` function by calling `agent.Register()`.
4. IF the Agent_ID `"vibe-kanban"` is already registered, THEN THE system SHALL panic with a message indicating a duplicate registration.

---

### Requirement VK-2: Installation

**User Story:** As a developer, I want Vibe Kanban to be pre-installed in the container image so it can start immediately when the container launches.

#### Acceptance Criteria

1. THE Vibe Kanban module SHALL contribute Dockerfile steps that install Node.js (>= 20) as a runtime dependency, compatible with the Base_Container_Image. Note: when Vibe Kanban is enabled alongside Claude Code and Augment Code (the default), the agents share a single Node.js installation. Since Augment Code requires Node.js 22+ (see AC-2), the installed version satisfies Vibe Kanban's >= 20 requirement. IF Node.js is already installed by another agent module's Dockerfile steps, THEN the Vibe Kanban module SHALL skip its own Node.js installation rather than installing a second copy.
2. THE Vibe Kanban module SHALL contribute Dockerfile steps that install the `vibe-kanban` npm package globally using `npm install -g vibe-kanban`.
3. WHEN the container image is built with Vibe Kanban enabled, the `vibe-kanban` command SHALL be available on the default `PATH` inside the Container for the Container_User, verifiable by running `which vibe-kanban` as the Container_User and receiving a zero exit code.
4. THE installation steps SHALL NOT require Rust or pnpm — the `vibe-kanban` npm package ships pre-built native binaries, so no native compilation toolchain beyond what `npm install -g` provides is needed.
5. THE installation steps SHALL NOT require any manual intervention after the container starts.

---

### Requirement VK-3: Automatic Service Start

**User Story:** As a developer, I want Vibe Kanban to be running automatically after the container starts, so I can immediately open it in my browser without manually launching it.

#### Acceptance Criteria

1. THE Vibe Kanban module SHALL configure the container (via Dockerfile RUN steps in its `Install()` method) so that the Vibe Kanban web server starts automatically as a background process when the container starts, without modifying the container's CMD instruction.
2. THE Vibe Kanban web server SHALL be started as the Container_User (not root).
3. WHEN the container starts, THE Vibe Kanban server SHALL be listening on the Vibe_Kanban_Port (auto-assigned by Vibe Kanban at startup) within 30 seconds.
4. WHEN the container restarts (due to the Container's Restart_Policy or Docker daemon restart), THE automatic start mechanism SHALL re-launch the Vibe Kanban process without user intervention, because the mechanism is baked into the container image.
5. IF the Vibe Kanban process crashes, THEN THE automatic start mechanism SHALL restart it without requiring user intervention, with a delay of at least 5 seconds between restart attempts and a maximum of 5 restart attempts within any 60-second window to prevent resource exhaustion from infinite crash loops.
6. THE automatic start mechanism SHALL NOT block the container's SSH server or other agent modules from starting.

---

### Requirement VK-4: No Credential Store

**User Story:** As the core system, I need the Vibe Kanban module to conform to the Agent_Interface even though it has no credentials to manage.

#### Acceptance Criteria

1. THE Vibe Kanban module SHALL return an empty string from `CredentialStorePath()` indicating no host-side credential directory exists for this agent.
2. THE Vibe Kanban module SHALL return an empty string from `ContainerMountPath(homeDir string)` regardless of the `homeDir` argument value, indicating no bind-mount is needed.
3. THE Vibe Kanban module SHALL return `(true, nil)` from `HasCredentials(storePath string)` regardless of the `storePath` argument value, indicating credentials are never missing.

---

### Requirement VK-5: Readiness Health Check

**User Story:** As the core system, I need to verify that Vibe Kanban is correctly installed and running inside a running container before reporting it as ready.

#### Acceptance Criteria

1. THE Vibe Kanban module SHALL implement a Health_Check that verifies the `vibe-kanban` binary is present and executable inside the Container by executing `vibe-kanban --version` and confirming it exits with code 0.
2. THE Health_Check SHALL verify that the Vibe Kanban web server process is running inside the Container by checking that a process matching `vibe-kanban` exists in the process table (e.g. via `pgrep -f vibe-kanban`). IF the process is not detected on the first attempt, THE Health_Check SHALL retry up to 5 times with a 2-second interval between attempts before reporting failure.
3. WHEN the Container starts, THE core SHALL invoke the Health_Check for the Vibe Kanban module.
4. IF the Health_Check fails, THEN THE core SHALL report the failure to the user with an error message identifying the Vibe Kanban agent and indicating which check failed (binary presence or process running).

---

### Requirement VK-6: No Core Coupling

**User Story:** As a platform maintainer, I want the Vibe Kanban module to be fully self-contained so that removing or replacing it requires no changes to core code.

#### Acceptance Criteria

1. THE Vibe Kanban module SHALL NOT be referenced by name (string literal `"vibe-kanban"`) or by Go import path anywhere in the core application code (all packages under `internal/` excluding `internal/agents/`).
2. THE Vibe Kanban module SHALL register itself with the Agent_Registry via an `init()` function that calls `agent.Register()`, without requiring any modification to core source files.
3. IF the Vibe Kanban module is not compiled in, THEN THE core application SHALL start, accept all CLI commands, and exit without panic or error attributable to the absent module.
4. IF the Vibe Kanban module is not compiled in and the user does not specify `--agents`, THEN THE core application SHALL operate using only the remaining agents present in `constants.DefaultAgents` that are registered.

---

### Requirement VK-7: Optional Inclusion

**User Story:** As a developer, I want Vibe Kanban available as an opt-in agent so that I can enable the project management board when I need it.

#### Acceptance Criteria

1. THE `constants.DefaultAgents` value SHALL NOT include `"vibe-kanban"` — the agent is opt-in only.
2. WHEN the user invokes the CLI with `--agents` including `"vibe-kanban"` (e.g. `--agents claude-code,augment-code,build-resources,vibe-kanban`), THE system SHALL include `"vibe-kanban"` in the enabled agents list and install it in the container.
3. IF the user specifies `--agents` with a list that does not contain `"vibe-kanban"`, THEN THE system SHALL not install or enable the Vibe Kanban module in the container, and `"vibe-kanban"` SHALL not appear in the session summary's enabled agents list.
4. THE `"vibe-kanban"` agent ID SHALL be registered in the agent registry so that `agent.Lookup("vibe-kanban")` resolves without error.

---

### Requirement VK-8: Host Browser Accessibility

**User Story:** As a developer, I want to access the Vibe Kanban web interface from my host browser, so I can manage tasks while working in the container.

#### Acceptance Criteria

1. WHEN the container is running in host network mode (Req 26, default), THE Vibe Kanban server SHALL be accessible from the host browser at `http://localhost:<Vibe_Kanban_Port>`, where the server responds with an HTTP 2xx status to a GET request on that URL.
2. WHEN the container is successfully started and the Vibe Kanban health check (VK-5) passes, THE Vibe Kanban module SHALL discover the Vibe_Kanban_Port via its `SummaryInfo()` method (see SI-5 in requirements-agent-summary-info.md) by inspecting the running Vibe Kanban process's listening port inside the Container, waiting up to 30 seconds for the port to become available.
3. THE session summary (Requirement 17 in requirements-core.md) SHALL include a labelled line "Vibe Kanban:" followed by the full URL `http://localhost:<Vibe_Kanban_Port>` so the user knows how to access it. This is delivered via the generic Agent Summary Info mechanism (SI-2, SI-7).
4. IF the Vibe Kanban module cannot discover the Vibe_Kanban_Port within the 30-second timeout (e.g. the process started but has not bound a port), THEN THE core SHALL print a warning message to stderr (per SI-3) and SHALL omit the Vibe Kanban URL from the session summary without failing the overall startup.
5. WHEN `--host-network-off` is set (bridge mode), THE Vibe Kanban server SHALL NOT be accessible from the host without additional port forwarding — this is a known limitation of bridge mode for non-SSH services.


---

### Requirement VK-9: Port Assignment and Discovery

**User Story:** As a developer running multiple bac containers simultaneously in host network mode, I need each container's Vibe Kanban instance to use a unique port so they don't conflict with each other.

#### Acceptance Criteria

1. THE Vibe Kanban server SHALL use auto-assigned port selection (port 0 / OS-assigned) at startup, allowing the operating system to choose a free port. THE supervisor script SHALL NOT hardcode or fix the port number.
2. BECAUSE containers in host network mode (Req 26, default) share the host's network namespace, a fixed port would cause bind failures when multiple bac containers run simultaneously. Auto-assignment ensures each instance gets a unique port.
3. THE `SummaryInfo()` method SHALL discover the auto-assigned port by reading a well-known port file (`/tmp/vibe-kanban.port`) written by the supervisor script. The supervisor discovers the port by polling `ss -tlnp` filtered by the vibe-kanban process PID, then writes the port number to the file. This approach is deterministic regardless of how many services listen in the container.
4. THE port discovery logic SHALL NOT rely on the process name appearing in `ss -tlnp` output, because the Rust binary downloaded by the npm wrapper may report under a different name depending on the platform and version. The supervisor uses PID-based filtering (`grep "pid=$VK_PID,"`) which is unambiguous.
5. THE container image SHALL include `iproute2` (provides `ss`) and `procps` (provides `pgrep`, `ps`) as runtime dependencies installed by the Vibe Kanban module's `Install()` method, to support port discovery and health checks.
6. THE supervisor script SHALL set `BROWSER=none` in the environment when launching vibe-kanban, to suppress the automatic browser-open attempt in the headless container environment.
7. THE Vibe Kanban module's `Install()` method SHALL pre-download the platform-specific binary during image build by running `vibe-kanban` with a timeout, so that the binary is cached in the container and does not require internet access at runtime.

#### Design Notes

- The vibe-kanban npm package (`npx vibe-kanban`) is a CLI wrapper that downloads a platform-specific Rust binary on first run and caches it in `~/.vibe-kanban/bin/`. The pre-download step during image build ensures the binary is available without network access at container start.
- The `HOST=0.0.0.0` environment variable is set so the server binds to all interfaces, making it accessible from the host in host network mode.
- In bridge mode (`--host-network-off`), port conflicts are not an issue since each container has its own network namespace, but the port is still auto-assigned for consistency.


---

## Agent Summary Info

See **[requirements-agent-summary-info.md](./requirements-agent-summary-info.md)** — Agent Summary Info mechanism: generic key:value pairs in session summary via Agent interface extension. Requirements SI-1–SI-7.
