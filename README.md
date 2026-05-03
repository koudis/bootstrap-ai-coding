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
4. Starts the container with an SSH server bound to a persisted port (default: 2222+)
5. Keeps `~/.ssh/known_hosts` and `~/.ssh/config` in sync so you can connect immediately
6. Prints a session summary

```
Data directory:  ~/.config/bootstrap-ai-coding/bac-myproject/
Project directory: /home/user/myproject
SSH port:        2222
SSH connect:     ssh bac-myproject
Enabled agents:  claude-code
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

Removes all bac-managed containers, images, tool data (`~/.config/bootstrap-ai-coding/`), `known_hosts` entries, and SSH config entries. Requires confirmation.

## Flags

| Flag | Description |
|---|---|
| `<project-path>` | Path to the project directory to mount (required for start/stop) |
| `--agents <ids>` | Comma-separated agent IDs to enable (default: `claude-code`) |
| `--port <n>` | Override the SSH port (1024–65535; default: auto-selected from 2222 upward) |
| `--ssh-key <path>` | Override the SSH public key path |
| `--rebuild` | Force a full container image rebuild |
| `--no-update-known-hosts` | Skip automatic `~/.ssh/known_hosts` management |
| `--no-update-ssh-config` | Skip automatic `~/.ssh/config` management |
| `--stop-and-remove` | Stop and remove the container for the given project |
| `--purge` | Remove all tool data, containers, and images (with confirmation) |

## Agents

The default agent is **Claude Code** (`claude-code`).

### Credential persistence

Credentials are stored on the host (e.g. `~/.claude/` for Claude Code) and bind-mounted into every container. **If you are already logged in on your host machine, the container inherits your credentials automatically — no login step required.** The tool only prompts you to authenticate when no credentials are found:

```
Authenticate claude-code inside the container: run 'claude' and complete the login flow.
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

# Integration tests (requires a running Docker daemon)
go test -tags integration ./...

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
