# Docker Image Architecture

This document describes the two-layer Docker image architecture that splits the monolithic container image into a shared Base_Image and thin per-project Instance_Images for fast startup.

> **Related documents:**
> - [design.md](design.md) — Overview and document index
> - [design-architecture.md](design-architecture.md) — High-level architecture, package layout, sequence diagrams
> - [design-components.md](design-components.md) — Core component designs (DockerfileBuilder, etc.)
> - [design-data-models.md](design-data-models.md) — Data models, error handling, test infrastructure
> - [design-build-resources.md](design-build-resources.md) — Build Resources agent module design
> - [design-agents.md](design-agents.md) — Agent modules: contract, implementations
> - [design-properties.md](design-properties.md) — Correctness properties and testing strategy

---

## Two-Layer Image Architecture (TL-1 through TL-11)

> See `requirements-two-layer-image.md` for the full requirements.

### Motivation

The current monolithic image build takes minutes (agent npm installs, apt packages, Go tarball) and is repeated per-project even though 95% of the layers are identical. Splitting into a shared Base_Image and a thin per-project Instance_Image makes subsequent project startups near-instant (< 2 seconds for the Instance_Image build).

### Image Layer Split

The monolithic Dockerfile (previously shown in the DockerfileBuilder section) is split at the boundary between shared infrastructure and per-project SSH configuration:

- **Base_Image** (`bac-base:latest`): Everything from `FROM ubuntu:26.04` through the manifest write. Includes OS packages, Container_User, sudoers, keyring, gitconfig, all agent `Install()` steps, and the manifest. Does NOT include SSH host keys, authorized_keys, sshd hardening, or CMD.
- **Instance_Image** (`bac-<name>:latest`): `FROM bac-base:latest` + SSH host key injection + authorized_keys + sshd_config hardening + `/run/sshd` + CMD.

See the "Dockerfile Layer Order" section in [design-build-resources.md](design-build-resources.md) for the full layer listing, now annotated with which layers belong to which image.

### Builder Changes

The `DockerfileBuilder` is split into two construction paths:

```go
// NewBaseImageBuilder produces the Dockerfile for bac-base:latest.
// Contains everything EXCEPT SSH keys, authorized_keys, sshd hardening, and CMD.
func NewBaseImageBuilder(info *hostinfo.Info, strategy UserStrategy,
    conflictingUser string, gitConfig string) *DockerfileBuilder

// NewInstanceImageBuilder produces the Dockerfile for bac-<name>:latest.
// Starts with FROM bac-base:latest, adds only per-project SSH config + CMD.
// When hostNetworkOff is false (default), sshd_config includes Port and ListenAddress
// directives for host network mode. When true, sshd uses default port 22 (bridge mode).
func NewInstanceImageBuilder(info *hostinfo.Info,
    publicKey, hostKeyPriv, hostKeyPub string, sshPort int, hostNetworkOff bool) *DockerfileBuilder
```

The existing `NewDockerfileBuilder` is replaced by these two functions. Agent `Install()` methods are called on the base builder only. The instance builder has no agent steps — it's just SSH key injection + CMD.

### Build Flow in `runStart`

```mermaid
flowchart TD
    A[runStart] --> B{Base_Image exists?}
    B -->|No| C[Build Base_Image]
    B -->|Yes| D{Manifest matches?}
    D -->|No| E["Print 'run --rebuild'<br/>exit 0"]
    D -->|Yes| F{Instance_Image exists?}
    D -->|Label absent/invalid| C
    C --> G[Build Instance_Image]
    F -->|No| G
    F -->|Yes| H[Skip both builds]
    G --> I[Create & start container]
    H --> I

    R["--rebuild"] --> C2[Build Base_Image<br/>(no-cache)]
    C2 --> G2[Build Instance_Image]
    G2 --> I
```

### Cache Detection Logic

```go
func determineBuilds(ctx context.Context, c *Client, enabledIDs []string, containerName string, rebuild bool) (needBase, needInstance bool, err error) {
    if rebuild {
        return true, true, nil
    }

    // Check base image
    baseInfo, _, err := c.ImageInspectWithRaw(ctx, constants.BaseImageName+":latest")
    if err != nil {
        // Base doesn't exist — must build both
        return true, true, nil
    }

    manifestJSON, ok := baseInfo.Config.Labels["bac.manifest"]
    if !ok {
        return true, true, nil // no label — rebuild base
    }
    var manifestIDs []string
    if err := json.Unmarshal([]byte(manifestJSON), &manifestIDs); err != nil {
        return true, true, nil // invalid JSON — rebuild base
    }
    if !StringSlicesEqual(manifestIDs, enabledIDs) {
        // Manifest mismatch — caller prints message and exits
        return false, false, ErrManifestMismatch
    }

    // Base is good. Check instance image.
    instanceTag := containerName + ":latest"
    _, _, err = c.ImageInspectWithRaw(ctx, instanceTag)
    if err != nil {
        return false, true, nil // instance missing — build it only
    }

    return false, false, nil // both cached
}
```

### `--rebuild` Behavior

When `--rebuild` is set:
1. Stop and remove existing container (if any)
2. Build Base_Image with `NoCache: true`
3. Build Instance_Image (inherits fresh base)
4. Create and start new container

### `--host-network-off` and Instance_Image

The `--host-network-off` flag (Req 26) affects the Instance_Image content:

- **Default (host network mode):** `NewInstanceImageBuilder` appends `Port <SSH_Port>` and `ListenAddress 127.0.0.1` to sshd_config. The container is created with `NetworkMode: "host"` and no port bindings.
- **`--host-network-off` set (bridge mode):** `NewInstanceImageBuilder` omits the `Port` and `ListenAddress` directives — sshd uses its default port 22. The container is created with bridge networking and Docker port bindings (`127.0.0.1:<SSH_Port>` → container port 22).

Changing `--host-network-off` between invocations produces a different Instance_Image (different sshd_config). The CLI detects this mismatch and requires `--rebuild` to regenerate the Instance_Image.

### `--stop-and-remove` Behavior

No change to image handling. Only the container is stopped/removed. Both Base_Image and Instance_Image remain cached for fast restart.

### `--purge` Behavior

Removes all images (both `bac-base:latest` and all `bac-<name>:latest` instance images) via the existing `bac.managed` label filter.

### Constants Addition

```go
// constants.go
const BaseImageName = "bac-base"
```

### Startup Sequence (Updated)

The startup sequence diagram above is updated: the "Build image" step becomes two steps:
1. "Build Base_Image" (only if needed)
2. "Build Instance_Image" (only if needed)

The manifest comparison now checks the Base_Image label rather than the per-project image label.

**Validates: TL-1 through TL-11**
