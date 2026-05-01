# Core Application Requirements

## Introduction

`bootstrap-ai-coding` is a CLI tool that provisions an isolated Docker container for AI-assisted coding sessions. The user invokes it with a local project path; the tool starts a container with the project bind-mounted, an SSH server running, and one or more AI coding agents installed and ready to use.

The core application is responsible for all orchestration: Docker lifecycle management, SSH access, volume mounting, and the Agent module API that decouples agent implementations from the core. The core does not contain any agent-specific logic — agents are pluggable modules that conform to the Agent_Interface.

> **Related documents:**
> - `requirements-agents.md` — requirements for individual agent module implementations (Claude Code and future agents)
> - `requirements-cli-combinations.md` — formal rules for valid and invalid CLI flag combinations

## Glossary

- **CLI**: The `bootstrap-ai-coding` command-line interface invoked by the user.
- **Container**: A Docker container provisioned by the CLI to run the AI coding session.
- **Host**: The user's local machine where the CLI is executed.
- **Host_User**: The OS user account on the Host that invokes the CLI. The CLI must not be run as root.
- **Project_Path**: The absolute or relative filesystem path supplied by the user pointing to the project directory on the Host.
- **Mounted_Volume**: The Docker bind-mount that makes the Project_Path accessible inside the Container at `/workspace`.
- **SSH_Server**: The OpenSSH daemon running inside the Container.
- **Public_Key**: The user's SSH public key used to authenticate into the Container without a password.
- **Session**: A single lifecycle of a Container from creation to termination.
- **Base_Container_Image**: The base Docker image for all Containers: `ubuntu:26.04` (Ubuntu 26.04 LTS "Resolute Raccoon"). No other base image or Ubuntu version shall be used.
- **Container_User**: The non-root OS user account inside the Container under which SSH sessions run. Username is `dev`; UID and GID match those of the Host_User who invoked the CLI.
- **Container_Image**: The Docker image built on top of the Base_Container_Image that includes the SSH server, the Container_User setup, and all Enabled_Agent installations.
- **Agent**: An AI coding assistant module that conforms to the Agent_Interface and can be installed, authenticated, and invoked inside the Container.
- **Agent_Interface**: The contract defined by the core that every Agent module must satisfy. It covers: unique identification, Dockerfile installation steps, credential store location on the Host, credential mount path inside the Container, credential presence check, and a readiness health check.
- **Agent_Registry**: The core component that holds all registered Agent modules and resolves them by ID at runtime.
- **Enabled_Agents**: The set of Agent modules selected by the user for a given Container, specified via CLI flag or tool configuration.
- **Credential_Store**: The directory on the Host where an Agent's authentication tokens are persisted between Sessions. Each Agent module declares its own default Credential_Store path via the Agent_Interface.
- **Credential_Volume**: The Docker bind-mount that makes an Agent's Credential_Store accessible inside the Container at the path the Agent expects.
- **SSH_Port**: The host-side TCP port mapped to port 22 inside the Container. The CLI selects the SSH_Port by starting at `2222` and incrementing by 1 until a free port is found on the Host. The selected port is persisted in the Tool_Data_Dir for the project so the same port is reused on subsequent runs. Can be overridden per invocation via `--port`.
- **Container_User_Home**: The home directory of the Container_User inside the Container. Defined as `/home/<Container_User>` where `<Container_User>` is the username defined in the Container_User glossary entry.
- **Tool_Data_Dir**: The directory on the Host where the CLI stores all persistent data for a given project (SSH host keys, SSH port assignment, agent manifests). Located at `~/.config/bootstrap-ai-coding/<container-name>/`.

---

## Requirements

### Requirement 1: CLI Invocation

**User Story:** As a developer, I want to invoke the tool with a single command and a project path, so that I can start an AI coding session without manual Docker setup.

#### Acceptance Criteria

1. THE CLI SHALL accept a single positional argument representing the Project_Path.
2. WHEN the CLI is invoked with a valid Project_Path, THE CLI SHALL provision and start a Container for that project.
3. IF the Project_Path argument is omitted, THEN THE CLI SHALL print a usage message to stderr and exit with a non-zero exit code.
4. IF the Project_Path does not exist on the Host filesystem, THEN THE CLI SHALL print a descriptive error message to stderr and exit with a non-zero exit code.

---

### Requirement 2: Project Volume Mount

**User Story:** As a developer, I want my project directory to be accessible inside the container, so that agents can read and modify my project files.

