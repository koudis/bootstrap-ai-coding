// Package codex_test contains property-based and unit tests for the OpenAI
// Codex CLI agent module. The blank import of the codex package triggers its
// init() function, which registers the codexAgent with the global agent
// registry.
package codex_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/codex"
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
// Property 1: Codex agent ID is stable
// ---------------------------------------------------------------------------

// Feature: codex-agent, Property 1: Codex agent ID is stable
func TestPropertyCodexAgentIDIsStable(t *testing.T) {
	// Validates: Requirements 1.2, 6.2
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.CodexAgentName)
		require.NoError(rt, err, "codex agent must be registered under constants.CodexAgentName")

		id := a.ID()
		require.Equal(rt, constants.CodexAgentName, id,
			"ID() must always return constants.CodexAgentName (%q)", constants.CodexAgentName)
	})
}

// ---------------------------------------------------------------------------
// Property 2: Codex credential presence check is consistent
// ---------------------------------------------------------------------------

// Feature: codex-agent, Property 2: Codex credential presence check is consistent
func TestPropertyCodexCredentialPresenceConsistent(t *testing.T) {
	// Validates: Requirements 4.1, 4.2, 4.3
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.CodexAgentName)
		require.NoError(rt, err, "codex agent must be registered")

		tmpDir := t.TempDir()

		hasFile := rapid.Bool().Draw(rt, "hasFile")

		if hasFile {
			err := os.WriteFile(filepath.Join(tmpDir, "auth.json"), []byte(`{"token":"test"}`), 0o600)
			require.NoError(rt, err, "failed to create test credential file")
		}

		hasCreds, err := a.HasCredentials(tmpDir)
		require.NoError(rt, err, "HasCredentials must not error for a valid directory")

		// Codex checks auth.json existence (not file size)
		require.Equal(rt, hasFile, hasCreds,
			"HasCredentials must return true iff auth.json exists in the store path")
	})
}

// Feature: codex-agent, Property 2: Codex credential presence check is consistent
func TestPropertyCodexCredentialAbsentDirectory(t *testing.T) {
	// Validates: Requirements 4.1, 4.2, 4.3
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.CodexAgentName)
		require.NoError(rt, err, "codex agent must be registered")

		// Use a path that does not exist.
		tmpDir := t.TempDir()
		nonExistent := filepath.Join(tmpDir, "does-not-exist")

		hasCreds, err := a.HasCredentials(nonExistent)
		require.NoError(rt, err, "HasCredentials must return (false, nil) for absent directory")
		require.False(rt, hasCreds, "HasCredentials must return false for absent directory")
	})
}

// ---------------------------------------------------------------------------
// Property 3: Codex container mount path uses runtime-provided home directory
// ---------------------------------------------------------------------------

// Feature: codex-agent, Property 3: Codex container mount path uses runtime-provided home directory
func TestPropertyCodexContainerMountPath(t *testing.T) {
	// Validates: Requirements 3.2
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.CodexAgentName)
		require.NoError(rt, err, "codex agent must be registered")

		homeDir := rapid.StringMatching("/[a-zA-Z][a-zA-Z0-9_/.-]*").Draw(rt, "homeDir")
		mountPath := a.ContainerMountPath(homeDir)
		wantPath := filepath.Join(homeDir, ".codex")

		require.Equal(rt, wantPath, mountPath,
			"ContainerMountPath(%q) must return %q", homeDir, wantPath)
	})
}

// ---------------------------------------------------------------------------
// Property 4: Codex Dockerfile steps include Node.js 22 and @openai/codex package
// ---------------------------------------------------------------------------

// Feature: codex-agent, Property 4: Codex Dockerfile steps include Node.js 22 and @openai/codex package
func TestPropertyCodexInstallIncludesNodeAndPackage(t *testing.T) {
	// Validates: Requirements 2.2, 2.3, 2.4
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.CodexAgentName)
		require.NoError(rt, err, "codex agent must be registered")

		b := newTestBuilder()
		a.Install(b)
		content := b.Build()

		require.Contains(rt, content, "setup_22.x",
			"Dockerfile must include Node.js 22 setup step")
		require.Contains(rt, content, "@openai/codex",
			"Dockerfile must include @openai/codex installation step")
	})
}

// ---------------------------------------------------------------------------
// Property 5: Codex agent is registered and satisfies the Agent interface
// ---------------------------------------------------------------------------

// Feature: codex-agent, Property 5: Codex agent is registered and satisfies the Agent interface
func TestPropertyCodexAgentSatisfiesInterface(t *testing.T) {
	// Validates: Requirements 1.1, 1.3, 10.1
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.CodexAgentName)
		require.NoError(rt, err, "codex agent must be registered under constants.CodexAgentName")
		require.NotNil(rt, a)

		// Verify all interface methods are callable without panicking.
		id := a.ID()
		require.NotEmpty(rt, id, "agent ID must not be empty")

		b := newTestBuilder()
		a.Install(b)

		credPath := a.CredentialStorePath()
		require.NotEmpty(rt, credPath, "CredentialStorePath must not be empty")

		mountPath := a.ContainerMountPath("/home/testuser")
		require.NotEmpty(rt, mountPath, "ContainerMountPath must not be empty")

		tmpDir := t.TempDir()
		_, err = a.HasCredentials(tmpDir)
		require.NoError(rt, err, "HasCredentials on a valid temp dir must not error")

		// HealthCheck requires a live Docker daemon — covered by integration tests.
	})
}

