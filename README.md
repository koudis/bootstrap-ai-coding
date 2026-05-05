# bootstrap-ai-coding

`bootstrap-ai-coding` (`bac`) is a Go CLI tool that provisions an isolated Docker container for AI-assisted coding sessions.

Primarirly designed to work with Visual Studio Code but it works any IDE with code-over-ssh. 

## Install

```
# arm64
wget https://github.com/koudis/bootstrap-ai-coding/releases/latest/download/bac-linux-arm64 -o bac

# amd64 
wget https://github.com/koudis/bootstrap-ai-coding/releases/latest/download/bac-linux-amd64 -o bac
```

## How to run

1. `bac <project_path>`
1. open Visual Studio Code, press Ctrl+Shift+P, run Remote-SSH and choose bac-<project_folder_name> target to connect.
1. <project_path> can be found under /workspace 

where project_folder_name is a name of the bottom-most folder in project_path. (/my/nice/project --> project)

## How it works

The tool:

1. Checks you are not running as root and that Docker is available (≥ 20.10)
2. Builds a Docker image on demand (`ubuntu:26.04` base, SSH server, non-root `dev` user, enabled AI agents)
3. Mounts your project directory into the container at `/workspace`
4. Attaches a named Docker volume for VS Code Server persistence (no re-download on reconnect)
5. Starts the container with an SSH server bound to a persisted port (default: 2222+)
6. Keeps `~/.ssh/known_hosts` and `~/.ssh/config` in sync so you can connect immediately
7. Prints a session summary

```
Data directory:  ~/.config/bootstrap-ai-coding/bac-myproject/
Project directory: /home/user/myproject
SSH port:        2222
SSH connect:     ssh bac-myproject
Enabled agents:  claude-code, augment-code
```

After that, `ssh bac-myproject` also works — no port or username to remember.

## Prerequisites

- Go 1.25+
- Docker daemon ≥ 20.10 running on the host
- An SSH public key at `~/.ssh/id_ed25519.pub` or `~/.ssh/id_rsa.pub` (or use `--ssh-key`)

## Installation

Build from source (requires Go 1.25+):

```bash
git clone https://github.com/koudis/bootstrap-ai-coding
cd bootstrap-ai-coding
make release
```

This produces two static, stripped binaries:

```
bac-linux-amd64   # Linux x86-64
bac-linux-arm64   # Linux arm64
```

## Usage

### Start a session

```bash
bac <project-path>
```

On first run, the image is built (takes a minute or two). Subsequent runs reconnect in seconds.

### Stop and remove the container

```bash
bac --stop-and-remove <project-path>
```

Stops and removes the container. Does not delete the image or tool data — the next `bac <project-path>` will reuse the existing image.

### Remove everything

```bash
bac --purge
```

Removes all bac-managed containers, images, named volumes, tool data (`~/.config/bootstrap-ai-coding/`), `known_hosts` entries, and SSH config entries. Requires confirmation.

## Flags

| Flag | Description |
|---|---|
| `<project-path>` | Path to the project directory to mount (required for start/stop) |
| `--agents <ids>` | Comma-separated agent IDs to enable (default: `claude-code,augment-code`) |
| `--port <n>` | Override the SSH port (1024–65535; default: auto-selected from 2222 upward) |
| `--ssh-key <path>` | Override the SSH public key path |
| `--rebuild` | Force a full container image rebuild |
| `--no-update-known-hosts` | Skip automatic `~/.ssh/known_hosts` management |
| `--no-update-ssh-config` | Skip automatic `~/.ssh/config` management |
| `--stop-and-remove` | Stop and remove the container for the given project |
| `--purge` | Remove all tool data, containers, images, and volumes (with confirmation) |
| `--version` | Print the version and exit |

## Supported Agents

Both agents are enabled by default. Use `--agents` to enable a specific subset.

| Agent ID | Tool | Credential store | Container mount |
|---|---|---|---|
| `claude-code` | [Claude Code](https://github.com/anthropics/claude-code) by Anthropic | `~/.claude/` | `/home/dev/.claude/` |
| `augment-code` | [Augment Code](https://www.augmentcode.com) (Auggie CLI) | `~/.augment/` | `/home/dev/.augment/` |

### Examples

```bash
# Both agents (default)
bac <project-path>

# Claude Code only
bac <project-path> --agents claude-code

# Augment Code only
bac <project-path> --agents augment-code
```

## Agents

Both **Claude Code** (`claude-code`) and **Augment Code** (`augment-code`) are enabled by default.

### Credential persistence

Credentials are stored on the host and bind-mounted into every container — so if you are already logged in on your host machine, the container inherits your credentials automatically with no extra login step. The tool only prompts you when no credentials are found:

```
Authenticate claude-code inside the container: run 'claude' and complete the login flow.
Authenticate augment-code inside the container: run 'auggie login' and complete the login flow.
```

Once you authenticate inside the container, the tokens are written back to the host-side credential store and reused for every future session across all projects.

### Adding a new agent

Agent modules are self-contained Go packages under `internal/agents/`. Adding a new agent requires no changes to core code — only a new package:

```
internal/agents/<name>/<name>.go
```

The package implements the `agent.Agent` interface and calls `agent.Register()` in its `init()` function. Then add a single blank import in `main.go`:

```go
import _ "github.com/koudis/bootstrap-ai-coding/internal/agents/myagent"
```

See [`internal/agents/claude/claude.go`](internal/agents/claude/claude.go) for a reference implementation and the [agent module guide](.kiro/steering/agent-module.md) for full instructions.

## Data directory

Per-project state is stored in `~/.config/bootstrap-ai-coding/<container-name>/`:

| File | Contents |
|---|---|
| `port` | Persisted SSH port for this project |
| `ssh_host_ed25519_key` | SSH host private key (generated once, reused across rebuilds) |
| `ssh_host_ed25519_key.pub` | SSH host public key |

The SSH host key is stable across rebuilds — no `known_hosts` churn.

## VS Code Server caching

A named Docker volume (`<container-name>-vscode-server`) is mounted at `/home/dev/.vscode-server` inside the container. This persists VS Code's Remote-SSH server binaries across container restarts and rebuilds — no re-download when you reconnect.

- The volume survives `--stop-and-remove` (only the container is removed)
- The volume is removed by `--purge` (full cleanup)

## Container naming

Container names follow the pattern `bac-<dirname>` derived from the project directory name. On collision, the tool falls back to `bac-<parentdir>_<dirname>`, then `bac-<parentdir>_<dirname>-2`, `-3`, and so on.

## Development

```bash
# Build (fast, dynamic)
go build ./...

# Static release binary
make release

# Unit and property-based tests
go test ./...

# Integration tests
# Integration tests (requires a running Docker daemon and explicit consent).
# The suite automatically removes the base image before running to ensure
# the pull path is exercised. No manual 'docker rmi' needed.
BAC_INTEGRATION_CONSENT=yes go test -tags integration -timeout 30m ./...

# Vet
go vet ./...

# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Coverage target: ~80% line coverage on all non-integration packages.

## Exit codes

| Condition | Exit code |
|---|---|
| Successful start or reconnect | 0 |
| `--stop-and-remove` (found or not found) | 0 |
| `--purge` completed or declined | 0 |
| Agent manifest mismatch (run with `--rebuild`) | 0 |
| Any error (missing path, Docker unavailable, bad flag, etc.) | 1 |

## License

Apache License 2.0