#### Acceptance Criteria

1. WHEN a Container is started, THE CLI SHALL mount the Project_Path from the Host into the Container at `/workspace`.
2. THE Mounted_Volume SHALL reflect the live contents of the Project_Path on the Host (bind-mount, not a copy).
3. WHEN an Agent writes files to `/workspace` inside the Container, those changes SHALL be immediately visible on the Host at the original Project_Path.

---

### Requirement 3: SSH Server Inside the Container

**User Story:** As a developer, I want SSH access into the container, so that I can connect to the running session from my terminal or IDE.

#### Acceptance Criteria

1. WHEN a Container is started, THE SSH_Server SHALL be running and listening on port 22 inside the Container.
2. WHILE the Container is running, THE SSH_Server SHALL accept incoming connections on the SSH_Port.
3. IF the SSH_Server fails to start inside the Container, THEN THE CLI SHALL stop the Container, print a descriptive error message to stderr, and exit with a non-zero exit code.

---

### Requirement 4: Passwordless SSH Authentication via Public Key

**User Story:** As a developer, I want my SSH public key trusted inside the container, so that I can connect without entering a password.

#### Acceptance Criteria

1. THE CLI SHALL read the user's Public_Key from `~/.ssh/id_ed25519.pub`, `~/.ssh/id_rsa.pub`, or a path supplied via a `--ssh-key` option, in that order of precedence.
2. WHEN a Container is started, THE CLI SHALL install the Public_Key into the Container's `~/.ssh/authorized_keys` for the Container_User.
3. WHEN a user connects to the SSH_Server using the corresponding private key, THE SSH_Server SHALL authenticate the connection without prompting for a password.
4. IF no Public_Key can be located, THEN THE CLI SHALL print a descriptive error message to stderr and exit with a non-zero exit code.
5. THE SSH_Server SHALL disable password-based authentication inside the Container.

---

### Requirement 5: Container Lifecycle Management

**User Story:** As a developer, I want the container to be managed predictably so I don't accumulate stale containers and can always reconnect to an existing session.

#### Acceptance Criteria

1. WHEN the CLI starts a Container, THE CLI SHALL assign the Container a deterministic name derived from the Project_Path to prevent duplicate containers for the same project.
2. IF a Container with the same name is already running when the CLI is invoked, THEN THE CLI SHALL print the session summary (as defined in Requirement 17) and exit with a zero exit code without starting a duplicate.
3. THE CLI SHALL support a `--stop-and-remove` flag that, when provided with a Project_Path, stops and removes the Container associated with that project regardless of whether it is running or stopped.
4. WHEN the `--stop-and-remove` flag is used and no matching Container exists, THE CLI SHALL print a descriptive message to stdout and exit with a zero exit code.

---

### Requirement 6: Docker Prerequisite Check

**User Story:** As a developer, I want the tool to verify Docker is available before attempting to run, so that I get a clear error instead of a cryptic failure.

#### Acceptance Criteria

1. WHEN the CLI is invoked, THE CLI SHALL verify that the Docker daemon is reachable before attempting any container operations.
2. IF the Docker daemon is not reachable, THEN THE CLI SHALL print a descriptive error message to stderr instructing the user to start Docker, and exit with a non-zero exit code.
3. THE CLI SHALL verify that a compatible version of Docker (>= 20.10) is installed on the Host.
4. IF an incompatible Docker version is detected, THEN THE CLI SHALL print the detected version and the minimum required version to stderr and exit with a non-zero exit code.

---

### Requirement 7: Agent Module API

**User Story:** As a platform maintainer, I want the core to define a stable Agent_Interface and Agent_Registry so that new agent modules can be added without modifying any core code.

#### Acceptance Criteria

1. THE core SHALL define an Agent_Interface that every Agent module must implement. The interface SHALL cover at minimum: a unique string identifier, Dockerfile installation steps, the default Credential_Store path on the Host, the mount path inside the Container, a credential presence check, and a readiness health check.
2. THE Agent_Registry SHALL allow Agent modules to register themselves without requiring changes to core system code. Adding a new Agent SHALL require only a new module that implements the Agent_Interface and registers itself.
3. IF an Agent identifier supplied by the user is not found in the Agent_Registry, THEN THE CLI SHALL print a descriptive error message to stderr listing the unknown identifier and exit with a non-zero exit code.
4. THE CLI SHALL accept an `--agents` flag whose value is a comma-separated list of Agent identifiers specifying the Enabled_Agents for the Container.
5. WHEN the `--agents` flag is omitted, THE CLI SHALL use `claude-code` as the default Enabled_Agent.

