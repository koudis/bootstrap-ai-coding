# Requirements

The requirements for this project are split across these documents:

- **[requirements-core.md](./requirements-core.md)** — Core application: CLI, Docker lifecycle, SSH, volume mounts, and the Agent module API (Agent_Interface + Agent_Registry). Requirements 1–26.
- **[requirements-agents.md](./requirements-agents.md)** — Agent module implementations: Claude Code, Augment Code, Build Resources, and Vibe Kanban. Requirements CC-1–CC-8, AC-1–AC-6, BR-1–BR-6, VK-1–VK-9.
- **[requirements-cli-combinations.md](./requirements-cli-combinations.md)** — Formal rules for valid and invalid CLI flag combinations. Requirements CLI-1–CLI-7.
- **[requirements-two-layer-image.md](./requirements-two-layer-image.md)** — Two-layer Docker image architecture. Requirements TL-1–TL-11.
- **[requirements-agent-summary-info.md](./requirements-agent-summary-info.md)** — Agent Summary Info: generic key:value pairs in session summary via Agent interface extension. Requirements SI-1–SI-7.
