// Package kiro_test contains property-based and unit tests for the Kiro CLI
// agent module. The blank import of the kiro package triggers its init()
// function, which registers the kiroAgent with the global agent registry.
package kiro_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/kiro"
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
// Property: Kiro agent ID is stable
// ---------------------------------------------------------------------------

// Feature: kiro-agent, Property 1: Kiro agent ID is stable
func TestPropertyKiroAgentIDIsStable(t *testing.T) {
	// Validates: KIRO-1.2, KIRO-8.2
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KiroAgentName)
		require.NoError(rt, err, "kiro agent must be registered under constants.KiroAgentName")

		id := a.ID()
		require.Equal(rt, constants.KiroAgentName, id,
			"ID() must always return constants.KiroAgentName (%q)", constants.KiroAgentName)
	})
}

// ---------------------------------------------------------------------------
// Property: Kiro credential presence check is consistent
// ---------------------------------------------------------------------------

// Feature: kiro-agent, Property 2: Kiro credential presence check is consistent
func TestPropertyKiroCredentialPresenceConsistent(t *testing.T) {
	// Validates: KIRO-6.1, KIRO-6.2, KIRO-6.3
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KiroAgentName)
		require.NoError(rt, err, "kiro agent must be registered")

		tmpDir := t.TempDir()

		hasFile := rapid.Bool().Draw(rt, "hasFile")
		if hasFile {
			err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(`{"token":"test"}`), 0o600)
			require.NoError(rt, err, "failed to create test credential file")
		}

		hasCreds, err := a.HasCredentials(tmpDir)
		require.NoError(rt, err, "HasCredentials must not error for a valid directory")

		// A non-empty regular file in ~/.kiro counts as authenticated.
		require.Equal(rt, hasFile, hasCreds,
			"HasCredentials must return true iff a non-empty file exists in the store path")
	})
}

// Feature: kiro-agent, Property 2: Kiro credential presence check is consistent
func TestPropertyKiroCredentialAbsentDirectory(t *testing.T) {
	// Validates: KIRO-6.3
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KiroAgentName)
		require.NoError(rt, err, "kiro agent must be registered")

		tmpDir := t.TempDir()
		nonExistent := filepath.Join(tmpDir, "does-not-exist")

		hasCreds, err := a.HasCredentials(nonExistent)
		require.NoError(rt, err, "HasCredentials must return (false, nil) for absent directory")
		require.False(rt, hasCreds, "HasCredentials must return false for absent directory")
	})
}

// ---------------------------------------------------------------------------
// Property: Kiro container mount path uses runtime-provided home directory
// ---------------------------------------------------------------------------

// Feature: kiro-agent, Property 3: Kiro container mount path uses runtime-provided home directory
func TestPropertyKiroContainerMountPath(t *testing.T) {
	// Validates: KIRO-5.2
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KiroAgentName)
		require.NoError(rt, err, "kiro agent must be registered")

		homeDir := rapid.StringMatching("/[a-zA-Z][a-zA-Z0-9_/.-]*").Draw(rt, "homeDir")
		mountPath := a.ContainerMountPath(homeDir)
		wantPath := filepath.Join(homeDir, ".kiro")

		require.Equal(rt, wantPath, mountPath,
			"ContainerMountPath(%q) must return %q", homeDir, wantPath)
	})
}

// ---------------------------------------------------------------------------
// Property: Kiro Dockerfile steps install the CLI and put it on PATH
// ---------------------------------------------------------------------------

// Feature: kiro-agent, Property 4: Kiro Dockerfile steps install the CLI and Node.js
func TestPropertyKiroInstallIncludesCLIAndNode(t *testing.T) {
	// Validates: KIRO-2.1, KIRO-2.2, KIRO-2.3
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KiroAgentName)
		require.NoError(rt, err, "kiro agent must be registered")

		b := newTestBuilder()
		a.Install(b)
		content := b.Build()

		require.Contains(rt, content, "setup_22.x",
			"Dockerfile must include Node.js 22 setup step")
		require.Contains(rt, content, "cli.kiro.dev/install",
			"Dockerfile must include the Kiro CLI installer")
		require.Contains(rt, content, "/usr/local/bin/kiro-cli",
			"Dockerfile must symlink kiro-cli onto the default PATH")
	})
}

// ---------------------------------------------------------------------------
// Property: Kiro agent is registered and satisfies the Agent interface
// ---------------------------------------------------------------------------

// Feature: kiro-agent, Property 5: Kiro agent is registered and satisfies the Agent interface
func TestPropertyKiroAgentSatisfiesInterface(t *testing.T) {
	// Validates: KIRO-1.1, KIRO-8.2
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KiroAgentName)
		require.NoError(rt, err, "kiro agent must be registered under constants.KiroAgentName")
		require.NotNil(rt, a)

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

// TestKiroAgentRegistered verifies that the blank import causes the kiro agent
// to self-register and that agent.Lookup succeeds for constants.KiroAgentName.
func TestKiroAgentRegistered(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err, "kiro agent must be registered under constants.KiroAgentName")
	require.NotNil(t, a)
	require.Equal(t, constants.KiroAgentName, a.ID())
	require.Equal(t, "kiro", a.ID())
}

