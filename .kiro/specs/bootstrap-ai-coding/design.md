# Design Document: bootstrap-ai-coding

## Overview

`bootstrap-ai-coding` (bac) is a Go CLI tool that provisions an isolated Docker container for AI-assisted coding sessions. The user supplies a project path; the tool builds a container image on demand, mounts the project and per-agent credential stores, starts an SSH server, and prints the connection details.

## Key Design Goals

- **Zero-friction startup**: one command, one argument, ready to SSH in.
- **Pluggable agents**: the `Agent` interface and `AgentRegistry` decouple agent logic from the orchestration layer entirely.
- **Open/closed core**: adding a new agent requires only a new package in `agents/` — no core files change.
- **Reproducible containers**: deterministic naming and dynamic Dockerfile generation ensure consistent, idempotent behaviour.
- **Credential persistence**: per-agent bind-mounts keep authentication tokens alive across sessions.
- **Non-root safety**: the CLI refuses to run as root; containers run as a user whose username, UID, and GID match the host user (resolved at runtime via `hostinfo`).
- **Stable SSH identity**: SSH host keys are generated once per project and reused across rebuilds, preventing `known_hosts` churn.
- **Persistent port assignment**: the SSH port is chosen once and remembered in the Tool_Data_Dir, so reconnecting is always the same command.
- **SSH config alias**: an entry is maintained in `~/.ssh/config` for each container so the user can connect with `ssh bac-<dirname>` — no port, user, or hostname to remember.

## Document Structure

The design is split across multiple focused files:

| File | Contents |
|---|---|
| `design.md` (this file) | Overview, key design goals, document index |
| [`design-architecture.md`](design-architecture.md) | Core architecture: component diagram, package layout, startup/stop/purge sequence diagrams |
| [`design-components.md`](design-components.md) | Core component designs: Constants, HostInfo, Agent Interface, AgentRegistry, DockerfileBuilder, Headless Keyring, Git Config Forwarding, Restart Policy, Base Image Inspection, Verbose Mode, Naming, SSH, DataDir |
| [`design-docker.md`](design-docker.md) | Two-layer Docker image architecture (TL-1 through TL-11): motivation, layer split, builder changes, build flow, cache detection |
| [`design-data-models.md`](design-data-models.md) | Core data models (Mode, Config, ContainerSpec, SessionSummary), error handling tables, integration test infrastructure |
| [`design-build-resources.md`](design-build-resources.md) | Build Resources agent module: implementation, design decisions, RunAsUser extension, Dockerfile layer order |
| [`design-agents.md`](design-agents.md) | Agent modules: contract, Claude Code implementation, adding future agents |
| [`design-vibekanban.md`](design-vibekanban.md) | Vibe Kanban agent module: auto-start mechanism, crash recovery, port discovery |
| [`design-properties.md`](design-properties.md) | Correctness properties (Properties 1–51) and full testing strategy |
| [`design-agent-summary-info.md`](design-agent-summary-info.md) | Agent Summary Info: KeyValue type, SummaryInfo interface method, generic collection, Vibe Kanban port discovery migration |

## Related Documents

- `requirements-core.md` — core application requirements (Req 1–22, including Req 22: Dynamic Container User Identity)
- `requirements-agents.md` — agent module requirements (CC-1–CC-8 for Claude Code, AC-1–AC-6 for Augment Code, BR-1–BR-6 for Build Resources, VK-1–VK-8 for Vibe Kanban)
- `requirements-cli-combinations.md` — valid and invalid CLI flag combinations (CLI-1–CLI-6)
- `requirements-two-layer-image.md` — two-layer Docker image requirements (TL-1–TL-11)
- `requirements-agent-summary-info.md` — Agent Summary Info requirements (SI-1–SI-7)
