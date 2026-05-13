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
- **Container_User**: The non-root OS user account inside the Container under which SSH sessions run. Username matches the Host_User's username; UID and GID match those of the Host_User who invoked the CLI.
- **Conflicting_Image_User**: An existing user in the Base_Container_Image whose UID or GID matches the Host_User's UID or GID. If present, must be resolved before the Container_Image can be built (see Requirement 10a).
- **Container_Image**: The Docker image built on top of the Base_Container_Image that includes the SSH server, the Container_User setup, and all Enabled_Agent installations.
- **Agent**: An AI coding assistant module that conforms to the Agent_Interface and can be installed, authenticated, and invoked inside the Container.
- **Agent_Interface**: The contract defined by the core that every Agent module must satisfy. It covers: unique identification, Dockerfile installation steps, credential store location on the Host, credential mount path inside the Container, credential presence check, and a readiness health check.
- **Agent_Registry**: The core component that holds all registered Agent modules and resolves them by ID at runtime.
- **Enabled_Agents**: The set of Agent modules selected by the user for a given Container, specified via CLI flag or tool configuration.
- **Credential_Store**: The directory on the Host where an Agent's authentication tokens are persisted between Sessions. Each Agent module declares its own default Credential_Store path via the Agent_Interface.
- **Credential_Volume**: The Docker bind-mount that makes an Agent's Credential_Store accessible inside the Container at the path the Agent expects.
- **SSH_Port**: The TCP port on which the SSH_Server is reachable from the Host at `127.0.0.1`. In host network mode (Req 26, default), sshd listens directly on this port inside the Container. In bridge mode (`--host-network-off`), Docker maps the Container's port 22 to this port on the Host. The CLI selects the SSH_Port by starting at `2222` and incrementing by 1 until a free port is found on the Host. The selected port is persisted in the Tool_Data_Dir for the project so the same port is reused on subsequent runs. Can be overridden per invocation via `--port`.
- **Container_User_Home**: The home directory of the Container_User inside the Container. Defined as the Host_User's home directory path (e.g. `/home/alice`). This ensures absolute paths stored by tools resolve identically inside the Container and on the Host.
- **Tool_Data_Dir**: The directory on the Host where the CLI stores all persistent data for a given project (SSH host keys, SSH port assignment, agent manifests). Located at `~/.config/bootstrap-ai-coding/<container-name>/`.
- **Known_Hosts_File**: The SSH client's `~/.ssh/known_hosts` file on the Host.
- **Known_Hosts_Entry**: A line in the Known_Hosts_File that associates a host pattern (`[localhost]:<SSH_Port>` or `127.0.0.1:<SSH_Port>`) with the Container's SSH host public key.
- **SSH_Config_File**: The SSH client configuration file at `~/.ssh/config` on the Host.
- **Container_Name**: The Docker container name assigned by the tool, derived from the project directory name with a `bac-` prefix (e.g. `bac-my-project`). The name is human-readable and collision-resistant; see Requirement 5 for the full resolution algorithm.
- **SSH_Config_Entry**: A `Host` stanza in the SSH_Config_File managed by the tool, identified by a `Host` value matching the Container name (e.g. `bac-my-project`).
- **Image_Build_Timeout**: The maximum wall-clock duration the CLI will wait for a Container_Image build to complete before cancelling it. Defined as `constants.ImageBuildTimeout` (8 minutes). Agent installation steps (Node.js, npm packages) are legitimately slow on a cold cache, but a build that exceeds this limit is assumed to be hung and is terminated.
- **Verbose_Mode**: The operating mode activated by the `--verbose` (`-v`) flag. When Verbose_Mode is active, all Docker build output (layer-by-layer progress, `RUN` step output, etc.) is streamed to stdout in real time during a Container_Image build. When Verbose_Mode is inactive (the default), the build runs silently and only the "Building image..." message is shown.
- **Host_Git_Config**: The git configuration file at `~/.gitconfig` on the Host. If present, its contents are injected into the Container_Image at build time as a read-only file at `<Container_User_Home>/.gitconfig`. This provides the Container_User with the Host_User's git identity and preferences (author name, email, aliases, etc.) without requiring manual configuration inside the Container.
- **Restart_Policy**: The Docker restart policy applied to the Container at creation time. Controls whether the Container automatically restarts after a host reboot or daemon restart. Valid values: `no`, `always`, `unless-stopped`, `on-failure`. Default: `unless-stopped`.

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

