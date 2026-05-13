# Part 1 — Core Application Design

## Architecture

### High-Level Component Diagram

```mermaid
graph TD
    User["Developer (CLI user)"]
    CLI["internal/cmd — Cobra CLI\n(core)"]
    Naming["internal/naming\n(core)"]
    Docker["internal/docker\n(core)"]
    SSH["internal/ssh\n(core)"]
    DataDir["internal/datadir\n(core)\n(includes credentials + port finding)"]
    AgentPkg["internal/agent — interface & registry\n(core)"]
    ClaudeAgent["internal/agents/claude\n(agent module)"]
    AugmentAgent["internal/agents/augment\n(agent module)"]
    BuildResAgent["internal/agents/buildresources\n(pseudo-agent module)"]
    FutureAgent["internal/agents/other\n(future agent module)"]
    DockerDaemon["Docker Daemon"]
    Container["Container\n(sshd + container user + enabled agents)"]

    User -->|"bac <path> [--agents ...]"| CLI
    CLI --> Naming
    CLI --> Docker
    CLI --> SSH
    CLI --> DataDir
    CLI --> AgentPkg
    ClaudeAgent -->|"Register() via init()"| AgentPkg
    AugmentAgent -->|"Register() via init()"| AgentPkg
    BuildResAgent -->|"Register() via init()"| AgentPkg
    FutureAgent -->|"Register() via init()"| AgentPkg
    Docker -->|"Docker SDK"| DockerDaemon
    DockerDaemon --> Container
```

The core packages (`internal/cmd`, `internal/naming`, `internal/docker`, `internal/ssh`, `internal/datadir`, `internal/agent`) have **no import dependency** on any package under `internal/agents/`. Agent modules are wired in exclusively via `main.go` blank imports.

> **Note (Module Consolidation):** The former `internal/credentials` and `internal/portfinder` packages have been merged into `internal/datadir`. Both dealt with per-project persistent state (credential paths, port selection/persistence) and had only `cmd/root.go` as their consumer. Consolidating them reduces package count without introducing import cycles or mixing unrelated concerns.

### Package Layout

```
bootstrap-ai-coding/
├── main.go                  # Blank-imports agent modules; wires everything together
│
└── internal/
    │   ── CORE ─────────────────────────────────────────────────────────────
    ├── constants/
    │   └── constants.go         # All glossary-derived constants — single source of truth
    ├── hostinfo/
    │   └── hostinfo.go          # Info struct + Current() — runtime host user identity (Req 22)
    ├── cmd/
    │   └── root.go              # Cobra root command, flag definitions, orchestration
    ├── naming/
    │   └── naming.go            # Deterministic container name from project path
    ├── docker/
    │   ├── client.go            # Docker SDK client wrapper; prerequisite checks (daemon reachable, version >= constants.MinDockerVersion)
    │   ├── builder.go           # DockerfileBuilder — dynamic Dockerfile assembly
    │   └── runner.go            # Container create/start/stop/inspect helpers
    ├── ssh/
    │   ├── keys.go              # Public key discovery
    │   ├── known_hosts.go       # ~/.ssh/known_hosts sync (SyncKnownHosts, RemoveKnownHostsEntries)
    │   └── ssh_config.go        # ~/.ssh/config sync (SyncSSHConfig, RemoveSSHConfigEntry, RemoveAllBACSSHConfigEntries)
    ├── datadir/
    │   ├── datadir.go           # Tool_Data_Dir management: create, read/write port, keys, manifest, purge
    │   ├── credentials.go       # Credential store path resolution and dir creation (merged from credentials/)
    │   └── portfinder.go        # SSH port auto-selection starting at constants.SSHPortStart (merged from portfinder/)
    ├── agent/
    │   ├── agent.go             # Agent interface definition  ← stable API boundary
    │   ├── preparer.go          # CredentialPreparer optional interface
    │   └── registry.go          # AgentRegistry — Register/Lookup/All
    │
    │   ── AGENT MODULES ────────────────────────────────────────────────────
    └── agents/
        ├── claude/
        │   └── claude.go        # Claude Code — reference Agent implementation
        ├── augment/
        │   └── augment.go       # Augment Code agent module
        └── buildresources/
            └── buildresources.go # Build Resources — pseudo-agent for dev toolchains
        # future agents added here, no core files change
```

