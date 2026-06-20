# Build Resources Agent Requirements

## Overview

Build Resources is a pseudo-agent that does not provide an AI coding tool. Instead, it installs common build toolchains and language runtimes into the container so that the development environment is ready for compilation, packaging, and general-purpose development tasks out of the box. It follows the standard agent module pattern (self-registers via `init()`, contributes Dockerfile steps, included in `DefaultAgents`) for architectural simplicity.

### Glossary

- **Build_Resources**: The set of system packages and language runtimes installed by this module: Python 3 (complete with setuptools/wheel), Python uv (system-wide via `UV_INSTALL_DIR`), CMake, build-essential, OpenJDK, Go, graphify (knowledge graph skill for AI coding assistants, installed via `uv tool install`), tree (directory listing utility), btop (terminal-based resource monitor), and common build dependencies (pkg-config, libssl-dev, libffi-dev, unzip, wget).

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
9. THE Build Resources module SHALL contribute Dockerfile steps that install **terminal and directory utilities**: `tree` (directory listing), `btop` (terminal-based resource monitor).
10. THE Build Resources module SHALL contribute Dockerfile steps that install **graphify** via `UV_TOOL_BIN_DIR=/usr/local/bin uv tool install graphifyy`. Graphify is an open-source knowledge graph skill for AI coding assistants (GitHub: safishamsi/graphify). After installation, `graphify install` SHALL be run to set it up as a Claude Code skill. Requires Python 3.10+ (satisfied by the python3 installation in AC-1). Note: `uv tool install` is used instead of `pip install` because Ubuntu 26.04 enforces PEP 668 (externally-managed-environment), and uv is already installed by this module (AC-2). The `UV_TOOL_BIN_DIR=/usr/local/bin` environment variable ensures the `graphify` executable is placed in a system-wide location accessible by all users (by default, `uv tool install` places executables in `$HOME/.local/bin` which would not be on the Container_User's PATH when the build runs as root).
11. ALL packages and runtimes SHALL be installed globally (system-wide), including uv which uses `UV_INSTALL_DIR=/usr/local/bin`.
12. ALL installed tools SHALL be available to the Container_User without manual intervention after the container starts.

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
   - `graphify --version`
   - `tree --version`
   - `btop --version`
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

## Cross-Cutting Concerns

This agent implements the general agent contract (Req 7 & 8) and the **Agent Summary Info** mechanism (SI-1–SI-7) defined in the [parent index](../requirements-agents.md).