1. WHEN a Container is started, THE SSH_Server SHALL be running and listening on the SSH_Port. In host network mode (Req 26, default), sshd listens on `127.0.0.1:<SSH_Port>` directly. In bridge mode (`--host-network-off`), sshd listens on port 22 and Docker maps it to the SSH_Port on the Host.
2. WHILE the Container is running, THE SSH_Server SHALL accept incoming connections on the SSH_Port from localhost.
3. IF the SSH_Server fails to start inside the Container, THEN THE CLI SHALL stop the Container, print a descriptive error message to stderr, and exit with a non-zero exit code.

---

### Requirement 4: Passwordless SSH Authentication via Public Key

**User Story:** As a developer, I want my SSH public key trusted inside the container, so that I can connect without entering a password.

#### Acceptance Criteria

1. THE CLI SHALL read the user's Public_Key from a path supplied via the `--ssh-key` option (highest precedence), or from `~/.ssh/id_ed25519.pub`, or from `~/.ssh/id_rsa.pub`, in that order of precedence (first found wins).
2. WHEN a Container is started, THE CLI SHALL install the Public_Key into the Container's `~/.ssh/authorized_keys` for the Container_User.
3. WHEN a user connects to the SSH_Server using the corresponding private key, THE SSH_Server SHALL authenticate the connection without prompting for a password.
4. IF no Public_Key can be located, THEN THE CLI SHALL print a descriptive error message to stderr and exit with a non-zero exit code.
5. THE SSH_Server SHALL disable password-based authentication inside the Container.

---

### Requirement 5: Container Lifecycle Management

**User Story:** As a developer, I want the container to be managed predictably so I don't accumulate stale containers and can always reconnect to an existing session.

#### Acceptance Criteria

1. WHEN the CLI starts a Container, THE CLI SHALL assign the Container a human-readable, collision-resistant name derived from the Project_Path using the following algorithm:
   a. Extract the directory name (last path component) and the parent directory name (second-to-last path component) from the absolute Project_Path. If the project is at the filesystem root (no parent), use `root` as the parent directory name.
   b. Sanitize each component: convert to lowercase; replace any character not in `[a-z0-9.-]` with `-` (note: `_` is intentionally excluded from the allowed set as it is reserved as the separator between `<parentdir>` and `<dirname>`); collapse consecutive `-` characters into one; trim leading and trailing `-` characters.
   c. Try candidate names in order, checking only against existing containers whose names start with `constants.ContainerNamePrefix` (`bac-`):
      - Candidate 1: `bac-<dirname>`
      - Candidate 2: `bac-<parentdir>_<dirname>`
      - Candidate 3+: `bac-<parentdir>_<dirname>-2`, `bac-<parentdir>_<dirname>-3`, … (incrementing by 1 until a free name is found)
   d. The first candidate not already in use is assigned as the Container name. The name is implicitly persisted by the existence of the Tool_Data_Dir directory (`~/.config/bootstrap-ai-coding/<container-name>/`) created for that project — on subsequent invocations the tool finds the existing Tool_Data_Dir and reuses the name encoded in its path. Note: renaming the project directory on disk changes the derived name and is treated as a new project; the old Tool_Data_Dir is not automatically cleaned up.
2. IF a Container with the derived name already exists (running or stopped) when the CLI is invoked, THE CLI SHALL reconnect to it: print the session summary (as defined in Requirement 17) and exit with a zero exit code without starting a duplicate. If the container was manually removed outside the tool, the Tool_Data_Dir will still exist and the tool will recreate the container reusing the same SSH host key and port.
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
5. WHEN the `--agents` flag is omitted, THE CLI SHALL enable `claude-code`, `augment-code`, and `build-resources` as the default Enabled_Agents (i.e. the default value of `--agents` is `constants.DefaultAgents`). See also BR-6 in `requirements-agents.md`.

---

### Requirement 8: Agent Lifecycle Orchestration