// TestKiroInstallStepsPresent verifies that Install appends RUN steps that
// install Node.js 22, the Kiro CLI, symlink it onto PATH, and pre-create ~/.kiro.
func TestKiroInstallStepsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "setup_22.x",
		"Dockerfile must contain a Node.js 22 setup step")
	require.Contains(t, content, "cli.kiro.dev/install",
		"Dockerfile must contain the Kiro CLI installer")
	require.Contains(t, content, "/usr/local/bin/kiro-cli",
		"Dockerfile must symlink kiro-cli onto the default PATH")
	require.Contains(t, content, "mkdir -p /home/testuser/.kiro",
		"Dockerfile must pre-create the ~/.kiro credential directory")
	require.Contains(t, content, "chown -R testuser:testuser /home/testuser/.kiro",
		"Dockerfile must chown the ~/.kiro directory to the container user")
	require.Contains(t, content, "unzip",
		"Dockerfile must install unzip for the Kiro installer")
}

// TestKiroCredentialPaths verifies that CredentialStorePath ends with ".kiro".
func TestKiroCredentialPaths(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	storePath := a.CredentialStorePath()
	require.NotEmpty(t, storePath)
	require.Equal(t, ".kiro", filepath.Base(storePath),
		"CredentialStorePath must end with .kiro")
}

// TestKiroContainerMountPath verifies that ContainerMountPath equals
// "<homeDir>/.kiro".
func TestKiroContainerMountPath(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	want := "/home/testuser/.kiro"
	require.Equal(t, want, a.ContainerMountPath("/home/testuser"))
}

// TestKiroHasCredentialsEmpty verifies that HasCredentials returns (false, nil)
// when the store directory exists but contains no files.
func TestKiroHasCredentialsEmpty(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.False(t, hasCreds)
}

// TestKiroHasCredentialsAbsentDir verifies that HasCredentials returns
// (false, nil) when the store directory does not exist.
func TestKiroHasCredentialsAbsentDir(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "does-not-exist")

	hasCreds, err := a.HasCredentials(nonExistent)
	require.NoError(t, err)
	require.False(t, hasCreds)
}

// TestKiroHasCredentialsEmptyFileIgnored verifies that a zero-length file does
// not count as credentials.
func TestKiroHasCredentialsEmptyFileIgnored(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	err = os.WriteFile(filepath.Join(tmpDir, "empty"), []byte{}, 0o600)
	require.NoError(t, err)

	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.False(t, hasCreds, "a zero-length file must not count as credentials")
}

// TestKiroHasCredentialsPresent verifies that HasCredentials returns (true, nil)
// when a non-empty file exists inside the store directory.
func TestKiroHasCredentialsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	err = os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(`{"token":"test"}`), 0o600)
	require.NoError(t, err)

	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.True(t, hasCreds)
}

// TestKiroHasCredentialsReadError verifies that HasCredentials returns
// (false, error) when a filesystem error other than "not exists" occurs.
func TestKiroHasCredentialsReadError(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	// Use a regular file as the store path. os.ReadDir on a non-directory
	// returns a "not a directory" error deterministically.
	tmpDir := t.TempDir()
	notADir := filepath.Join(tmpDir, "fakedir")
	err = os.WriteFile(notADir, []byte("not a directory"), 0o644)
	require.NoError(t, err)

	hasCreds, credErr := a.HasCredentials(notADir)
	require.False(t, hasCreds)
	require.Error(t, credErr)
	require.Contains(t, credErr.Error(), "checking kiro credentials")
}

// TestKiroInstallNodeNotInstalled verifies that when IsNodeInstalled() returns
// false, Kiro appends Node.js install steps and calls MarkNodeInstalled().
func TestKiroInstallNodeNotInstalled(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	require.False(t, b.IsNodeInstalled(), "fresh builder must have IsNodeInstalled() == false")

	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "setup_22.x",
		"must install Node.js 22 when not already installed")
	require.Contains(t, content, "nodejs",
		"must install nodejs package when not already installed")
	require.Contains(t, content, "cli.kiro.dev/install",
		"must always install the Kiro CLI")
	require.True(t, b.IsNodeInstalled(),
		"MarkNodeInstalled() must be called after Node.js installation")
}

// TestKiroInstallNodeAlreadyInstalled verifies that when IsNodeInstalled()
// returns true, Kiro skips the Node.js setup step but still installs its CLI.
func TestKiroInstallNodeAlreadyInstalled(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	b.MarkNodeInstalled() // simulate a prior agent having installed Node.js
	require.True(t, b.IsNodeInstalled())

	a.Install(b)
	content := b.Build()

	require.NotContains(t, content, "setup_22.x",
		"must skip Node.js setup when already installed")
	require.Contains(t, content, "cli.kiro.dev/install",
		"must always install the Kiro CLI")
	require.Contains(t, content, "curl ca-certificates git",
		"must always install curl, ca-certificates, git")
}

// TestKiroSummaryInfoReturnsNil verifies that the Kiro agent's SummaryInfo
// method returns (nil, nil) since it has no additional session summary info.
func TestKiroSummaryInfoReturnsNil(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err, "kiro agent must be registered")

	info, err := a.SummaryInfo(context.Background(), nil, "")
	require.NoError(t, err)
	require.Nil(t, info)
}

// TestKiroNotInDefaultAgents verifies that constants.DefaultAgents does NOT
// contain "kiro" — the Kiro agent is opt-in only.
func TestKiroNotInDefaultAgents(t *testing.T) {
	require.False(t, strings.Contains(constants.DefaultAgents, "kiro"),
		"constants.DefaultAgents must NOT contain 'kiro' — it is opt-in only")
}

// TestKiroNotAdditionalMounter verifies that the Kiro agent does not implement
// the optional AdditionalMounter interface (single credential directory).
func TestKiroNotAdditionalMounter(t *testing.T) {
	a, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err)

	_, ok := a.(agent.AdditionalMounter)
	require.False(t, ok, "kiro agent must not implement AdditionalMounter")
}
