# Requirements: Two-Layer Image Architecture

## Introduction

This feature splits the current monolithic Container_Image build (core Req 14) into a two-layer architecture: a shared **Base_Image** built once and reused across all projects, and a thin **Instance_Image** built per-project containing only SSH keys and sshd configuration. The goal is near-instant per-project startup when the Base_Image already exists and matches the current agent configuration.

> **Related documents:**
> - `requirements-core.md` — core requirements (Container_Image, Agent_Interface, Tool_Data_Dir, etc.)
> - `requirements-agents.md` — agent module requirements (CC-2, AC-2, BR-2 installation steps)

## Glossary

> All terms from `requirements-core.md` and `requirements-agents.md` apply here unchanged. Only new terms introduced by this feature are listed below.

- **Base_Image**: The shared Docker image tagged `bac-base:latest`. Contains OS packages, Container_User, all Enabled_Agents, gnome-keyring, and Host_Git_Config. Built once, reused across all projects. Replaces the monolithic Container_Image for the heavy layers.
- **Instance_Image**: The per-project Docker image tagged `<Container_Name>:latest`, built `FROM Base_Image`. Contains only the project's SSH host key, authorized_keys, sshd hardening, and CMD. This is what containers run from.

## Requirements

### Requirement TL-1: Base Image Construction

**User Story:** As a developer, I want a shared base image that contains all heavy dependencies, so that per-project container startup is near-instant.

> Supersedes the monolithic build in core Req 14.1. Agent installation steps (CC-2, AC-2, BR-2) are baked into the Base_Image.

#### Acceptance Criteria

1. WHEN a Base_Image build is needed (no `bac-base:latest` exists, or `--rebuild` is set), THE Builder SHALL produce a Dockerfile starting with `FROM constants.BaseContainerImage` (core Req 9).
2. THE Base_Image SHALL contain openssh-server and sudo (core Req 3).
3. THE Base_Image SHALL contain Container_User with UID/GID matching Host_User (core Req 10).
4. THE Base_Image SHALL contain all Enabled_Agents installed via `Install()` (core Req 8.1).
5. THE Base_Image SHALL contain D-Bus and gnome-keyring with the keyring profile script (CC-7).
6. IF Host_Git_Config exists, THE Base_Image SHALL contain it in Container_User_Home. IF absent, skip without error.
7. THE Base_Image SHALL contain the manifest at `constants.ManifestFilePath` (core Req 14.2).
8. THE Base_Image SHALL be tagged `bac-base:latest`.
9. THE Base_Image SHALL carry labels `bac.manifest` (JSON agent IDs) and `bac.managed` = `"true"`.
10. THE Base_Image SHALL NOT contain SSH host keys, authorized_keys, sshd_config hardening, or CMD — those belong in the Instance_Image (TL-2).
11. IF the build exceeds Image_Build_Timeout, cancel and return a timeout error (core Req 14.8).

### Requirement TL-2: Instance Image Construction

**User Story:** As a developer, I want a thin per-project image that layers only SSH configuration on top of the base, so that rebuilds are near-instant.

> Contains per-project steps previously in the monolithic Container_Image: SSH host key (core Req 13), authorized_keys (core Req 4), sshd hardening.

#### Acceptance Criteria

1. WHEN an Instance_Image build is needed (no existing image, base was rebuilt, or `--rebuild`), THE Builder SHALL produce a Dockerfile starting with `FROM bac-base:latest`.
2. THE Instance_Image SHALL inject the project's persisted SSH host key pair (core Req 13) into `/etc/ssh/ssh_host_ed25519_key` (`0600`) and `.pub` (`0644`).
3. THE Instance_Image SHALL inject Public_Key (core Req 4) into `<Container_User_Home>/.ssh/authorized_keys` (`0600`), directory at `0700`, owned by Container_User.
4. THE Instance_Image SHALL append sshd_config directives: `PasswordAuthentication no`, `PermitRootLogin no`, `PubkeyAuthentication yes`. WHEN host network mode is active (default, `--host-network-off` NOT set), THE Instance_Image SHALL additionally append `Port <SSH_Port>` and `ListenAddress 127.0.0.1` to configure sshd to listen on the project's SSH_Port bound to localhost only. WHEN bridge mode is active (`--host-network-off` IS set), these port/address directives SHALL be omitted (sshd uses its default port 22).
5. THE Instance_Image SHALL create `/run/sshd`.
6. THE Instance_Image SHALL set `CMD ["/usr/sbin/sshd", "-D"]` as the final instruction. sshd reads the port and bind address from sshd_config; no `-p` flag is needed in CMD.
7. THE Instance_Image SHALL be tagged `<Container_Name>:latest`.
8. THE Instance_Image SHALL carry labels `bac.managed` = `"true"` and `bac.container` = Container_Name.

### Requirement TL-3: Base Image Cache Detection

**User Story:** As a developer, I want the CLI to skip the base image build when it already exists and matches my agent configuration, so that startup is fast.

> Refines core Req 14.3 for the two-layer model.

#### Acceptance Criteria

