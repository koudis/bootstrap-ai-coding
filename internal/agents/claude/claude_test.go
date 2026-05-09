// Package claude_test contains property-based tests for the Claude Code agent module.
// The blank import of the claude package triggers its init() function, which
// registers the claudeAgent with the global agent registry.
package claude_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"
)

// newTestBuilder returns a DockerfileBuilder pre-seeded with the base layer,
// using fixed key material and UserStrategyCreate with uid=1000, gid=1000.
func newTestBuilder() *docker.DockerfileBuilder {
	return docker.NewBaseImageBuilder(
		&hostinfo.Info{Username: "testuser", HomeDir: "/home/testuser", UID: 1000, GID: 1000},
		docker.UserStrategyCreate, "",
		"",
	)
}

// ---------------------------------------------------------------------------
// Property 27: All registered agents satisfy the Agent interface
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 27: All registered agents satisfy the Agent interface
func TestPropertyAllAgentsSatisfyInterface(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		agents := agent.All()

		// At least one agent must be registered (the claude agent from the blank import).
		require.NotEmpty(rt, agents, "agent.All() must return at least one agent")

		for _, a := range agents {
			// Verify all six interface methods are callable without panicking.
			id := a.ID()
			require.NotEmpty(rt, id, "agent ID must not be empty")

			b := newTestBuilder()
			a.Install(b)

			credPath := a.CredentialStorePath()
			require.NotEmpty(rt, credPath, "CredentialStorePath must not be empty")

			mountPath := a.ContainerMountPath("/home/testuser")
			require.NotEmpty(rt, mountPath, "ContainerMountPath must not be empty")

			// HasCredentials with a temp dir must not panic and must return a
			// consistent result (no error for a valid directory path).
			tmpDir := t.TempDir()
			_, err := a.HasCredentials(tmpDir)
			require.NoError(rt, err, "HasCredentials on a valid temp dir must not error")

			// HealthCheck is not called here because it requires a live Docker
			// daemon and a running container — that is an integration concern.
		}
	})
}

// ---------------------------------------------------------------------------
// Property 28: Claude Code agent ID is stable
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 28: Claude Code agent ID is stable
func TestPropertyClaudeAgentIDIsStable(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.ClaudeCodeAgentName)
		require.NoError(rt, err, "claude agent must be registered under constants.ClaudeCodeAgentName")

		id := a.ID()
		require.Equal(rt, constants.ClaudeCodeAgentName, id,
			"ID() must always return constants.ClaudeCodeAgentName (%q)", constants.ClaudeCodeAgentName)
	})
}

// ---------------------------------------------------------------------------
// Property 29: Claude Code credential presence check is consistent
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 29: Claude Code credential presence check is consistent
func TestPropertyClaudeCredentialPresenceConsistent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.ClaudeCodeAgentName)
		require.NoError(rt, err, "claude agent must be registered")

		// Use a fresh temp dir for each iteration to avoid cross-iteration state.
		tmpDir := t.TempDir()
		credFile := filepath.Join(tmpDir, ".credentials.json")

		fileExists := rapid.Bool().Draw(rt, "fileExists")

		if fileExists {
			// Create the credentials file.
			err := os.WriteFile(credFile, []byte(`{"token":"test"}`), 0o600)
			require.NoError(rt, err, "failed to create test credentials file")
		}

		hasCreds, err := a.HasCredentials(tmpDir)
		require.NoError(rt, err, "HasCredentials must not error for a valid directory")

		require.Equal(rt, fileExists, hasCreds,
			"HasCredentials must return true iff .credentials.json exists in the store path")
	})
}

// ---------------------------------------------------------------------------
// Property 30: Claude Code container mount path is always constants.ContainerUserHome/.claude
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 30: Claude Code container mount path is always constants.ContainerUserHome/.claude
func TestPropertyClaudeContainerMountPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.ClaudeCodeAgentName)
		require.NoError(rt, err, "claude agent must be registered")

		mountPath := a.ContainerMountPath("/home/testuser")
		wantPath := "/home/testuser/.claude"

		require.Equal(rt, wantPath, mountPath,
			"ContainerMountPath() must always return %q", wantPath)
	})
}