---

### Requirement 8: Agent Lifecycle Orchestration

**User Story:** As a developer, I want the core to handle installation and credential setup for every enabled agent automatically, so I don't have to configure each agent manually.

#### Acceptance Criteria

1. WHEN a Container_Image is built, THE CLI SHALL call the installation procedure of every Enabled_Agent so each agent contributes its Dockerfile steps to the image.
2. THE Container_Image SHALL include installation steps only for Enabled_Agents; agents not in the Enabled_Agents set SHALL NOT be installed in the image.
3. WHEN a Container is started, THE CLI SHALL mount each Enabled_Agent's Credential_Store from the Host as a Credential_Volume at the path declared by that Agent via the Agent_Interface.
4. WHEN the Credential_Store directory for an Enabled_Agent does not exist on the Host, THE CLI SHALL create it before starting the Container.
5. WHEN a Container is started and the Credential_Store for an Enabled_Agent contains no existing authentication tokens, THE CLI SHALL print a message to stdout identifying that Agent by name and instructing the user to authenticate it inside the Container.
6. Credentials written by an Agent inside the Container SHALL be immediately persisted to the Host Credential_Store via the bind-mount, and SHALL be available in future Sessions without re-authentication.

---

### Requirement 9: Container Base Image

**User Story:** As a developer, I want the container to be based on a known, stable Ubuntu LTS release so that I have a predictable, well-supported environment.

#### Acceptance Criteria

1. THE Container_Image SHALL be built on top of the Base_Container_Image.
2. THE Container_Image SHALL NOT be based on any other Linux distribution or Ubuntu version.
3. THE Dockerfile FROM instruction SHALL reference the Base_Container_Image tag exactly.

---

### Requirement 10: Container User Identity

**User Story:** As a developer, I want the container to run as a non-root user whose UID and GID match my host user, so that files created inside the container have the correct ownership on the host.

#### Acceptance Criteria

1. THE Container SHALL run SSH sessions as the Container_User.
2. THE Container_User's UID inside the Container SHALL match the effective UID of the Host_User who invoked the CLI.
3. THE Container_User's GID inside the Container SHALL match the effective GID of the Host_User who invoked the CLI.
4. THE Container_User SHALL have passwordless `sudo` access inside the Container to allow installation of additional tools during a session.
5. THE CLI SHALL pass the Host_User's UID and GID to the Container at creation time.
6. Files written to `/workspace` inside the Container SHALL be owned by the Host_User's UID/GID on the Host filesystem.

---

### Requirement 11: Root Execution Prevention

**User Story:** As a developer, I want the tool to refuse to run as root, so that containers are not accidentally created with root-owned files and volumes.

#### Acceptance Criteria

1. WHEN the CLI is invoked, THE CLI SHALL check the effective user ID of the process.
2. IF the effective user ID is 0 (root), THEN THE CLI SHALL print a descriptive error message to stderr explaining that running as root is not permitted, and exit with a non-zero exit code.
3. THE error message SHALL suggest re-running the command as a non-root user.

---

### Requirement 12: SSH Port Configuration

**User Story:** As a developer, I want a predictable SSH port that is automatically assigned and remembered, so I can reconnect without looking up the port each time and run multiple projects simultaneously without conflicts.

#### Acceptance Criteria

1. WHEN the CLI starts a Container for the first time, THE CLI SHALL select the SSH_Port by checking port `2222` on the Host; if it is in use, THE CLI SHALL increment by 1 and repeat until a free port is found.
2. THE selected SSH_Port SHALL be persisted in the Tool_Data_Dir for the project so that subsequent invocations reuse the same port.
3. THE CLI SHALL accept a `--port` flag that overrides the automatic SSH_Port selection for the Container; the overridden value SHALL also be persisted in the Tool_Data_Dir.
4. WHEN a Container is started, THE CLI SHALL bind the Container's internal SSH port 22 to the SSH_Port on the Host.
5. IF the persisted SSH_Port is in use on the Host by a process other than the Container for this project when the Container is started, THE CLI SHALL print a descriptive error message to stderr identifying the port conflict and exit with a non-zero exit code.
6. THE CLI SHALL print the SSH_Port and the full SSH connection command as part of the session summary defined in Requirement 18.

