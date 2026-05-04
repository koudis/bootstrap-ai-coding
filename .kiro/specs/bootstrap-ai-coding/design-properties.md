# Correctness Properties and Testing Strategy

*A property is a characteristic or behavior that should hold true across all valid executions of a system — essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

---

## Correctness Properties

### Core Properties

#### Property 1: Non-existent project paths always produce errors

*For any* string that does not correspond to an existing filesystem path, the CLI's path validation SHALL return a non-nil error with a non-empty message.

**Validates: Req 1.4**

---

#### Property 2: Project path always produces a constants.WorkspaceMountPath bind mount

*For any* valid absolute project path, the container spec SHALL contain exactly one bind mount with `ContainerPath == constants.WorkspaceMountPath` and `HostPath == projectPath`.

**Validates: Req 2.1, 2.2**

---

#### Property 3: Generated Dockerfile always uses constants.BaseContainerImage as base image

*For any* set of enabled agents (including empty), the Dockerfile produced by `DockerfileBuilder` SHALL have `FROM ` + `constants.BaseContainerImage` as its first instruction and SHALL NOT reference any other base image.

**Validates: Req 9.1, 9.2, 9.3**

---

#### Property 4: Generated Dockerfile always includes SSH server and Container_User

*For any* set of enabled agents (including empty), the Dockerfile produced by `DockerfileBuilder` SHALL contain a `RUN` instruction that installs `openssh-server`, a `RUN` instruction that creates the `constants.ContainerUser` user with the correct UID/GID, and a `CMD` that starts `sshd`.

**Validates: Req 3.1, 10.1**

---

#### Property 5: Container_User UID and GID always match the host user

*For any* host UID and GID values, the Dockerfile produced by `DockerfileBuilder` SHALL contain either `useradd` arguments (UserStrategyCreate) or `usermod -l` arguments (UserStrategyRename) that result in the Container_User having the host UID and GID.

**Validates: Req 10.2, 10.3, 10.5**

---

#### Property 5a: UserStrategyRename uses usermod -l, UserStrategyCreate uses useradd

*For any* `DockerfileBuilder` constructed with `UserStrategyCreate`, the Dockerfile SHALL contain `useradd` and SHALL NOT contain `usermod -l`. *For any* builder constructed with `UserStrategyRename`, the Dockerfile SHALL contain `usermod -l` and SHALL NOT contain `useradd`.

**Validates: Req 10a.4**

---

#### Property 5b: No conflict returns nil from FindConflictingUser

*For any* UID/GID pair not present in the base image's `/etc/passwd`, `FindConflictingUser` SHALL return `(nil, nil)`.

**Validates: Req 10a.2**

---

#### Property 3b: Env, Copy, and Cmd instructions appear verbatim in the Dockerfile

*For any* key/value pair, source/destination pair, or command string, calling `DockerfileBuilder.Env(k, v)`, `Copy(src, dst)`, or `Cmd(cmd)` SHALL produce a Dockerfile containing exactly `ENV k=v`, `COPY src dst`, or `CMD ["/bin/sh", "-c", cmd]` respectively.

**Validates: Req 9.3**

---

*For any* Dockerfile produced by `DockerfileBuilder`, the content SHALL include a `sudoers` entry granting `constants.ContainerUser` passwordless `sudo` access.

**Validates: Req 10.4**

---

#### Property 7: sshd_config always disables password authentication

*For any* Dockerfile produced by `DockerfileBuilder`, the content SHALL include `PasswordAuthentication no`.

**Validates: Req 4.5**

---

#### Property 8: Public key is always injected into constants.ContainerUserHome/.ssh/authorized_keys

*For any* non-empty public key string, the Dockerfile SHALL contain a `RUN` instruction that appends that exact key to `constants.ContainerUserHome + "/.ssh/authorized_keys"`.

**Validates: Req 4.2**

---

#### Property 9: Public key discovery respects precedence order

*For any* combination of key files present (`id_ed25519.pub`, `id_rsa.pub`, custom `--ssh-key`), `DiscoverPublicKey` SHALL return the highest-precedence available key: `--ssh-key` > `id_ed25519.pub` > `id_rsa.pub`.

**Validates: Req 4.1**

---

#### Property 10: SSH host key is always injected into the Dockerfile

