# CLI Flags Reference

All flags are defined in `cmd/root.go` using Cobra. This file is the authoritative reference for flag names, defaults, and the requirements they satisfy.

## Invocation

```
bootstrap-ai-coding <project-path> [flags]
bootstrap-ai-coding --stop-and-remove <project-path>
bootstrap-ai-coding --purge
```

## Flags

### `<project-path>` (positional, required for start)

The path to the project directory on the host. Mounted into the container at `constants.WorkspaceMountPath` (`/workspace`).

- Accepts absolute or relative paths
- Resolved to absolute path before use
- Must exist on the host filesystem
- **Validates:** Req 1.1, 1.4, 2.1

---

### `--agents <id[,id,...]>`

Comma-separated list of agent IDs to install in the container.

- Default: `constants.DefaultAgents` (`"claude-code,augment-code,build-resources"`)
- Example: `--agents claude-code`
- Example: `--agents augment-code`
- Example: `--agents claude-code,augment-code`
- Unknown IDs produce an error listing available agents
- **Validates:** Req 7.4, 7.5

---

### `--port <n>`

Override the SSH port bound on the host.

- Default: auto-selected starting at `constants.SSHPortStart` (`2222`), incrementing until free, then persisted
- When provided, the value is persisted in `Tool_Data_Dir` for the project
- **Validates:** Req 12.2, 12.3

---

### `--ssh-key <path>`

Override the SSH public key path.

- Default: auto-discovered in order: `~/.ssh/id_ed25519.pub` → `~/.ssh/id_rsa.pub`
- Must point to a readable public key file
- **Validates:** Req 4.1

---

### `--rebuild`

Force a full container image rebuild, ignoring the existing image and manifest.

- Without this flag: rebuild only happens when no image exists or the agent manifest has changed
- With this flag: always rebuilds
- **Validates:** Req 14.4

---

### `--no-update-known-hosts`

Skip all automatic `~/.ssh/known_hosts` management for this invocation.

- Default: known_hosts is kept in sync automatically (Req 18)
- When provided: no entries are added, updated, or removed; a notice is printed to stdout
- Only valid in START mode
- **Validates:** Req 18.9

---

### `--no-update-ssh-config`

Skip all automatic `~/.ssh/config` management for this invocation.

- Default: `~/.ssh/config` is kept in sync automatically (Req 19)
- When provided: no entries are added, updated, or removed; a notice is printed to stdout
- Only valid in START mode
- **Validates:** Req 19.9

---

### `--stop-and-remove`

Stop and remove the container for the given project path.

- Works whether the container is running or stopped
- Does nothing (with a message) if no container exists for the project
- Does **not** delete the `Tool_Data_Dir` or the container image
- Removes the `known_hosts` entries for the project's SSH_Port (both `localhost` and `127.0.0.1` forms)
- Removes the SSH config entry for the project's Container name from `~/.ssh/config`
- **Validates:** Req 5.3, 5.4, 18.7, 19.7

---

### `--purge`

Remove all data the tool has stored on the host.

- Stops and removes all bac-managed containers
- Removes all bac-managed Docker images
- Deletes the entire `~/.config/bootstrap-ai-coding/` directory
- Removes all `known_hosts` entries for all SSH_Ports managed by the tool
- Removes all SSH config entries for all bac-managed containers from `~/.ssh/config`
- Requires explicit confirmation before any destructive action
- **Validates:** Req 16.1–16.6, 18.8, 19.8

---

## Flag Combination Rules

See `requirements-cli-combinations.md` for the formal definition of valid and invalid flag combinations. Summary:

- `--stop-and-remove` and `--purge` are mutually exclusive (CLI-1)
- `<project-path>` is required for START and STOP modes, forbidden in PURGE mode (CLI-2)
- `--agents`, `--port`, `--ssh-key`, `--rebuild`, `--no-update-known-hosts` are only valid in START mode (CLI-3)
- `--port` must be in range 1024–65535 (CLI-5)
- `--agents` must resolve to at least one known agent ID (CLI-6)

## Exit Codes

| Condition | Exit code |
|---|---|
| Successful start or reconnect | 0 |
| `--stop-and-remove` (found or not found) | 0 |
| `--purge` completed or declined | 0 |
| Manifest mismatch (instruct `--rebuild`) | 0 |
| Any error (missing path, no Docker, bad flag, etc.) | 1 |