**User Story:** As a developer, I want the core to handle installation and credential setup for every enabled agent automatically, so I don't have to configure each agent manually.

#### Acceptance Criteria

1. WHEN a Container_Image is built, THE CLI SHALL call the installation procedure of every Enabled_Agent so each agent contributes its Dockerfile steps to the image.
2. THE Container_Image SHALL include installation steps only for Enabled_Agents; agents not in the Enabled_Agents set SHALL NOT be installed in the image.
3. WHEN a Container is started, THE CLI SHALL mount each Enabled_Agent's Credential_Store from the Host as a Credential_Volume at the path declared by that Agent via the Agent_Interface.
4. WHEN the Credential_Store directory for an Enabled_Agent does not exist on the Host, THE CLI SHALL create it before starting the Container.
5. IF an Enabled_Agent implements the optional `CredentialPreparer` interface, THE CLI SHALL call `PrepareCredentials(storePath)` after ensuring the Credential_Store directory exists and before starting the Container. This allows agents to synchronise external state (e.g. onboarding files stored outside the Credential_Store) into the mounted directory.
6. WHEN a Container is started and the Credential_Store for an Enabled_Agent contains no existing authentication tokens, THE CLI SHALL print a message to stdout identifying that Agent by name and instructing the user to authenticate it inside the Container.
7. Credentials written by an Agent inside the Container SHALL be immediately persisted to the Host Credential_Store via the bind-mount, and SHALL be available in future Sessions without re-authentication.

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
7. THE Container_User's username SHALL match the Host_User's username on the Host.
8. THE Container_User_Home inside the Container SHALL match the Host_User's home directory path on the Host, so that absolute paths in bind-mounted configuration files (e.g. plugin caches, marketplace metadata) resolve correctly inside the Container.

---

### Requirement 10a: Container User UID/GID Conflict Resolution

**User Story:** As a developer, I want the tool to detect and resolve UID/GID conflicts in the base image before building, so that the Container_User is always created correctly without silent failures.

#### Acceptance Criteria

1. BEFORE building the Container_Image, THE CLI SHALL inspect the Base_Container_Image to determine whether any existing user already occupies the Host_User's UID or GID.
2. IF no existing user in the Base_Container_Image has the Host_User's UID or GID, THE CLI SHALL proceed with normal Container_User creation (Requirement 10).
3. IF an existing user in the Base_Container_Image has the Host_User's UID or GID, THE CLI SHALL print a message to stdout identifying the conflicting username and its UID/GID, and ask the user whether that existing user may be renamed to the Container_User name.
4. IF the user confirms the rename, THE CLI SHALL generate Dockerfile steps that rename the conflicting user to the Container_User name (using `usermod -l`) instead of creating a new user, and proceed with the image build.
5. IF the user declines the rename, THE CLI SHALL print a descriptive error message to stderr explaining that the image cannot be built without resolving the UID/GID conflict, and exit with a non-zero exit code.
6. THE rename operation SHALL preserve the conflicting user's home directory contents and group memberships.

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
4. WHEN a Container is started, THE SSH_Server inside the Container SHALL be reachable on `127.0.0.1:<SSH_Port>` from the Host. In host network mode (Req 26), sshd listens directly on that address via sshd_config. In bridge mode, Docker port mapping binds the Container's port 22 to `127.0.0.1:<SSH_Port>` on the Host.
5. IF the persisted SSH_Port is in use on the Host by a process other than the Container for this project when the Container is started, THE CLI SHALL print a descriptive error message to stderr identifying the port conflict and exit with a non-zero exit code.
6. THE CLI SHALL print the SSH_Port and the full SSH connection command as part of the session summary defined in Requirement 17.

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
4. THE CLI SHALL support a `--rebuild` flag that forces a full Container_Image rebuild regardless of the existing manifest. WHEN `--rebuild` is used, THE CLI SHALL disable the Docker layer cache (`NoCache`) so that all Dockerfile steps are re-executed from scratch.
5. WHEN `--rebuild` is used and the Container is already running, THE CLI SHALL stop and remove the existing Container before creating a new one from the rebuilt image.
6. WHEN a rebuild is triggered (automatically or via `--rebuild`), THE CLI SHALL print a message to stdout indicating that the image is being built.
7. IF the image build fails, THE CLI SHALL print the build output to stderr and exit with a non-zero exit code.
8. THE CLI SHALL enforce a maximum build duration of the Image_Build_Timeout. IF the build exceeds this deadline, THE CLI SHALL cancel the build, print a descriptive error message to stderr identifying the timeout, and exit with a non-zero exit code.

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
   - **SSH connect**: the SSH alias command (e.g. `ssh bac-<container-name>`), which relies on the SSH_Config_Entry maintained by Requirement 19
   - **Enabled agents**: the list of Enabled_Agent identifiers