*For any* SSH host key pair content, the Dockerfile produced by `DockerfileBuilder` SHALL contain a `RUN` instruction that writes the private key to `/etc/ssh/ssh_host_ed25519_key` and the public key to `/etc/ssh/ssh_host_ed25519_key.pub`.

**Validates: Req 13.2**

---

#### Property 11: SSH host key is stable across rebuilds

*For any* project, once an SSH host key pair has been generated and stored in the Tool_Data_Dir, reading it back SHALL return the same key pair on every subsequent read.

**Validates: Req 13.3**

---

#### Property 12: Container naming produces correct, collision-resistant names

*For any* absolute project path, `ContainerName` SHALL always return the same name when called with the same `existingNames` set. The returned name SHALL start with `constants.ContainerNamePrefix`. The dirname and parentdir components in the name SHALL be sanitized (lowercase, only `[a-z0-9.-]` and the reserved `_` separator). *For any* two distinct project paths that would produce the same level-1 candidate, `ContainerName` SHALL return distinct names by advancing to level 2 or the counter suffix.

**Validates: Req 5.1**

---

#### Property 13: Docker version comparison is correct

*For any* version string, the version checker SHALL accept it if and only if the parsed version is `>= 20.10.0`.

**Validates: Req 6.3**

---

#### Property 14: Credential store path resolution respects override precedence

*For any* agent default path and override string, `credentials.Resolve` SHALL return the override when non-empty, and the expanded agent default when the override is empty.

**Validates: Req 8.3**

---

#### Property 15: Credential store directory is always created before mounting

*For any* non-existent credential store path, `credentials.EnsureDir` SHALL create the directory (and all parents) before the container starts.

**Validates: Req 8.4**

---

#### Property 16: --agents flag parsing produces correct agent ID slices

*For any* comma-separated string of agent IDs (including single IDs, multiple IDs, IDs with surrounding whitespace), the flag parser SHALL produce a slice of exactly the trimmed, non-empty IDs in original order.

**Validates: Req 7.4**

---

#### Property 17: Dockerfile contains install steps for exactly the enabled agents

*For any* non-empty set of enabled agent IDs, the Dockerfile SHALL contain each enabled agent's `Install()` steps and SHALL NOT contain steps from agents not in the enabled set.

**Validates: Req 8.1, 8.2**

---

#### Property 18: Every enabled agent's credential store is bind-mounted

*For any* non-empty set of enabled agents, the container spec's mount list SHALL contain one bind mount per agent with `HostPath == resolvedCredStorePath` and `ContainerPath == agent.ContainerMountPath()`.

**Validates: Req 8.3**

---

#### Property 19: Auth warning is printed for every agent with empty credentials

*For any* set of enabled agents where a subset has empty credential stores, the CLI output SHALL contain one warning per agent in that subset, each identifying the agent by `ID()`.

**Validates: Req 8.5**

---

#### Property 20: Port finder returns the first free port at or above constants.SSHPortStart

*For any* set of occupied ports starting at `constants.SSHPortStart`, `portfinder.FindFreePort` SHALL return the lowest port number >= `constants.SSHPortStart` that is not in the occupied set.

**Validates: Req 12.1**

---

#### Property 21: Persisted port round-trips correctly

*For any* valid port number, writing it to the Tool_Data_Dir and reading it back SHALL return the same port number.

**Validates: Req 12.2**

---

#### Property 22: Manifest round-trips correctly for any agent ID list

*For any* non-empty list of agent ID strings, writing it with `datadir.WriteManifest` and reading it back with `datadir.ReadManifest` SHALL return the same list in the same order.

**Validates: Req 14.2, 15.3**

---

#### Property 22b: SSH port is always bound to the selected host port

*For any* valid port number, the container spec SHALL contain a port binding mapping container port `constants.ContainerSSHPort/tcp` to that host port.

**Validates: Req 12.4**

---

#### Property 23: Manifest is written for exactly the enabled agents

*For any* set of enabled agent IDs, the Dockerfile SHALL contain a step that writes `constants.ManifestFilePath` listing exactly those agent IDs.

**Validates: Req 14.2**

---

#### Property 24: Tool_Data_Dir is created with constants.ToolDataDirPerm permissions

