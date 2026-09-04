# Kiro Agent Requirements

## Overview

Kiro is AWS's AI-powered coding agent. This module makes Kiro usable inside a `bootstrap-ai-coding` container in two complementary ways:

1. **Remote IDE connection (primary intent).** The container already runs an SSH server on a persisted `SSH_Port` (see core Req 13 & the SSH requirements). The Kiro IDE running on the Host connects into the container as a remote workspace using its Open Remote - SSH capability — the same `ssh bac-<container-name>` alias the tool writes to `~/.ssh/config`. No extra software is strictly required inside the container for this path beyond the SSH server that already exists.
2. **In-container Kiro CLI agent.** The Kiro CLI (`kiro-cli`) is pre-installed in the image so it can be run directly over SSH, and so Kiro's remote server components have their runtime dependencies (Node.js) available. Authentication persists through the bind-mounted `~/.kiro` directory (the same credential-persistence approach used by every other agent), so a login or API key established once on the Host is available inside the Container across sessions and rebuilds.

Kiro is installed via the official install script (`curl -fsSL https://cli.kiro.dev/install | bash`) and invoked inside the Container via the `kiro-cli` command. Like Codex and OpenCode, Kiro is **opt-in only** — users must explicitly include it via `--agents kiro`.

> **Note on environment variables:** The core does not currently forward Host environment variables (such as `KIRO_API_KEY`) into the running Container — `docker.ContainerSpec` has no runtime-env field and `CreateContainer` never sets `container.Config.Env`. This module therefore relies exclusively on the `~/.kiro` bind-mount for credential/config persistence, consistent with the other agents, and does not depend on any env-passthrough mechanism.

