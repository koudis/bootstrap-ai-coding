# Agent Module Requirements

## Introduction

This document defines the requirements for AI coding agent modules that plug into the `bootstrap-ai-coding` core. Each agent module is a self-contained implementation of the Agent_Interface defined by the core. The core does not need to be modified to add a new agent — only a new module conforming to this specification is required.

> **Related documents:**
> - `requirements-core.md` — core application requirements including the Agent_Interface contract
> - `requirements-cli-combinations.md` — formal rules for valid and invalid CLI flag combinations

## Glossary

> Terms defined in `requirements-core.md` (Agent_Interface, Base_Container_Image, Container_User, Container_User_Home, Credential_Store, Credential_Volume) are not repeated here. See the core glossary for their definitions.
>
> **Note:** `Container_User` and `Container_User_Home` match the Host_User's username and home directory path (see core Requirement 22). Agent modules that reference these values (e.g. for credential mount paths) receive them via the Agent_Interface contract.

- **Agent_ID**: The unique, stable string identifier for an agent module (e.g. `"claude-code"`). This is the value users supply to the `--agents` flag.
- **Health_Check**: A verification step run after container start to confirm the agent is installed and ready to use.

---

## General Agent Contract

Every agent module must conform to the contract defined in the core requirements:

- **[Requirement 7: Agent Module API](./requirements-core.md)** — defines the `Agent_Interface` (ID, Install, CredentialStorePath, ContainerMountPath, HasCredentials, HealthCheck, SummaryInfo), self-registration via `Agent_Registry`, and `--agents` flag behaviour.
- **[Requirement 8: Agent Installation & Credential Mount](./requirements-core.md)** — defines how the core calls agent installation steps, mounts credential stores, auto-creates missing directories, and invokes the optional `CredentialPreparer` interface.
- **[Agent Summary Info](./requirements-agent-summary-info.md)** (SI-1–SI-7) — defines the `SummaryInfo` method contract for contributing key:value pairs to the session summary.

Individual agent files below specify how each module satisfies this contract.

---

## Agent Requirements (per module)

| Agent | Requirements | Status |
|---|---|---|
| [Claude Code](./agents/requirements-claude-code.md) | CC-1 through CC-8 | Default agent |
| [Augment Code](./agents/requirements-augment-code.md) | AC-1 through AC-6 | Default agent |
| [Build Resources](./agents/requirements-build-resources.md) | BR-1 through BR-6 | Default pseudo-agent |
| [Codex](./agents/requirements-codex.md) | CX-1 through CX-6 | Opt-in only |

---

## Cross-Cutting Concerns

### Agent Summary Info

See **[requirements-agent-summary-info.md](./requirements-agent-summary-info.md)** — Agent Summary Info mechanism: generic key:value pairs in session summary via Agent interface extension. Requirements SI-1–SI-7.