*For any* non-existent Tool_Data_Dir path, `datadir.New` SHALL create the directory with permissions `constants.ToolDataDirPerm`.

**Validates: Req 15.2**

---

#### Property 25: Session summary always contains all required fields

*For any* valid session configuration, the session summary printed to stdout SHALL contain labelled lines for: data directory, project directory, SSH port, SSH connect command, and enabled agents.

**Validates: Req 17.1, 17.2, 17.3**

---

#### Property 26: Unknown agent IDs always produce errors

*For any* string not matching a registered agent ID, `AgentRegistry.Lookup` SHALL return a non-nil error.

**Validates: Req 7.3**

---

#### Property 43: ExpandHome never returns a path starting with ~/

*For any* input string, `cmd.ExpandHome` SHALL return a path that does not start with `~/`. If the input starts with `~/`, the `~/` prefix SHALL be replaced with the absolute home directory path.

**Validates: Req 15.1**

---

#### Property 44: StringSlicesEqual is reflexive and symmetric

*For any* two string slices `s` and `t`, `StringSlicesEqual(s, s)` SHALL always return `true`, and `StringSlicesEqual(s, t)` SHALL equal `StringSlicesEqual(t, s)`.

**Validates: internal correctness invariant**

---

### CLI Flag Combination Properties

#### Property 32: S ∧ U is always rejected (CLI-1)

*For any* invocation where both `--stop-and-remove` and `--purge` are set, `ResolveMode` SHALL return a non-nil error.

**Validates: CLI-1**

---

#### Property 33: Mode is always exactly one of START, STOP, PURGE (CLI-1)

*For any* valid combination of `stopAndRemove` and `purge` booleans (excluding `true ∧ true`), `ResolveMode` SHALL return exactly one of `ModeStart`, `ModeStop`, or `ModePurge`.

**Validates: CLI-1**

---

#### Property 34: START-only flags in STOP or PURGE mode always produce errors (CLI-3)

*For any* invocation in STOP or PURGE mode where any of `--agents`, `--port`, `--ssh-key`, `--rebuild`, `--no-update-known-hosts`, or `--no-update-ssh-config` is set, the CLI SHALL return a non-nil error identifying the incompatible flag(s).

**Validates: CLI-3**

---

#### Property 35: --port is always within 1024–65535 when provided (CLI-5)

*For any* integer value of `--port`, the CLI SHALL accept it if and only if `1024 ≤ port ≤ 65535`.

**Validates: CLI-5**

---

#### Property 36: --agents always resolves to a non-empty list of known IDs (CLI-6)

*For any* comma-separated `--agents` string, the CLI SHALL reject it if the parsed list is empty or contains any ID not in the AgentRegistry.

**Validates: CLI-6**

---

### Agent Module Properties

#### Property 27: All registered agents satisfy the Agent interface

*For any* agent returned by `agent.All()`, the agent SHALL implement all six methods of the `Agent` interface: `ID()`, `Install()`, `CredentialStorePath()`, `ContainerMountPath()`, `HasCredentials()`, `HealthCheck()`.

**Validates: Req 7.1, Agent Req CC-1 through CC-5**

---

#### Property 28: Claude Code agent ID is stable

*For any* invocation, `claudeAgent.ID()` SHALL always return `constants.DefaultAgent` (`"claude-code"`).

**Validates: Agent Req CC-1**

---

#### Property 29: Claude Code credential presence check is consistent

*For any* directory path, `HasCredentials` SHALL return `true` if and only if `.credentials.json` exists in that directory.

**Validates: Agent Req CC-4**

---

#### Property 30: Claude Code container mount path is always constants.ContainerUserHome/.claude

*For any* invocation, `claudeAgent.ContainerMountPath()` SHALL always return `constants.ContainerUserHome + "/.claude"`.

**Validates: Agent Req CC-3**

---

#### Property 31: Claude Code Dockerfile steps include Node.js and claude-code package

*For any* `DockerfileBuilder`, after calling `claudeAgent.Install(b)`, the resulting Dockerfile SHALL contain `RUN` instructions that install Node.js and `@anthropic-ai/claude-code`.

**Validates: Agent Req CC-2**

---

#### Property 45: Augment Code agent ID is stable

