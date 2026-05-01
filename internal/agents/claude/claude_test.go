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
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// fixedHostKeyPriv and fixedHostKeyPub are stable test values used wherever
// the exact key content is not the subject of the property under test.
const (
	fixedHostKeyPriv = "-----BEGIN OPENSSH PRIVATE KEY-----\nfakePrivKey\n-----END OPENSSH PRIVATE KEY-----"
	fixedHostKeyPub  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIfakeHostPub host-key"
	fixedPublicKey   = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIfakePubKey test@host"
)

// newTestBuilder returns a DockerfileBuilder pre-seeded with the base layer,
// using fixed key material and UserStrategyCreate with uid=1000, gid=1000.
func newTestBuilder() *docker.DockerfileBuilder {
	return docker.NewDockerfileBuilder(
		1000, 1000,
		fixedPublicKey,
		fixedHostKeyPriv, fixedHostKeyPub,
		docker.UserStrategyCreate, "",
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

			mountPath := a.ContainerMountPath()
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
		a, err := agent.Lookup(constants.DefaultAgent)
		require.NoError(rt, err, "claude agent must be registered under constants.DefaultAgent")

		id := a.ID()
		require.Equal(rt, constants.DefaultAgent, id,
			"ID() must always return constants.DefaultAgent (%q)", constants.DefaultAgent)
	})
}

// ---------------------------------------------------------------------------
// Property 29: Claude Code credential presence check is consistent
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 29: Claude Code credential presence check is consistent
func TestPropertyClaudeCredentialPresenceConsistent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.DefaultAgent)
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
		a, err := agent.Lookup(constants.DefaultAgent)
		require.NoError(rt, err, "claude agent must be registered")

		mountPath := a.ContainerMountPath()
		wantPath := constants.ContainerUserHome + "/.claude"

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
		a, err := agent.Lookup(constants.DefaultAgent)
		require.NoError(rt, err, "claude agent must be registered")

		b := newTestBuilder()
		a.Install(b)
		content := b.Build()

		require.Contains(rt, content, "nodejs",
			"Dockerfile must include nodejs installation step")
		require.Contains(rt, content, "@anthropic-ai/claude-code",
			"Dockerfile must include @anthropic-ai/claude-code installation step")
	})
}

// ---------------------------------------------------------------------------
// Unit tests — CC-1, CC-2, CC-3, CC-4, CC-6
// ---------------------------------------------------------------------------

// TestClaudeAgentRegistered verifies that the blank import causes the claude
// agent to self-register and that agent.Lookup succeeds for constants.DefaultAgent.
// Validates: CC-1
func TestClaudeAgentRegistered(t *testing.T) {
	a, err := agent.Lookup(constants.DefaultAgent)
	require.NoError(t, err, "claude agent must be registered under constants.DefaultAgent")
	require.NotNil(t, a)
	require.Equal(t, constants.DefaultAgent, a.ID())
}

// TestClaudeInstallStepsPresent verifies that Install appends RUN steps that
// install Node.js and the @anthropic-ai/claude-code npm package.
// Validates: CC-2
func TestClaudeInstallStepsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.DefaultAgent)
	require.NoError(t, err)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "nodejs",
		"Dockerfile must contain a Node.js installation step")
	require.Contains(t, content, "@anthropic-ai/claude-code",
		"Dockerfile must contain an @anthropic-ai/claude-code installation step")
}

// TestClaudeCredentialPaths verifies that CredentialStorePath ends with ".claude".
// Validates: CC-3
func TestClaudeCredentialPaths(t *testing.T) {
	a, err := agent.Lookup(constants.DefaultAgent)
	require.NoError(t, err)

	storePath := a.CredentialStorePath()
	require.NotEmpty(t, storePath)
	require.Equal(t, ".claude", filepath.Base(storePath),
		"CredentialStorePath must end with .claude")
}

// TestClaudeContainerMountPath verifies that ContainerMountPath equals
// constants.ContainerUserHome + "/.claude".
// Validates: CC-4
func TestClaudeContainerMountPath(t *testing.T) {
	a, err := agent.Lookup(constants.DefaultAgent)
	require.NoError(t, err)

	want := constants.ContainerUserHome + "/.claude"
	require.Equal(t, want, a.ContainerMountPath())
}

// TestClaudeHasCredentialsEmpty verifies that HasCredentials returns (false, nil)
// when the store directory exists but contains no .credentials.json file.
// Validates: CC-6
func TestClaudeHasCredentialsEmpty(t *testing.T) {
	a, err := agent.Lookup(constants.DefaultAgent)
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
	a, err := agent.Lookup(constants.DefaultAgent)
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

	a, err := agent.Lookup(constants.DefaultAgent)
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