> **Sources (for maintainers):** Kiro CLI install command and `kiro-cli` invocation — [Kiro CLI installation docs](https://kiro.dev/docs/getting-started/installation/) and [Arm Learning Paths: install Kiro CLI](https://learn.arm.com/install-guides/kiro-cli/). Headless mode via `KIRO_API_KEY` + `--no-interactive` — [Kiro headless mode docs](https://kiro.dev/docs/cli/headless/) and [Kiro CLI 2.0 changelog](https://kiro.dev/changelog/cli/2-0/). Remote IDE connection over SSH — [Kiro remote server issue #496](https://github.com/kirodotdev/Kiro/issues/496). Content was rephrased for compliance with licensing restrictions.

### Glossary

- **Kiro_CLI**: The Kiro command-line AI coding agent, installed via `https://cli.kiro.dev/install`, providing the `kiro-cli` binary. Requires Node.js as a runtime dependency.
- **Kiro_Config_Store**: The host-side directory `~/.kiro/` where Kiro persists agent configuration, steering, authentication state, and related data. This directory is bind-mounted into the Container so that everything established on the Host — including sign-in state — is available inside the Container.

---

### Requirement KIRO-1: Agent Identity

**User Story:** As the core system, I need the Kiro module to declare a stable, unique identifier so it can be selected via the `--agents` flag.

#### Acceptance Criteria

1. THE Kiro module SHALL declare the Agent_ID `"kiro"`.
2. THE Agent_ID SHALL be stable across versions of the module and SHALL NOT change.
3. THE Agent_ID `"kiro"` SHALL be unique among all registered agent modules.

---

### Requirement KIRO-2: Installation

**User Story:** As a developer, I want the Kiro CLI to be pre-installed in the container image so I can run it and connect the Kiro IDE remotely immediately after the container starts.

#### Acceptance Criteria

1. THE Kiro module SHALL contribute Dockerfile steps that install Node.js 22 as a runtime dependency, using the shared `IsNodeInstalled()`/`MarkNodeInstalled()` deduplication pattern so it does not conflict with other Node-based agents.
2. THE Kiro module SHALL contribute Dockerfile steps that install the Kiro CLI via the official installer (`curl -fsSL https://cli.kiro.dev/install | bash`).
3. WHEN the container image is built with the Kiro agent enabled, the `kiro-cli` command SHALL be available on the default `PATH` inside the Container for the Container_User.
4. THE installation SHALL run non-interactively and SHALL NOT trigger a browser-based authentication flow during image build.
5. THE installation steps SHALL NOT require any manual intervention after the container starts.
6. IF the Kiro installer requires additional OS packages (e.g. `curl`, `ca-certificates`, `unzip`), THE Kiro module SHALL install them as part of its Dockerfile steps.

---

### Requirement KIRO-3: Remote IDE Connectivity over SSH

**User Story:** As a developer, I want to open the container as a remote workspace in the Kiro IDE so I can run Kiro against the mounted project without leaving my editor.

#### Acceptance Criteria

1. THE container SHALL expose an SSH server reachable from the Host on the persisted `SSH_Port`, as already provided by the core (this module SHALL NOT re-implement SSH provisioning).
2. THE Kiro module SHALL rely on the tool's existing `~/.ssh/config` alias (`ssh bac-<container-name>`) as the connection target for the Kiro IDE Open Remote - SSH flow.
3. THE container image SHALL include the runtime dependencies (Node.js) that Kiro's remote server components require to bootstrap on the remote host.
4. THE module SHALL NOT require any inbound network port beyond the SSH port the core already manages.
5. NOTE: Completing an interactive Kiro sign-in inside a remote session may require the port-forwarded local OAuth callback that the Kiro IDE sets up automatically; this module does not manage that flow and defers to the IDE. For persistence of authentication state across sessions, see KIRO-4.

---

### Requirement KIRO-4: Authentication Persistence via Bind-Mount

**User Story:** As a developer, I want my Kiro authentication and configuration to persist across sessions and rebuilds so I only sign in once, without relying on any environment-variable passthrough the core does not provide.

#### Acceptance Criteria

1. THE Kiro module SHALL rely on the bind-mounted Kiro_Config_Store (`~/.kiro`, see KIRO-5) as the sole mechanism for carrying authentication and configuration state between the Host and the Container.
2. WHEN the user has authenticated Kiro on the Host (via the Kiro IDE or `kiro-cli`) such that credentials or an API key are persisted under `~/.kiro`, THOSE credentials SHALL be visible inside the Container through the bind-mount without re-authentication.
3. THE Kiro module SHALL NOT depend on the Host `KIRO_API_KEY` (or any other Host environment variable) being forwarded into the Container, because the core does not provide a runtime environment-variable passthrough mechanism.
4. THE Kiro module SHALL NOT hardcode, log, or bake any API key or credential value into the container image, the agent manifest, or any `ENV` instruction.
5. WHEN no authentication state exists under `~/.kiro`, THE module SHALL still install successfully and SHALL leave interactive/IDE-based authentication (KIRO-3) as the fallback, without failing the build or container start.

---

### Requirement KIRO-5: Credential & Configuration Store

**User Story:** As a developer, I want my Kiro configuration and any persisted credentials to survive across sessions so I only configure once.

#### Acceptance Criteria

1. THE Kiro module SHALL declare `~/.kiro` as its Credential_Store path on the Host (Kiro_Config_Store).
2. THE Kiro module SHALL declare `<Container_User_Home>/.kiro` as its Credential_Volume mount path inside the Container.
3. THE Credential_Volume SHALL be a bind-mount so that configuration and any credentials written inside the Container are immediately persisted to the Host Credential_Store.
4. IF the Host Kiro_Config_Store directory does not exist at container start time, THEN THE core SHALL create it with permissions `0700` before mounting.
5. Configuration persisted in the Host Kiro_Config_Store SHALL be available in future Sessions without reconfiguration.

---

### Requirement KIRO-6: Credential Presence Check

**User Story:** As the core system, I need to know whether the user has configured Kiro authentication so I can inform them if they haven't.

#### Acceptance Criteria

1. THE Kiro module SHALL implement a credential presence check that inspects the Kiro_Config_Store (`~/.kiro`) for recognisable Kiro authentication state.
2. THE credential presence check SHALL return `true` when authentication state is present in the Kiro_Config_Store.
3. THE credential presence check SHALL return `false` with a nil error when the Kiro_Config_Store is absent or contains no recognisable Kiro authentication state.
4. IF a filesystem error other than file-not-found or directory-not-found occurs while inspecting the Kiro_Config_Store, THE credential presence check SHALL return `false` and a non-nil error describing the failure.
5. WHEN the credential presence check returns `false`, THE core SHALL print a message to stdout instructing the user to authenticate Kiro (via the Kiro IDE or `kiro-cli`) so that credentials are persisted under `~/.kiro`.

---

### Requirement KIRO-7: Readiness Health Check

**User Story:** As the core system, I need to verify that the Kiro CLI is correctly installed inside a running container before reporting it as ready.

#### Acceptance Criteria

1. THE Kiro module SHALL implement a Health_Check that executes `kiro-cli --version` inside the Container and verifies exit code 0.
2. THE Health_Check SHALL be invoked by the core after the Container starts, running as the Container_User.
3. IF `kiro-cli --version` returns a non-zero exit code, THE Kiro module SHALL return an error indicating that the health check failed, including the exit code.
4. IF the Health_Check fails, THE core SHALL report the failure to the user with a descriptive error message identifying the Kiro agent.

---

### Requirement KIRO-8: No Core Coupling

**User Story:** As a platform maintainer, I want the Kiro module to be fully self-contained so that removing or replacing it requires no changes to core code.

#### Acceptance Criteria

1. THE Kiro module SHALL NOT be referenced by name or identifier anywhere in the core application code.
2. THE Kiro module SHALL register itself with the Agent_Registry via `init()` without requiring any modification to core source files.
3. THE core application SHALL function correctly if the Kiro module is not compiled in.
4. THE string literal `"kiro"` (as an Agent_ID) SHALL NOT appear in any source file under `internal/cmd/`, `internal/naming/`, `internal/docker/`, `internal/ssh/`, `internal/datadir/`, `internal/pathutil/`, `internal/hostinfo/`, or `internal/agent/`.
5. THE Agent_ID constant SHALL be defined in `internal/constants/` (e.g. `KiroAgentName`) and referenced via `constants.KiroAgentName`, consistent with the other agent modules.

> **Note:** The Kiro agent is NOT included in `constants.DefaultAgents`. It is opt-in only — users must explicitly pass `--agents kiro` (or e.g. `--agents claude-code,kiro`) to include it.

---

## Cross-Cutting Concerns

This agent implements the general agent contract (Req 7 & 8) and the **Agent Summary Info** mechanism (SI-1–SI-7) defined in the [parent index](../requirements-agents.md). Its `SummaryInfo` MAY contribute a key:value line (for example, confirming that the Kiro IDE can connect via the `ssh bac-<container-name>` alias); returning `(nil, nil)` is acceptable if there is nothing to report.