*For any* invocation, `augmentAgent.ID()` SHALL always return `"augment-code"`.

**Validates: Agent Req AC-1**

---

#### Property 46: Augment Code credential presence check is consistent

*For any* directory path, `HasCredentials` SHALL return `true` if and only if the directory exists and contains at least one non-empty file. It SHALL return `false` when the directory does not exist or contains no non-empty files, and SHALL return `(false, nil)` — not an error — when the directory is absent.

**Validates: Agent Req AC-4**

---

#### Property 47: Augment Code container mount path is always constants.ContainerUserHome/.augment

*For any* invocation, `augmentAgent.ContainerMountPath()` SHALL always return `filepath.Join(constants.ContainerUserHome, ".augment")`.

**Validates: Agent Req AC-3**

---

#### Property 48: Augment Code Dockerfile steps include Node.js 22+ and auggie package

*For any* `DockerfileBuilder`, after calling `augmentAgent.Install(b)`, the resulting Dockerfile SHALL contain `RUN` instructions that install Node.js 22 (via `setup_22.x`) and `@augmentcode/auggie`.

**Validates: Agent Req AC-2**

---

#### Property 49: Augment Code agent is registered and satisfies the Agent interface

*For any* binary compiled with the `agents/augment` package blank-imported, `agent.Lookup("augment-code")` SHALL return a non-nil agent that implements all six methods of the `Agent` interface.

**Validates: Agent Req AC-1, AC-6**

---

### SSH Config Properties

#### Property 39: SyncSSHConfig never modifies unrelated SSH config entries

*For any* pre-existing `~/.ssh/config` with N stanzas whose `Host` values do not start with `constants.ContainerNamePrefix`, after `SyncSSHConfig` the file still contains all N original stanzas unchanged.

**Validates: Req 19.6**

---

#### Property 40: RemoveSSHConfigEntry only removes the entry for the given container name

*For any* `~/.ssh/config` with entries for multiple container names, `RemoveSSHConfigEntry(name)` removes only the stanza whose `Host` value equals `name` and leaves all other stanzas intact.

**Validates: Req 19.7**

---

#### Property 41: SyncSSHConfig is idempotent when entry matches

*For any* container name and port, calling `SyncSSHConfig` twice with the same arguments produces the same file as calling it once.

**Validates: Req 19.3**

---

#### Property 42: RemoveAllBACSSHConfigEntries only removes bac- prefixed entries

*For any* `~/.ssh/config` containing a mix of bac-prefixed and non-bac entries, `RemoveAllBACSSHConfigEntries` removes all and only the entries whose `Host` value starts with `constants.ContainerNamePrefix`.

**Validates: Req 19.8**

---

---

#### Property 50: Silent mode produces no Docker build output on stdout

*For any* build invocation where `verbose == false`, the `BuildImageWithTimeout` function SHALL NOT write any Docker build stream content to stdout. The only visible output during a silent build is the "Building image..." message printed by the caller before invoking `BuildImage`.

**Validates: Req 20.2**

---

#### Property 51: Verbose mode streams non-empty output for any non-trivial Dockerfile

*For any* Dockerfile containing at least one `RUN` instruction, a `BuildImageWithTimeout` call with `verbose == true` SHALL result in at least one non-empty `stream` line being written to stdout before the build completes successfully.

**Validates: Req 20.3**

---

## Testing Strategy

### Dual Testing Approach

Unit/property-based tests cover pure logic. Integration tests cover Docker interactions and are gated by a build tag (`//go:build integration`).

### Property-Based Testing