---

### Requirement 13: SSH Host Key Persistence

**User Story:** As a developer, I want the container's SSH host key to remain stable across image rebuilds, so that I never see a "host key changed" warning and am not trained to ignore SSH security alerts.

#### Acceptance Criteria

1. THE CLI SHALL generate an SSH host key pair for each project the first time a Container_Image is built for that project, and store it in the Tool_Data_Dir for that project.
2. WHEN a Container_Image is built or rebuilt, THE CLI SHALL inject the persisted SSH host key from the Tool_Data_Dir into the Container_Image so the SSH_Server uses it.
3. THE SSH host key SHALL remain the same across Container_Image rebuilds for the same project, so that the SSH client's `known_hosts` entry remains valid.
4. THE persisted SSH host key files SHALL be readable only by the Host_User (file permissions `0600`).

---

### Requirement 14: Container Image Rebuild

**User Story:** As a developer, I want the container image to stay up to date with my agent configuration, and to be warned when a rebuild is needed rather than having it happen silently.

#### Acceptance Criteria

1. WHEN the CLI starts a Container and no existing Container_Image is found, THE CLI SHALL build the Container_Image automatically before starting the Container.
2. THE CLI SHALL store a manifest file inside the Container_Image (at `/bac-manifest.json`) listing the Enabled_Agents used to build it.
3. WHEN the CLI starts a Container and the Enabled_Agents set does not match the manifest in the existing Container_Image, THE CLI SHALL stop, print a message to stdout informing the user that the agent configuration has changed, and instruct them to run with `--rebuild` to update the image.
4. THE CLI SHALL support a `--rebuild` flag that forces a full Container_Image rebuild regardless of the existing manifest.
5. WHEN a rebuild is triggered (automatically or via `--rebuild`), THE CLI SHALL print a message to stdout indicating that the image is being built.
6. IF the image build fails, THE CLI SHALL print the build output to stderr and exit with a non-zero exit code.

---

### Requirement 15: Tool Data Directory

**User Story:** As a developer, I want all tool-generated files to be stored in a single well-known directory, so I always know where to find them and can manage them easily.

#### Acceptance Criteria

1. THE CLI SHALL store all persistent data for each project in the Tool_Data_Dir.
2. WHEN the Tool_Data_Dir does not exist, THE CLI SHALL create it (and all parent directories) with permissions `0700` before writing any files.
3. All files written to the Tool_Data_Dir SHALL be readable only by the Host_User (permissions `0600` for files, `0700` for subdirectories).

---

### Requirement 16: Purge Tool Data

**User Story:** As a developer, I want to be able to completely remove all data the tool has stored on my machine, so there are no leftovers if I stop using the tool.

#### Acceptance Criteria

1. THE CLI SHALL support a `--purge` flag that removes all data stored by the tool on the Host.
2. WHEN `--purge` is invoked, THE CLI SHALL stop and remove all running or stopped Containers managed by the tool.
3. WHEN `--purge` is invoked, THE CLI SHALL remove the entire Tool_Data_Dir root (`~/.config/bootstrap-ai-coding/`) and all its contents.
4. WHEN `--purge` is invoked, THE CLI SHALL remove all Container_Images built by the tool from the local Docker image store.
5. BEFORE executing any destructive action, THE CLI SHALL print a summary of what will be deleted and prompt the user for confirmation; if the user does not confirm, THE CLI SHALL exit with a zero exit code without deleting anything.
6. WHEN `--purge` completes successfully, THE CLI SHALL print a confirmation message to stdout listing what was removed.

---

### Requirement 17: Startup Session Summary

**User Story:** As a developer, I want a concise summary printed when the tool starts a container, so I can immediately see the key session details without having to look them up.

#### Acceptance Criteria

1. WHEN a Container is successfully started (or is already running), THE CLI SHALL print a session summary to stdout before exiting.
2. THE session summary SHALL include the following fields on separate labelled lines:
   - **Data directory**: the Tool_Data_Dir path for this project
   - **Project directory**: the Project_Path
   - **SSH port**: the SSH_Port
   - **SSH connect**: the full SSH connection command (e.g. `ssh -p <SSH_Port> <Container_User>@localhost`)
   - **Enabled agents**: the list of Enabled_Agent identifiers
3. THE session summary SHALL be printed as plain text to stdout, with one field per line.
4. THE session summary SHALL be printed after all startup checks pass and the Container is confirmed ready.
