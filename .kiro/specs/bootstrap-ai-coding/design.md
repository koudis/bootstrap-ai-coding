# Design Document: bootstrap-ai-coding

## Overview

`bootstrap-ai-coding` (bac) is a Go CLI tool that provisions an isolated Docker container for AI-assisted coding sessions. The user supplies a project path; the tool builds a container image on demand, mounts the project and per-agent credential stores, starts an SSH server, and prints the connection details.

## Key Design Goals

- **Zero-friction startup**: one command, one argument, ready to SSH in.
- **Pluggable agents**: the `Agent` interface and `AgentRegistry` decouple agent logic from the orchestration layer entirely.
- **Open/closed core**: adding a new agent requires only a new package in `agents/` — no core files change.
- **Reproducible containers**: deterministic naming and dynamic Dockerfile generation ensure consistent, idempotent behaviour.
- **Credential persistence**: per-agent bind-mounts keep authentication tokens alive across sessions.
- **Non-root safety**: the CLI refuses to run as root; containers run as a `dev` user whose UID/GID match the host user.
- **Stable SSH identity**: SSH host keys are generated once per project and reused across rebuilds, preventing `known_hosts` churn.
- **Persistent port assignment**: the SSH port is chosen once and remembered in the Tool_Data_Dir, so reconnecting is always the same command.
- **SSH config alias**: an entry is maintained in `~/.ssh/config` for each container so the user can connect with `ssh bac-<dirname>` — no port, user, or hostname to remember.

## Document Structure

The design is split across four files:

| File | Contents |
|---|---|
| `design.md` (this file) | Overview, key design goals, document index |
| [`design-architecture.md`](design-architecture.md) | Part 1 — Core: component diagram, package layout, startup/stop/purge sequences, all core component designs, data models, error handling |
| [`design-agents.md`](design-agents.md) | Part 2 — Agent modules: contract, Claude Code implementation, adding future agents |
| [`design-properties.md`](design-properties.md) | Correctness properties (Properties 1–42) and full testing strategy |

## Related Documents

- `requirements-core.md` — core application requirements (Req 1–19)
- `requirements-agents.md` — agent module requirements (CC-1–CC-6)
- `requirements-cli-combinations.md` — valid and invalid CLI flag combinations (CLI-1–CLI-6)