// ---------------------------------------------------------------------------
// Unit tests
// ---------------------------------------------------------------------------

// TestCodexAgentRegistered verifies that the blank import causes the codex
// agent to self-register and that agent.Lookup succeeds for constants.CodexAgentName.
func TestCodexAgentRegistered(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
	require.NoError(t, err, "codex agent must be registered under constants.CodexAgentName")
	require.NotNil(t, a)
	require.Equal(t, constants.CodexAgentName, a.ID())
}

// TestCodexInstallStepsPresent verifies that Install appends RUN steps that
// install Node.js 22 (setup_22.x) and the @openai/codex npm package.
func TestCodexInstallStepsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "setup_22.x",
		"Dockerfile must contain a Node.js 22 setup step")
	require.Contains(t, content, "@openai/codex",
		"Dockerfile must contain an @openai/codex installation step")
}

// TestCodexCredentialPaths verifies that CredentialStorePath ends with ".codex".
func TestCodexCredentialPaths(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
	require.NoError(t, err)

	storePath := a.CredentialStorePath()
	require.NotEmpty(t, storePath)
	require.Equal(t, ".codex", filepath.Base(storePath),
		"CredentialStorePath must end with .codex")
}

// TestCodexContainerMountPath verifies that ContainerMountPath equals
// "<homeDir>/.codex".
func TestCodexContainerMountPath(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
	require.NoError(t, err)

	want := "/home/testuser/.codex"
	require.Equal(t, want, a.ContainerMountPath("/home/testuser"))
}

// TestCodexHasCredentialsEmpty verifies that HasCredentials returns (false, nil)
// when the store directory exists but contains no files.
func TestCodexHasCredentialsEmpty(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.False(t, hasCreds)
}

// TestCodexHasCredentialsAbsentDir verifies that HasCredentials returns
// (false, nil) when the store directory does not exist.
func TestCodexHasCredentialsAbsentDir(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "does-not-exist")

	hasCreds, err := a.HasCredentials(nonExistent)
	require.NoError(t, err)
	require.False(t, hasCreds)
}

// TestCodexHasCredentialsPresent verifies that HasCredentials returns (true, nil)
// when auth.json exists inside the store directory.
func TestCodexHasCredentialsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	err = os.WriteFile(filepath.Join(tmpDir, "auth.json"), []byte(`{"token":"test"}`), 0o600)
	require.NoError(t, err)

	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.True(t, hasCreds)
}

// TestCodexHasCredentialsStatError verifies that HasCredentials returns
// (false, error) when a filesystem error other than "not exists" occurs.
func TestCodexHasCredentialsStatError(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
	require.NoError(t, err)

	// Use a regular file as the store path. When HasCredentials tries to stat
	// "auth.json" inside it (filepath.Join(file, "auth.json")), the OS returns
	// a "not a directory" error deterministically — no chmod tricks needed.
	tmpDir := t.TempDir()
	notADir := filepath.Join(tmpDir, "fakedir")
	err = os.WriteFile(notADir, []byte("not a directory"), 0o644)
	require.NoError(t, err)

	hasCreds, credErr := a.HasCredentials(notADir)
	require.False(t, hasCreds)
	require.Error(t, credErr)
	require.Contains(t, credErr.Error(), "checking codex credentials")
}

// TestCodexInstallNodeNotInstalled verifies that when IsNodeInstalled()
// returns false, Codex appends Node.js install steps and calls MarkNodeInstalled().
func TestCodexInstallNodeNotInstalled(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	require.False(t, b.IsNodeInstalled(), "fresh builder must have IsNodeInstalled() == false")

	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "setup_22.x",
		"must install Node.js 22 when not already installed")
	require.Contains(t, content, "nodejs",
		"must install nodejs package when not already installed")
	require.Contains(t, content, "@openai/codex",
		"must always install the @openai/codex npm package")
	require.True(t, b.IsNodeInstalled(),
		"MarkNodeInstalled() must be called after Node.js installation")
}

// TestCodexInstallNodeAlreadyInstalled verifies that when IsNodeInstalled()
// returns true, Codex skips Node.js install steps but still installs its npm package.
func TestCodexInstallNodeAlreadyInstalled(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
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
	require.Contains(t, content, "@openai/codex",
		"must always install the @openai/codex npm package")

	// Must still install curl/ca-certificates/git (idempotent prereqs)
	require.Contains(t, content, "curl ca-certificates git",
		"must always install curl, ca-certificates, git")

	// Should have added exactly 2 lines (apt-get prereqs + npm install)
	linesAfter := len(b.Lines())
	require.Equal(t, linesBefore+2, linesAfter,
		"must add exactly 2 RUN steps when Node.js is already installed (prereqs + npm)")
}

// TestCodexSummaryInfoReturnsNil verifies that the Codex agent's SummaryInfo
// method returns (nil, nil) since it has no additional session summary info.
func TestCodexSummaryInfoReturnsNil(t *testing.T) {
	a, err := agent.Lookup(constants.CodexAgentName)
	require.NoError(t, err, "codex agent must be registered")

	info, err := a.SummaryInfo(context.Background(), nil, "")
	require.NoError(t, err)
	require.Nil(t, info)
}

// TestCodexInDefaultAgents verifies that constants.DefaultAgents contains "codex"
// as part of the expanded five-agent default set.
func TestCodexInDefaultAgents(t *testing.T) {
	require.True(t, strings.Contains(constants.DefaultAgents, "codex"),
		"constants.DefaultAgents must contain 'codex' — it is now a default agent")
}