### Startup Sequence

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant DataDir
    participant AgentRegistry
    participant Docker
    participant Container

    User->>CLI: bac /path/to/project [--agents claude-code]
    CLI->>CLI: Check effective UID — if 0, print error and exit 1 (Req 11)
    CLI->>CLI: Resolve host user identity via hostinfo.Current() (Req 22)
    note over CLI: *hostinfo.Info (Username, HomeDir, UID, GID) now available for all subsequent operations
    CLI->>CLI: Validate project path exists
    CLI->>Docker: Ping daemon, check version >= 20.10
    CLI->>AgentRegistry: Resolve enabled agents from --agents flag (default: "claude-code,augment-code,build-resources")
    note over AgentRegistry: Unknown agent ID → error, exit 1
    CLI->>SSH: Discover public key (~/.ssh/id_ed25519.pub → id_rsa.pub → --ssh-key)
    CLI->>DataDir: Init Tool_Data_Dir (~/.config/bootstrap-ai-coding/<name>/)
    CLI->>DataDir: Load persisted port or find free port starting at constants.SSHPortStart
    CLI->>DataDir: Persist chosen SSH port
    CLI->>DataDir: Load or generate SSH host key pair (stored in Tool_Data_Dir)
    loop For each enabled agent
        CLI->>AgentRegistry: agent.HasCredentials(resolvedStorePath)
        CLI->>CLI: Record agents needing auth
        CLI->>CLI: EnsureDir(resolvedStorePath)
    end
    CLI->>Docker: Inspect existing image — compare manifest vs enabled agents
    alt Manifest mismatch (and no --rebuild)
        CLI->>User: "Agent config changed — run with --rebuild" (stdout), exit 0
    else No image or --rebuild
        CLI->>Docker: Inspect Base_Container_Image for UID/GID conflict (Req 10a)
        alt Conflicting_Image_User found (existing user has Host_User UID or GID)
            CLI->>User: "User '<name>' (UID/GID) already exists in base image. Rename to '<host-username>'? [y/N]"
            alt User confirms rename
                CLI->>CLI: Set user_strategy = rename (use usermod -l in Dockerfile)
            else User declines
                CLI->>User: Error — cannot build without resolving UID/GID conflict (stderr), exit 1
            end
        else No conflict
            CLI->>CLI: Set user_strategy = create (use useradd in Dockerfile)
        end
        CLI->>Docker: Build image (DockerfileBuilder: ubuntu:26.04 + user_strategy + sshd + host key + agents + manifest) [verbose=Config.Verbose]
    end
    CLI->>Docker: Inspect container by name
    alt Container already running
        CLI->>SSH: SyncKnownHosts(port, hostPubKey, noUpdateKnownHosts)
        CLI->>SSH: SyncSSHConfig(containerName, port, noUpdateSSHConfig)
        CLI->>User: Print session summary (stdout), exit 0
    else Container not running
        CLI->>Docker: Create container (bind mounts: /workspace + per-agent creds)
        CLI->>Docker: Start container
        CLI->>Docker: TCP health-check: wait for sshd on SSH_Port (10s timeout)
        alt sshd timeout
            CLI->>Docker: Stop + remove container
            CLI->>User: Error message (stderr), exit 1
        else sshd ready
            loop For each agent needing auth
                CLI->>User: "Authenticate <agent-id> inside the container" (stdout)
            end
            CLI->>SSH: SyncKnownHosts(port, hostPubKey, noUpdateKnownHosts)
            CLI->>SSH: SyncSSHConfig(containerName, port, noUpdateSSHConfig)
            CLI->>User: Print session summary (stdout), exit 0
        end
    end
```

### Stop Sequence

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Docker

    User->>CLI: bac --stop-and-remove /path/to/project
    CLI->>CLI: Derive container name from path
    CLI->>Docker: Inspect container by name
    alt Container exists (running or stopped)
        CLI->>Docker: Stop container (if running)
        CLI->>Docker: Remove container
        CLI->>SSH: RemoveKnownHostsEntries(port)
        CLI->>SSH: RemoveSSHConfigEntry(containerName)
        CLI->>User: "Container stopped and removed"
    else Container not found
        CLI->>User: "No container found for this project"
    end
```

### Purge Sequence

```mermaid
sequenceDiagram
    participant User
    participant CLI
    participant Docker
    participant DataDir

    User->>CLI: bac --purge
    CLI->>CLI: Collect all bac-managed containers and images
    CLI->>CLI: Collect Tool_Data_Dir root path
    CLI->>User: Print summary of what will be deleted
    CLI->>User: Prompt for confirmation
    alt User confirms
        CLI->>Docker: Stop and remove all bac containers
        CLI->>Docker: Remove all bac images
        CLI->>DataDir: Delete ~/.config/bootstrap-ai-coding/ recursively
        CLI->>SSH: Remove all known_hosts entries for all bac SSH ports
        CLI->>SSH: Remove all SSH config entries for all bac- containers
        CLI->>User: Print confirmation of what was removed
    else User declines
        CLI->>User: Exit 0, nothing deleted
    end
```

---

## Related Documents

The detailed designs that were previously in this file have been split into focused documents:

| File | Contents |
|---|---|
| [design-components.md](design-components.md) | Core component designs: Constants, HostInfo, Agent Interface, AgentRegistry, DockerfileBuilder, Headless Keyring, Git Config Forwarding, Restart Policy, Base Image Inspection, Verbose Mode, Naming, SSH Key Discovery, SSH known_hosts, SSH Config, Credentials, DataDir, PortFinder |
| [design-docker.md](design-docker.md) | Two-layer Docker image architecture (TL-1 through TL-11): motivation, layer split, builder changes, build flow, cache detection, rebuild/stop/purge behaviour |
| [design-data-models.md](design-data-models.md) | Core data models (Mode, Config, ContainerSpec, Mount, SessionSummary), error handling tables, integration test infrastructure |
| [design-build-resources.md](design-build-resources.md) | Build Resources agent module: implementation, design decisions, RunAsUser extension, Dockerfile layer order |
| [design-agents.md](design-agents.md) | Agent modules: contract, Claude Code implementation, adding future agents |
| [design-properties.md](design-properties.md) | Correctness properties (Properties 1–51) and full testing strategy |