3. THE session summary SHALL be printed as plain text to stdout, with one field per line.
4. THE session summary SHALL be printed after all startup checks pass and the Container is confirmed ready.

---

### Requirement 18: SSH known_hosts Consistency

**User Story:** As a developer, I want the tool to keep my `~/.ssh/known_hosts` in sync with the container's SSH host key, so that I never get a spurious "host key changed" warning and am not trained to ignore SSH security alerts.

#### Acceptance Criteria

1. WHEN a Container is successfully started (or is already running), THE CLI SHALL check the Known_Hosts_File for entries matching both `[localhost]:<SSH_Port>` and `127.0.0.1:<SSH_Port>`.
2. IF no matching entry exists for either host pattern, THE CLI SHALL append the correct entries (derived from the persisted SSH host key in the Tool_Data_Dir) for both `[localhost]:<SSH_Port>` and `127.0.0.1:<SSH_Port>` to the Known_Hosts_File.
3. IF matching entries exist and they match the persisted SSH host key, THE CLI SHALL leave the Known_Hosts_File unchanged.
4. IF matching entries exist but do NOT match the persisted SSH host key, THE CLI SHALL prompt the user asking whether to replace the stale entries; IF the user confirms, THE CLI SHALL remove the stale entries and append the correct ones, then print a message to stdout confirming the update; IF the user declines, THE CLI SHALL print a warning to stdout and continue without modifying the Known_Hosts_File.
5. IF the Known_Hosts_File does not exist, THE CLI SHALL create it with permissions `0600` before writing.
6. THE CLI SHALL NOT modify any entries in the Known_Hosts_File other than those matching `[localhost]:<SSH_Port>` and `127.0.0.1:<SSH_Port>` for the current project's SSH_Port.
7. WHEN `--stop-and-remove` is used and a Container is successfully stopped and removed, THE CLI SHALL remove the Known_Hosts_Entries for that project's SSH_Port (both `[localhost]:<SSH_Port>` and `127.0.0.1:<SSH_Port>` forms) from the Known_Hosts_File, if present.
8. WHEN `--purge` completes successfully, THE CLI SHALL remove all Known_Hosts_Entries for all SSH_Ports managed by the tool from the Known_Hosts_File.
9. WHEN `--no-update-known-hosts` is provided, THE CLI SHALL skip all Known_Hosts_File modifications described in this requirement and print a notice to stdout that `known_hosts` management is disabled.

---

### Requirement 19: SSH Config Entry Management

**User Story:** As a developer, I want the tool to maintain an entry in `~/.ssh/config` for each container, so that I can connect with a simple `ssh bac-<dirname>` alias without specifying the port, user, or hostname manually.

#### Acceptance Criteria

