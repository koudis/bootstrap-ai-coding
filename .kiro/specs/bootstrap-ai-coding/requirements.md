# Requirements

The requirements for this project are split across multiple documents:

- **[requirements-core.md](./requirements-core.md)** — Core application: CLI, Docker lifecycle, SSH, volume mounts, host network mode, restart policy, and the Agent module API (Agent_Interface + Agent_Registry). Requirements 1–26.
- **[requirements-agents.md](./requirements-agents.md)** — Agent module implementations: Claude Code (CC-1–CC-8), Augment Code (AC-1–AC-6), Build Resources (BR-1–BR-6), and Vibe Kanban (VK-1–VK-8).
- **[requirements-cli-combinations.md](./requirements-cli-combinations.md)** — Formal rules for valid and invalid CLI flag combinations. Requirements CLI-1–CLI-7.
- **[requirements-two-layer-image.md](./requirements-two-layer-image.md)** — Two-layer Docker image architecture (Base_Image + Instance_Image). Requirements TL-1–TL-11.