// ---------------------------------------------------------------------------
// Property 31: Claude Code Dockerfile steps include Node.js and claude-code package
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 31: Claude Code Dockerfile steps include Node.js and claude-code package
func TestPropertyClaudeInstallIncludesNodeAndPackage(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.ClaudeCodeAgentName)
		require.NoError(rt, err, "claude agent must be registered")

		b := newTestBuilder()
		a.Install(b)
		content := b.Build()

		require.Contains(rt, content, "setup_22.x",
			"Dockerfile must include Node.js 22 setup step")
		require.Contains(rt, content, "@anthropic-ai/claude-code",
			"Dockerfile must include @anthropic-ai/claude-code installation step")
	})
}

// ---------------------------------------------------------------------------
// Unit tests — CC-1, CC-2, CC-3, CC-4, CC-6
// ---------------------------------------------------------------------------

// TestClaudeAgentRegistered verifies that the blank import causes the claude
// agent to self-register and that agent.Lookup succeeds for constants.ClaudeCodeAgentName.
// Validates: CC-1
func TestClaudeAgentRegistered(t *testing.T) {
	a, err := agent.Lookup(constants.ClaudeCodeAgentName)
	require.NoError(t, err, "claude agent must be registered under constants.ClaudeCodeAgentName")
	require.NotNil(t, a)
	require.Equal(t, constants.ClaudeCodeAgentName, a.ID())
}

// TestClaudeInstallStepsPresent verifies that Install appends RUN steps that
// install Node.js 22 and the @anthropic-ai/claude-code npm package when Node.js
// is not already installed.
// Validates: CC-2
func TestClaudeInstallStepsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.ClaudeCodeAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "setup_22.x",
		"Dockerfile must contain a Node.js 22 setup step")
	require.Contains(t, content, "@anthropic-ai/claude-code",
		"Dockerfile must contain an @anthropic-ai/claude-code installation step")
}

// TestClaudeCredentialPaths verifies that CredentialStorePath ends with ".claude".
// Validates: CC-3
func TestClaudeCredentialPaths(t *testing.T) {
	a, err := agent.Lookup(constants.ClaudeCodeAgentName)
	require.NoError(t, err)

	storePath := a.CredentialStorePath()
	require.NotEmpty(t, storePath)
	require.Equal(t, ".claude", filepath.Base(storePath),
		"CredentialStorePath must end with .claude")
}

// TestClaudeContainerMountPath verifies that ContainerMountPath equals
// "<homeDir>/.claude".
// Validates: CC-4
func TestClaudeContainerMountPath(t *testing.T) {
	a, err := agent.Lookup(constants.ClaudeCodeAgentName)
	require.NoError(t, err)

	want := "/home/testuser/.claude"
	require.Equal(t, want, a.ContainerMountPath("/home/testuser"))
}

// TestClaudeHasCredentialsEmpty verifies that HasCredentials returns (false, nil)
// when the store directory exists but contains no .credentials.json file.
// Validates: CC-6
func TestClaudeHasCredentialsEmpty(t *testing.T) {
	a, err := agent.Lookup(constants.ClaudeCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.False(t, hasCreds)
}

// TestClaudeHasCredentialsPresent verifies that HasCredentials returns (true, nil)
// when .credentials.json exists inside the store directory.
// Validates: CC-6
func TestClaudeHasCredentialsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.ClaudeCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	credFile := filepath.Join(tmpDir, ".credentials.json")
	err = os.WriteFile(credFile, []byte(`{"token":"test"}`), 0o600)
	require.NoError(t, err)

	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.True(t, hasCreds)
}