1. WHEN a Container is successfully started (or is already running), THE CLI SHALL check `~/.ssh/config` for an entry whose `Host` value matches the Container name (e.g. `bac-my-project`).
2. IF no matching entry exists, THE CLI SHALL append an SSH_Config_Entry for the Container to `~/.ssh/config` with the following fields:
   - `Host <container-name>`
   - `HostName localhost`
   - `Port <SSH_Port>`
   - `User <host-username>` (the Host_User's username, which matches the Container_User)
   - `StrictHostKeyChecking yes` — `IdentityFile` is intentionally omitted: the container already has the user's public key installed in `authorized_keys` (Requirement 4), and the host key is kept consistent in `known_hosts` (Requirement 18), so SSH will authenticate and verify correctly without an explicit key path in the config entry.
3. IF a matching entry exists and all fields match the current SSH_Port and Container_User, THE CLI SHALL leave `~/.ssh/config` unchanged.
4. IF a matching entry exists but one or more fields do not match the current values (e.g. the SSH_Port changed), THE CLI SHALL replace the stale entry with the correct one and print a message to stdout confirming the update.
5. IF `~/.ssh/config` does not exist, THE CLI SHALL create it with permissions `0600` before writing.
6. THE CLI SHALL NOT modify any entries in `~/.ssh/config` other than the entry whose `Host` value matches the current Container name.
7. WHEN `--stop-and-remove` is used and a Container is successfully stopped and removed, THE CLI SHALL remove the SSH_Config_Entry for that Container from `~/.ssh/config`, if present.
8. WHEN `--purge` completes successfully, THE CLI SHALL remove all SSH_Config_Entries whose `Host` value starts with `constants.ContainerNamePrefix` (`bac-`) from `~/.ssh/config`.
9. WHEN `--no-update-ssh-config` is provided, THE CLI SHALL skip all `~/.ssh/config` modifications described in this requirement and print a notice to stdout that SSH config management is disabled.

---

### Requirement 20: Verbose Docker Build Output

**User Story:** As a developer, I want to see Docker build output in real time when I choose to, so that I can monitor progress and diagnose slow or failing builds without having to guess what is happening.

#### Acceptance Criteria

1. THE CLI SHALL accept a `--verbose` flag (short form: `-v`) that activates Verbose_Mode for the current invocation.
2. WHEN `--verbose` is NOT set (default), THE CLI SHALL run the Container_Image build silently: only the "Building image..." message is printed to stdout, and all Docker build output is discarded after being read (to drain the stream and detect errors).
3. WHEN `--verbose` IS set, THE CLI SHALL stream all Docker build output to stdout in real time as each JSON-encoded build message is received from the Docker daemon, including layer-by-layer progress lines and `RUN` step output.
4. WHEN `--verbose` IS set and the build fails, THE CLI SHALL still print the build error to stderr and exit with a non-zero exit code (consistent with Req 14.6).
5. THE `--verbose` flag SHALL only be valid in START mode; it is a START-only flag subject to the same CLI-3 constraint as `--rebuild`, `--agents`, `--port`, `--ssh-key`, `--no-update-known-hosts`, and `--no-update-ssh-config`.
6. WHEN `--verbose` is set but no build is triggered (the existing image matches the manifest and `--rebuild` is not set), THE CLI SHALL NOT print any Docker build output — there is nothing to stream.

---

### Requirement 21: Dockerfile Instruction Ordering for Layer Cache Efficiency

**User Story:** As a developer, I want the container image to build quickly on repeated invocations, so that I don't wait minutes for a rebuild when nothing has changed.

#### Acceptance Criteria

1. THE `DockerfileBuilder` SHALL place all `RUN` installation steps (base packages, agent installs, manifest write) **before** the `CMD` instruction in the generated Dockerfile.
2. THE `CMD ["/usr/sbin/sshd", "-D"]` instruction SHALL always be the **last** instruction in the generated Dockerfile.
3. THE `DockerfileBuilder.NewDockerfileBuilder()` constructor SHALL NOT append the `CMD` instruction. Instead, a separate `Finalize()` method SHALL append it.
4. THE caller (`cmd/root.go`) SHALL call `Finalize()` only after all agent `Install()` steps and the manifest `RUN` step have been appended to the builder.
5. Agent modules SHALL append only `RUN` (and optionally `ENV`, `COPY`) instructions via `Install()` — never `CMD` or `FROM`.

> **Rationale:** Docker's layer cache is sequential. Any instruction that changes invalidates all layers below it. Placing `CMD` before agent `RUN` steps means every agent installation step runs uncached on every build, even when the agent configuration has not changed. With `CMD` last, all `RUN` layers are stable and cached after the first build, reducing subsequent build times from minutes to seconds.

---

### Requirement 22: Dynamic Container User Identity

**User Story:** As a developer, I want the container user's username and home directory to match my host user exactly, so that absolute paths in bind-mounted configuration files (e.g. Claude Code plugin caches, marketplace git metadata) resolve correctly inside the container without path translation.

#### Acceptance Criteria

1. THE Container_User's username SHALL be the Host_User's username (not a hardcoded value).
2. THE Container_User_Home SHALL be the Host_User's home directory path (not a hardcoded value).
3. THE Container_User username and Container_User_Home SHALL be determined before any Docker or SSH operations begin and used consistently across all operations (Dockerfile generation, credential mount paths, SSH config entries).
4. THE CLI SHALL only support Linux hosts. No macOS home directory path translation is required.


---

### Requirement 23: Container Hostname Matches Container Name

**User Story:** As a developer, I want the hostname inside the container to match the container name, so that my shell prompt shows which project session I am in when I SSH into the container.

#### Acceptance Criteria

1. WHEN a Container is created, THE CLI SHALL set the Container's hostname to the Container_Name.
2. WHEN a user runs the `hostname` command inside the Container, THE output SHALL be the Container_Name.
3. THE Container hostname SHALL be set via the Docker SDK `Hostname` field in the container configuration passed to `ContainerCreate`.
4. THE CLI SHALL NOT override the default bash PS1 behaviour — the default Ubuntu shell prompt configuration (which includes `\h`) SHALL be sufficient to display the Container_Name in the prompt.

---

### Requirement 24: Git Configuration Forwarding

**User Story:** As a developer, I want my host `~/.gitconfig` to be available inside the container, so that git operations (commits, pushes, rebases) use my identity and preferences without manual setup inside the container.

#### Acceptance Criteria

1. WHEN a Container_Image is built, THE CLI/startup layer SHALL read the Host_User's `~/.gitconfig` file and pass its contents as the `gitConfig` string parameter to `DockerfileBuilder`. THE `DockerfileBuilder` SHALL inject the provided `gitConfig` into the Container_Image at `<Container_User_Home>/.gitconfig`. THE `DockerfileBuilder` itself SHALL NOT perform any filesystem I/O to read `~/.gitconfig` — it receives the content as a pre-read parameter.
2. THE injected `.gitconfig` file inside the Container_Image SHALL be owned by the Container_User.
3. THE injected `.gitconfig` file inside the Container_Image SHALL have permissions `0444` (read-only for all; the Container_User SHALL NOT be able to write to it).
4. IF the Host_User's `~/.gitconfig` file does not exist on the Host at build time, THE CLI/startup layer SHALL pass an empty string as `gitConfig`, and THE `DockerfileBuilder` SHALL skip the git configuration injection silently (no error, no warning, no Dockerfile instruction emitted).
5. WHEN `--rebuild` is used, THE CLI/startup layer SHALL re-read the current Host_User's `~/.gitconfig` and pass the latest content to `DockerfileBuilder` via the `gitConfig` parameter.
6. THE injection mechanism SHALL use base64 encoding within a `RUN` instruction (not `COPY`) so that the Dockerfile remains self-contained — no external build context files are required. This keeps the builder's output a single string, consistent with how SSH host keys and the keyring script are injected.

---

### Requirement 25: Container Restart Policy

**User Story:** As a developer, I want my container to automatically restart after a host reboot, so that my AI coding session is available again without me having to manually re-run the tool.

#### Acceptance Criteria

1. THE CLI SHALL accept a `--docker-restart-policy` flag whose value is one of the Docker restart policy names: `no`, `always`, `unless-stopped`, `on-failure`.
2. WHEN the `--docker-restart-policy` flag is omitted, THE CLI SHALL default to `unless-stopped`.
3. WHEN a Container is created, THE CLI SHALL set the Docker `RestartPolicy` in the container's `HostConfig` to the value specified by `--docker-restart-policy`.
4. WHEN the restart policy is `unless-stopped`, THE Container SHALL automatically restart after a host reboot if it was running at the time of shutdown. A Container that was explicitly stopped (via `--stop-and-remove` or `docker stop`) SHALL NOT restart after reboot.
5. WHEN the restart policy is `always`, THE Container SHALL restart after a host reboot regardless of its previous state.
6. WHEN the restart policy is `no`, THE Container SHALL NOT restart automatically under any circumstances.
7. WHEN the restart policy is `on-failure`, THE Container SHALL restart only if it exited with a non-zero exit code.
8. IF the `--docker-restart-policy` flag is provided with an invalid value (not one of `no`, `always`, `unless-stopped`, `on-failure`), THEN THE CLI SHALL print a descriptive error message to stderr listing the valid values and exit with a non-zero exit code.
9. THE `--docker-restart-policy` flag SHALL only be valid in START mode; it is a START-only flag subject to the CLI-3 constraint.
10. WHEN a Container already exists and the CLI reconnects to it, THE CLI SHALL NOT modify the existing Container's restart policy — the policy is set only at Container creation time.

---

### Requirement 26: Host Network Mode

**User Story:** As a developer, I want the container to share the host's network namespace by default, so that services running on the host (e.g. Vibe Kanban on localhost:3000) are directly accessible from inside the container, and services started inside the container are directly accessible from the host browser — without any additional port forwarding configuration. I also want the option to disable host networking and fall back to bridge mode with port mapping if needed.

#### Acceptance Criteria

1. THE CLI SHALL accept a `--host-network-off` flag (boolean, default: absent/false) that disables Docker host network mode. When `--host-network-off` is NOT set (the default), host network mode is active. When `--host-network-off` IS set, the Container uses bridge networking with port mapping.
2. WHEN `--host-network-off` is NOT set (the default), THE CLI SHALL configure the Container with Docker host network mode (`NetworkMode: "host"`), so the Container shares the Host's network namespace.
3. WHEN `--host-network-off` is NOT set, THE SSH_Server inside the Container SHALL be configured to listen on `127.0.0.1:<SSH_Port>` via sshd_config directives (`Port <SSH_Port>` and `ListenAddress 127.0.0.1`) instead of the default port 22.
4. WHEN `--host-network-off` is NOT set, Docker port binding configuration (`PortBindings`, `ExposedPorts`) SHALL NOT be set on the Container — they are ignored by Docker in host network mode and would produce a warning.
5. WHEN `--host-network-off` is NOT set, THE Container SHALL be able to reach any TCP/UDP service listening on the Host's loopback or external interfaces without additional configuration.
6. WHEN `--host-network-off` IS set, THE CLI SHALL configure the Container with the default Docker bridge network and SHALL map the Container's internal SSH port (`constants.ContainerSSHPort`, port 22) to the SSH_Port on the Host via Docker port bindings (`HostIP: constants.HostBindIP`, `HostPort: SSH_Port`).
7. WHEN `--host-network-off` IS set, THE SSH_Server inside the Container SHALL listen on port 22 (the default sshd port). The sshd_config SHALL NOT include `Port` or `ListenAddress` directives — sshd uses its defaults.
8. THE existing SSH_Port selection logic (Requirement 12) SHALL remain unchanged regardless of `--host-network-off` — the port is auto-selected starting at `constants.SSHPortStart` (2222), incrementing until free, and persisted in the Tool_Data_Dir.
9. THE `--port` flag SHALL continue to override the SSH_Port regardless of `--host-network-off`.
10. THE `WaitForSSH` function SHALL connect to `127.0.0.1:<SSH_Port>` to verify SSH readiness in both modes (in host mode sshd listens directly on that port; in bridge mode Docker maps it).
11. THE `HostBindIP` constant (`127.0.0.1`) SHALL be used as the sshd bind address in host mode and as the Docker port binding IP in bridge mode, ensuring SSH is only accessible from localhost in both cases.
12. THE `--host-network-off` flag SHALL only be valid in START mode; it is a START-only flag subject to the CLI-3 constraint.
13. THE `--host-network-off` value SHALL influence the Instance_Image build: when absent (host mode), sshd_config includes `Port <SSH_Port>` and `ListenAddress 127.0.0.1`; when set (bridge mode), these directives are omitted.
14. WHEN `--host-network-off` is changed between invocations for the same project (e.g. added or removed), THE CLI SHALL require `--rebuild` to regenerate the Instance_Image with the correct sshd_config. IF the network mode has changed and `--rebuild` is not set, THE CLI SHALL print a message instructing the user to run with `--rebuild` and exit with a zero exit code.
