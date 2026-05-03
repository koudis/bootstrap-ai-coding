# Product: bootstrap-ai-coding (bac)

`bootstrap-ai-coding` is a Go CLI tool that provisions an isolated Docker container for AI-assisted coding sessions.

## What it does

The user runs `bac <project-path>` and the tool:
1. Checks the user is not running as root
2. Verifies Docker is available and meets the minimum version requirement
3. Builds a Docker container image on demand (Base_Container_Image base, SSH server, non-root Container_User, enabled AI agents)
4. Mounts the project directory into the container at the WorkspaceMountPath
5. Starts the container with an SSH server bound to a persisted SSH port
6. Prints a session summary so the user can SSH in immediately

## Session Summary

On every successful start (or reconnect to an already-running container), the tool prints:

```
Data directory:  ~/.config/bootstrap-ai-coding/<container-name>/
Project directory: /path/to/project
SSH port:        2222
SSH connect:     ssh bac-<container-name>
Enabled agents:  claude-code, augment-code
```

## Key design goals

- **Zero-friction startup**: one command, one argument, ready to SSH in
- **Pluggable agents**: AI coding agents (Claude Code, Augment Code, etc.) are self-contained modules — adding a new agent requires no changes to core code
- **Credential persistence**: per-agent bind-mounts keep auth tokens alive across sessions; login once, never again
- **Non-root safety**: CLI refuses to run as root; containers run as Container_User with UID/GID matching the host user
- **Stable SSH identity**: SSH host keys are generated once per project and reused across rebuilds — no `known_hosts` churn
- **known_hosts consistency**: `~/.ssh/known_hosts` is kept in sync automatically; stale entries are detected and the user is prompted before replacement
- **Persistent port**: SSH port is chosen once per project and remembered — reconnecting is always the same command
- **Clean uninstall**: `--purge` removes all containers, images, tool data, and `known_hosts` entries with a confirmation prompt

## Primary user

Developers who want to run AI coding agents (Claude Code, Augment Code, etc.) in an isolated, reproducible container environment without manual Docker setup.

## CLI flags

| Flag | Description |
|---|---|
| `<project-path>` | (positional) Path to the project directory to mount |
| `--agents <ids>` | Comma-separated agent IDs to enable (default: `claude-code,augment-code`) |
| `--port <n>` | Override the SSH port (default: auto-selected from 2222 upward) |
| `--ssh-key <path>` | Override the SSH public key path |
| `--rebuild` | Force a full container image rebuild |
| `--no-update-known-hosts` | Skip automatic `~/.ssh/known_hosts` management for this invocation |
| `--no-update-ssh-config` | Skip automatic `~/.ssh/config` management for this invocation |
| `--stop-and-remove` | Stop and remove the container for the given project |
| `--purge` | Remove all tool data, containers, images, and known_hosts entries (with confirmation) |