1. On ModeStart, THE CLI SHALL inspect `bac-base:latest` locally.
2. IF absent, trigger a Base_Image build before building the Instance_Image.
3. IF present and `bac.manifest` label matches Enabled_Agents, skip the Base_Image build.
4. IF present but `bac.manifest` does not match, print a message instructing `--rebuild` and exit 0 (core Req 14.3 UX).
5. IF `bac.manifest` label is absent or invalid JSON, trigger a Base_Image build.
6. `--rebuild` overrides all cache checks — always rebuild both images.

### Requirement TL-4: Instance Image Cache Detection

**User Story:** As a developer, I want the CLI to skip the instance image build when it already exists and the base has not changed, so that reconnecting is instant.

#### Acceptance Criteria

1. IF `<Container_Name>:latest` exists locally and Base_Image was NOT rebuilt this invocation, skip the Instance_Image build.
2. IF `<Container_Name>:latest` does not exist, build the Instance_Image.
3. IF Base_Image was rebuilt this invocation, always rebuild the Instance_Image.
4. `--rebuild` forces Instance_Image rebuild regardless.
5. IF `--rebuild` and a container is running, stop and remove it first (core Req 14.5).

### Requirement TL-5: --rebuild Flag Behavior

**User Story:** As a developer, I want `--rebuild` to force a complete rebuild of both layers, so that I can recover from a corrupted or stale state.

> Extends core Req 14.4 to cover both layers.

#### Acceptance Criteria

1. Build Base_Image with Docker no-cache, enforcing Image_Build_Timeout.
2. Build Instance_Image from scratch after Base_Image completes.
3. IF a container exists (running or stopped), stop and remove it before building.
4. IF no container exists, proceed directly to build.
5. After both builds succeed, create and start a new container, print session summary (core Req 17).

### Requirement TL-6: --stop-and-remove Flag Behavior

**User Story:** As a developer, I want `--stop-and-remove` to remove only the container without deleting any images, so that I can restart quickly.

> Unchanged from core Req 5.3–5.4 except explicitly stating images are preserved.

#### Acceptance Criteria

1. Stop and remove the container, print confirmation, exit 0 (core Req 5.3).
2. Do NOT remove Base_Image or Instance_Image.
3. Remove Known_Hosts_Entries for the project's SSH_Port (core Req 18.7).
4. Remove SSH_Config_Entry for the project's Container_Name (core Req 19.7).
5. IF no container exists, print a message and exit 0 (core Req 5.4).

### Requirement TL-7: --purge Flag Behavior

**User Story:** As a developer, I want `--purge` to remove everything, so that I can cleanly uninstall.

> Extends core Req 16 to explicitly cover both Base_Image and all Instance_Images.

#### Acceptance Criteria

1. Prompt for `yes` confirmation (core Req 16.5).
2. IF confirmed: stop/remove all bac-managed containers, remove all bac-managed images (Base_Image + all Instance_Images), delete Tool_Data_Dir root, remove all Known_Hosts_Entries, remove all SSH_Config_Entries.
3. IF not confirmed, print "Purge cancelled." and exit 0.
4. IF removal of an individual item fails, print warning to stderr and continue.

### Requirement TL-8: Agent Manifest Change Detection

**User Story:** As a developer, I want the CLI to detect when my enabled agents have changed and notify me, so that the container reflects my configuration.

> Refines core Req 14.2–14.3 for the two-layer model where the manifest lives on the Base_Image.

#### Acceptance Criteria

1. Compute manifest as a sorted JSON array of Enabled_Agent IDs.
2. Compare against `bac.manifest` label on existing Base_Image.
3. IF mismatch, print message instructing `--rebuild` and exit 0.
4. IF match, skip Base_Image build.
5. IF label absent or no Base_Image exists, build it.
6. `--rebuild` overrides — always rebuild.

### Requirement TL-9: UID/GID Conflict Handling

**User Story:** As a developer, I want the base image build to handle UID/GID conflicts the same way the current build does.

> Identical to core Req 10a, now applies only during Base_Image builds.

#### Acceptance Criteria

1. Before building Base_Image, run `FindConflictingUser` (core Req 10a.1).
2. IF conflict found, prompt for rename (core Req 10a.3).
3. IF confirmed, use `UserStrategyRename`.
4. IF declined or error, abort with descriptive error.

### Requirement TL-10: Build Output and Verbosity

**User Story:** As a developer, I want to see build progress for both layers.

> Extends core Req 14.6–14.8 for two build steps.

#### Acceptance Criteria

1. Print "Building base image..." before Base_Image build.
2. Print "Building instance image..." before Instance_Image build.
3. In Verbose_Mode, stream build output for both.
4. On success without Verbose_Mode, suppress output.
5. On failure, print build output to stderr and exit 1.
6. On timeout, treat as failure (core Req 14.8).

### Requirement TL-11: Image Naming Constant

**User Story:** As a developer, I want the base image name defined as a constant for consistency.

#### Acceptance Criteria

1. `internal/constants` SHALL define `BaseImageName` = `"bac-base"`.
2. Base_Image_Tag is derived as `constants.BaseImageName + ":latest"`.
3. Instance_Image_Tag remains `<Container_Name> + ":latest"` (unchanged).