// TestClaudeHasCredentialsStatError verifies that HasCredentials returns a
// non-nil error when os.Stat fails for a reason other than IsNotExist
// (e.g. the parent directory is not readable).
// Validates: CC-6
func TestClaudeHasCredentialsStatError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}

	a, err := agent.Lookup(constants.ClaudeCodeAgentName)
	require.NoError(t, err)

	// Create a directory and make it unreadable so os.Stat on a file inside
	// it returns a permission error (not IsNotExist).
	base := t.TempDir()
	locked := filepath.Join(base, "locked")
	require.NoError(t, os.Mkdir(locked, 0o000))
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	_, err = a.HasCredentials(locked)
	require.Error(t, err, "HasCredentials must return an error when os.Stat fails")
}

// ---------------------------------------------------------------------------
// Node.js deduplication tests
// ---------------------------------------------------------------------------

// TestClaudeInstallNodeNotInstalled verifies that when IsNodeInstalled()
// returns false, Claude appends Node.js install steps and calls MarkNodeInstalled().
func TestClaudeInstallNodeNotInstalled(t *testing.T) {
	a, err := agent.Lookup(constants.ClaudeCodeAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	require.False(t, b.IsNodeInstalled(), "fresh builder must have IsNodeInstalled() == false")

	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "setup_22.x",
		"must install Node.js 22 when not already installed")
	require.Contains(t, content, "nodejs",
		"must install nodejs package when not already installed")
	require.Contains(t, content, "@anthropic-ai/claude-code",
		"must always install the claude-code npm package")
	require.True(t, b.IsNodeInstalled(),
		"MarkNodeInstalled() must be called after Node.js installation")
}

// TestClaudeInstallNodeAlreadyInstalled verifies that when IsNodeInstalled()
// returns true, Claude skips Node.js install steps but still installs its npm package.
func TestClaudeInstallNodeAlreadyInstalled(t *testing.T) {
	a, err := agent.Lookup(constants.ClaudeCodeAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	b.MarkNodeInstalled() // simulate a prior agent having installed Node.js
	require.True(t, b.IsNodeInstalled())

	linesBefore := len(b.Lines())
	a.Install(b)
	content := b.Build()

	// Must NOT contain the Node.js setup step
	require.NotContains(t, content, "setup_22.x",
		"must skip Node.js setup when already installed")

	// Must still install the npm package
	require.Contains(t, content, "@anthropic-ai/claude-code",
		"must always install the claude-code npm package")

	// Must still install curl/ca-certificates/git (idempotent prereqs)
	require.Contains(t, content, "curl ca-certificates git",
		"must always install curl, ca-certificates, git")

	// Should have added exactly 3 lines (apt-get prereqs + npm install + symlink)
	// plus optionally 1 more if ~/.claude/CLAUDE.md exists on the host (memory injection)
	linesAfter := len(b.Lines())
	added := linesAfter - linesBefore
	require.True(t, added == 3 || added == 4,
		"must add 3 RUN steps (prereqs + npm + symlink) plus optionally 1 memory injection step, got %d", added)
}

// ---------------------------------------------------------------------------
// Property 57: Agent ContainerMountPath uses runtime-provided home directory
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 57: Agent ContainerMountPath uses runtime-provided home directory
func TestPropertyAgentContainerMountPathUsesRuntimeHomeDir(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Draw a random valid absolute home directory path (no trailing slash).
		homeDir := rapid.StringMatching(`/[a-z][a-z0-9]*(/[a-z][a-z0-9]*)*`).Draw(rt, "homeDir")

		agents := agent.All()
		require.NotEmpty(rt, agents, "agent.All() must return at least one agent")

		for _, a := range agents {
			mountPath := a.ContainerMountPath(homeDir)

			// The returned path must start with homeDir + "/"
			require.True(rt, len(mountPath) > len(homeDir) && mountPath[:len(homeDir)+1] == homeDir+"/",
				"agent %q: ContainerMountPath(%q) = %q must start with %q",
				a.ID(), homeDir, mountPath, homeDir+"/")

			// If homeDir is NOT "/home/dev", the returned path must NOT contain "/home/dev"
			if homeDir != "/home/dev" {
				require.NotContains(rt, mountPath, "/home/dev",
					"agent %q: ContainerMountPath(%q) = %q must not contain /home/dev",
					a.ID(), homeDir, mountPath)
			}
		}
	})
}