The chosen library is **[`pgregory.net/rapid`](https://github.com/flyingmutant/rapid)**. Each property test runs a minimum of **100 iterations**.

**Tag format:** `// Feature: bootstrap-ai-coding, Property N: <property text>`

#### Property Test Sketches

```go
// Feature: bootstrap-ai-coding, Property 12: Container naming produces correct, collision-resistant names
func TestContainerNameDeterminism(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        path := rapid.StringMatching(`/[a-z/]+`).Draw(t, "path")
        name1, _ := naming.ContainerName(path, nil)
        name2, _ := naming.ContainerName(path, nil)
        if name1 != name2 {
            t.Fatalf("non-deterministic: %q != %q for path %q", name1, name2, path)
        }
    })
}

// Feature: bootstrap-ai-coding, Property 12: Container naming produces correct, collision-resistant names
func TestContainerNameHasBacPrefix(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        path := rapid.StringMatching(`/[a-z/]+`).Draw(t, "path")
        name, err := naming.ContainerName(path, nil)
        require.NoError(t, err)
        require.True(t, strings.HasPrefix(name, constants.ContainerNamePrefix),
            "name %q does not start with %q", name, constants.ContainerNamePrefix)
    })
}

// Feature: bootstrap-ai-coding, Property 12: Container naming produces correct, collision-resistant names
func TestContainerNameConflictAdvancesToNextLevel(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        path := rapid.StringMatching(`/[a-z]+/[a-z]+`).Draw(t, "path")
        // Level-1 name
        level1, err := naming.ContainerName(path, nil)
        require.NoError(t, err)
        // With level-1 occupied, must return a different name
        level2, err := naming.ContainerName(path, []string{level1})
        require.NoError(t, err)
        require.NotEqual(t, level1, level2, "should advance past occupied level-1 name")
        require.True(t, strings.HasPrefix(level2, constants.ContainerNamePrefix))
    })
}

// Feature: bootstrap-ai-coding, Property 2: Project path always produces a constants.WorkspaceMountPath bind mount
func TestWorkspaceMountAlwaysPresent(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        path := rapid.StringMatching(`/[a-zA-Z0-9_/.-]+`).Draw(t, "path")
        spec := docker.BuildContainerSpec(path, nil, "")
        found := false
        for _, m := range spec.Mounts {
            if m.ContainerPath == constants.WorkspaceMountPath && m.HostPath == path {
                found = true
            }
        }
        if !found {
            t.Fatalf("no %s mount for path %q", constants.WorkspaceMountPath, path)
        }
    })
}

// Feature: bootstrap-ai-coding, Property 3: Generated Dockerfile always uses constants.BaseContainerImage as base image
func TestDockerfileBaseImage(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
        gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
        pubKey := rapid.StringMatching(`ssh-ed25519 [A-Za-z0-9+/]+ test@host`).Draw(t, "pubKey")
        b := docker.NewDockerfileBuilder(uid, gid, pubKey, "priv-key", "pub-key",
            docker.UserStrategyCreate, "")
        lines := b.Lines()
        want := "FROM " + constants.BaseContainerImage
        if lines[0] != want {
            t.Fatalf("first line is %q, want %q", lines[0], want)
        }
    })
}

// Feature: bootstrap-ai-coding, Property 5: Container_User UID and GID always match the host user
func TestDockerfileDevUserUID(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
        gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
        b := docker.NewDockerfileBuilder(uid, gid, "ssh-rsa AAAA test@host", "priv", "pub",
            docker.UserStrategyCreate, "")
        content := b.Build()
        if !strings.Contains(content, fmt.Sprintf("--uid %d", uid)) {
            t.Fatalf("Dockerfile missing --uid %d", uid)
        }
        if !strings.Contains(content, fmt.Sprintf("--gid %d", gid)) {
            t.Fatalf("Dockerfile missing --gid %d", gid)
        }
        if !strings.Contains(content, constants.ContainerUser) {
            t.Fatalf("Dockerfile missing container user %q", constants.ContainerUser)
        }
    })
}

// Feature: bootstrap-ai-coding, Property 13: Docker version comparison is correct
func TestDockerVersionComparison(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        major := rapid.IntRange(0, 30).Draw(t, "major")
        minor := rapid.IntRange(0, 20).Draw(t, "minor")
        patch := rapid.IntRange(0, 10).Draw(t, "patch")
        ver := fmt.Sprintf("%d.%d.%d", major, minor, patch)
        ok := docker.IsVersionCompatible(ver)
        expected := major > 20 || (major == 20 && minor >= 10)
        if ok != expected {
            t.Fatalf("version %q: got %v, want %v", ver, ok, expected)
        }
    })
}

// Feature: bootstrap-ai-coding, Property 16: --agents flag parsing produces correct agent ID slices
func TestAgentsFlagParsing(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        ids := rapid.SliceOfN(rapid.StringMatching(`[a-z][a-z0-9-]*`), 1, 5).Draw(t, "ids")
        input := strings.Join(ids, ",")
        parsed := cmd.ParseAgentsFlag(input)
        if !reflect.DeepEqual(parsed, ids) {
            t.Fatalf("parsed %v from %q, want %v", parsed, input, ids)
        }
    })
}

// Feature: bootstrap-ai-coding, Property 20: Port finder returns the first free port at or above constants.SSHPortStart
func TestPortFinderReturnsFirstFreePort(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        numOccupied := rapid.IntRange(0, 5).Draw(t, "numOccupied")
        listeners := make([]net.Listener, 0, numOccupied)
        for i := 0; i < numOccupied; i++ {
            ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", constants.SSHPortStart+i))
            if err != nil {
                break
            }
            listeners = append(listeners, ln)
        }
        defer func() {
            for _, ln := range listeners {
                ln.Close()
            }
        }()
        port, err := portfinder.FindFreePort()
        require.NoError(t, err)
        require.GreaterOrEqual(t, port, constants.SSHPortStart)
        require.True(t, portfinder.IsPortFree(port))
    })
}

// Feature: bootstrap-ai-coding, Property 21: Persisted port round-trips correctly
func TestPortRoundTrip(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        port := rapid.IntRange(1024, 65535).Draw(t, "port")
        dir := t.TempDir()
        dd := &datadir.DataDir{} // test helper constructor
        dd.WritePort(port)
        got, err := dd.ReadPort()
        require.NoError(t, err)
        require.Equal(t, port, got)
    })
}

// Feature: bootstrap-ai-coding, Property 25: Session summary always contains all required fields
func TestSessionSummaryContainsAllFields(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        port := rapid.IntRange(1024, 65535).Draw(t, "port")
        projectDir := rapid.StringMatching(`/[a-z/]+`).Draw(t, "projectDir")
        agentIDs := rapid.SliceOfN(rapid.StringMatching(`[a-z][a-z0-9-]*`), 1, 3).Draw(t, "agentIDs")
        summary := cmd.FormatSessionSummary(cmd.SessionSummary{
            DataDir:       "/home/user/.config/bootstrap-ai-coding/bac-abc123",
            ProjectDir:    projectDir,
            SSHPort:       port,
            SSHConnect:    fmt.Sprintf("ssh -p %d %s@localhost", port, constants.ContainerUser),
            EnabledAgents: agentIDs,
        })
        require.Contains(t, summary, "Data directory:")
        require.Contains(t, summary, "Project directory:")
        require.Contains(t, summary, "SSH port:")
        require.Contains(t, summary, "SSH connect:")
        require.Contains(t, summary, "Enabled agents:")
    })
}

// Feature: bootstrap-ai-coding, Property 29: Claude Code credential presence check is consistent
func TestClaudeHasCredentials(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        dir := t.TempDir()
        hasFile := rapid.Bool().Draw(t, "hasFile")
        if hasFile {
            os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(`{}`), constants.ToolDataFilePerm)
        }
        a, _ := agent.Lookup(constants.DefaultAgent)
        got, err := a.HasCredentials(dir)
        require.NoError(t, err)
        require.Equal(t, hasFile, got)
    })
}

// Feature: bootstrap-ai-coding, Property 39: SyncSSHConfig never modifies unrelated SSH config entries
func TestSyncSSHConfigPreservesUnrelatedEntries(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        // Write N unrelated Host stanzas (Host values not starting with "bac-")
        // Call SyncSSHConfig for a bac- container
        // Assert all N original stanzas are still present verbatim
    })
}

// Feature: bootstrap-ai-coding, Property 41: SyncSSHConfig is idempotent when entry matches
func TestSyncSSHConfigIdempotent(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        containerName := rapid.StringMatching(`bac-[a-z][a-z0-9-]*`).Draw(t, "name")
        port := rapid.IntRange(1024, 65535).Draw(t, "port")
        // Call SyncSSHConfig twice with same args; assert file content identical after both calls
    })
}
```

### Unit Tests (Example-Based)

| Test | Validates |
|---|---|
| `TestNoArgsShowsUsage` | Req 1.3 |
| `TestInvalidProjectPathError` | Req 1.4 |
| `TestNoPublicKeyError` | Req 4.4 |
| `TestDockerDaemonUnreachable` | Req 6.2 |
| `TestIncompatibleDockerVersion` | Req 6.4 |
| `TestExistingContainerReturnsSessionSummary` | Req 5.2 |
| `TestStopAndRemoveNonExistentContainer` | Req 5.4 |
| `TestUnknownAgentIDError` | Req 7.3 |
| `TestDefaultAgentsUsedWhenFlagOmitted` | Req 7.5 |
| `TestRootExecutionPrevented` | Req 11.1, 11.2, 11.3 |
| `TestSSHHostKeyGeneratedOnFirstBuild` | Req 13.1 |
| `TestSSHHostKeyFilePermissions` | Req 13.4 |
| `TestManifestMismatchInstructsRebuild` | Req 14.3 |
| `TestRebuildFlagForcesRebuild` | Req 14.4 |
| `TestPurgeConfirmationPrompt` | Req 16.5 |
| `TestPurgeDeclinedDoesNothing` | Req 16.5 |
| `TestStopAndPurgeTogetherRejected` | CLI-1 |
| `TestPurgeWithProjectPathRejected` | CLI-2 |
| `TestStopWithoutProjectPathRejected` | CLI-2 |
| `TestAgentsFlagWithStopRejected` | CLI-3 |
| `TestPortFlagWithPurgeRejected` | CLI-3 |
| `TestRebuildFlagWithStopRejected` | CLI-3 |
| `TestPortBelowRangeRejected` | CLI-5 |
| `TestPortAboveRangeRejected` | CLI-5 |
| `TestEmptyAgentsFlagRejected` | CLI-6 |
| `TestClaudeAgentRegistered` | Agent Req CC-1, CC-6 |
| `TestClaudeInstallStepsPresent` | Agent Req CC-2 |
| `TestClaudeCredentialPaths` | Agent Req CC-3 |
| `TestClaudeContainerMountPath` | Agent Req CC-3 |
| `TestClaudeHasCredentialsEmpty` | Agent Req CC-6 |
| `TestClaudeHasCredentialsPresent` | Agent Req CC-6 |
| `TestClaudeHasCredentialsStatError` | Agent Req CC-6 |
| `TestAugmentAgentRegistered` | Agent Req AC-1, AC-6 |
| `TestAugmentInstallStepsPresent` | Agent Req AC-2 |
| `TestAugmentCredentialPaths` | Agent Req AC-3 |
| `TestAugmentContainerMountPath` | Agent Req AC-3 |
| `TestAugmentHasCredentialsEmpty` | Agent Req AC-4 |
| `TestAugmentHasCredentialsPresent` | Agent Req AC-4 |
| `TestAll_ReturnsRegisteredAgents` | Req 7.1 |
| `TestAll_CountMatchesKnownIDs` | Req 7.1 |
| `TestBuilderEnvAppendsCorrectInstruction` | Req 9.3 |
| `TestBuilderCopyAppendsCorrectInstruction` | Req 9.3 |
| `TestBuilderCmdAppendsCorrectInstruction` | Req 9.3 |
| `TestGenerateHostKeyPairProducesValidKeys` | Req 13.1 |
| `TestGenerateHostKeyPairIsUnique` | Req 13.1 |
| `TestReadPortCorruptContent` | Req 12.2 |
| `TestReadHostKeyMissingPubKey` | Req 13.3 |
| `TestReadManifestCorruptJSON` | Req 14.2 |
| `TestReadManifestRoundTrip` | Req 14.2 |
| `TestPurgeRoot` | Req 16.4 |
| `TestListContainerNames` | Req 15.1 |
| `TestSyncKnownHostsNoUpdateSkipsFile` | Req 18.9 |
| `TestRemoveKnownHostsEntriesNoopWhenFileAbsent` | Req 18.7 |
| `TestRemoveKnownHostsEntriesNoopWhenPortNotPresent` | Req 18.7 |
| `TestSyncKnownHostsAppendsNewEntries` | Req 18.1 |
| `TestRemoveSSHConfigEntryNoopWhenFileAbsent` | Req 19.7 |
| `TestRemoveSSHConfigEntryNoopWhenStanzaAbsent` | Req 19.7 |
| `TestRemoveAllBACSSHConfigEntriesNoopWhenFileAbsent` | Req 19.8 |
| `TestRemoveAllBACSSHConfigEntriesNoopWhenNoBacEntries` | Req 19.8 |
| `TestStringSlicesEqualBothEmpty` | Property 44 |
| `TestStringSlicesEqualSameElements` | Property 44 |
| `TestStringSlicesEqualDifferentLength` | Property 44 |
| `TestStringSlicesEqualDifferentContent` | Property 44 |
| `TestStringSlicesEqualOrderMatters` | Property 44 |
| `TestExpandHomeNoTilde` | Property 43 |
| `TestExpandHomeTildeExpanded` | Property 43 |
| `TestExpandHomeTildeOnly` | Property 43 |
| `TestResolveExpandsHomeTilde` | Req 8.3 |
| `TestResolveNoTildePassthrough` | Req 8.3 |
| `TestEnsureDirIdempotent` | Req 8.4 |
| `TestSSHConfigEntryAddedOnStart` | Req 19.1, 19.2 |
| `TestSSHConfigNoChangeWhenEntryMatches` | Req 19.3 |
| `TestSSHConfigStaleEntryReplaced` | Req 19.4 |
| `TestSSHConfigEntryRemovedOnStopAndRemove` | Req 19.7 |
| `TestSSHConfigSkippedWithNoUpdateFlag` | Req 19.9 |
| `TestNoUpdateSSHConfigFlagWithStopRejected` | CLI-3 |
| `TestNoUpdateSSHConfigFlagWithPurgeRejected` | CLI-3 |
| `TestVerboseFlagWithStopRejected` | CLI-3, Req 20.5 |
| `TestVerboseFlagWithPurgeRejected` | CLI-3, Req 20.5 |
| `TestVerboseSilentModeNoStdout` | Req 20.2 |
| `TestVerboseModeStreamsOutput` | Req 20.3 |

### Integration Tests

Gated by `//go:build integration`. Require a running Docker daemon.

#### Environment precondition: base image must NOT be present

The `internal/docker` integration suite enforces via `TestMain` that `constants.BaseContainerImage` is **not** present in the local Docker image store when the suite starts. `TestFindConflictingUserPullsImageIfAbsent` specifically tests the auto-pull path — if the image is already cached, that test would never exercise the pull logic and its result would be a false positive.

`TestMain` fails the entire suite immediately if the image is present:

```
INTEGRATION TEST ENVIRONMENT ERROR
The base image "ubuntu:26.04" is already present in the local Docker image store.
Fix: docker rmi ubuntu:26.04
```

**Before running integration tests:**

```bash
docker rmi ubuntu:26.04
go test -tags integration -timeout 30m ./...
```

| Test | Validates |
|---|---|
| `TestContainerStartsAndSSHConnects` | Req 3.3, 4.3 |
| `TestWorkspaceMountLiveSync` | Req 2.3 |
| `TestFileOwnershipMatchesHostUser` | Req 10.6 |
| `TestCredentialVolumePersistedAcrossRestart` | Req 8.6 |
| `TestSSHPortPersistenceAcrossRestarts` | Req 12.2 |
| `TestSSHHostKeyStableAcrossRebuild` | Req 13.3 |
| `TestPurgeRemovesContainersAndImages` | Req 16.2, 16.4 |
| `TestKnownHostsEntriesLifecycle` | Req 18.1–18.2, 18.7 |
| `TestSSHConfigEntryLifecycle` | Req 19.1–19.2, 19.7 |
| `TestBuildImageTimeoutEnforced` | Req 14.7 |
| `TestFindConflictingUserPullsImageIfAbsent` | Req 10a.1 |
| `TestClaudeAvailableInContainer` | Agent Req CC-2.3 |
| `TestClaudeHealthCheck` | Agent Req CC-5 |
| `TestAugmentAvailableInContainer` | Agent Req AC-2.3 |
| `TestAugmentHealthCheck` | Agent Req AC-5 |

### Test Coverage Targets

- Unit + property tests: ≥ 80% line coverage on `naming`, `ssh`, `credentials`, `datadir`, `portfinder`, `agent`, `docker/builder.go`, `agents/claude`, `agents/augment`
- Integration tests: full happy path + SSH health-check failure path + rebuild path
