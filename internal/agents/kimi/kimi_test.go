// Package kimi_test contains property-based and unit tests for the Kimi Code
// agent module. The blank import of the kimi package triggers its init()
// function, which registers the kimiAgent with the global agent registry.
package kimi_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/kimi"
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
// Unit tests — Task 4.1: agent registration, Install content, credential paths
// ---------------------------------------------------------------------------

// TestKimiAgentRegistered verifies that the blank import causes the kimi agent
// to self-register and that agent.Lookup succeeds for constants.KimiCodeAgentName.
func TestKimiAgentRegistered(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err, "kimi agent must be registered under constants.KimiCodeAgentName")
	require.NotNil(t, a)
	require.Equal(t, constants.KimiCodeAgentName, a.ID())
	require.Equal(t, "kimi-code", a.ID())
}

// TestKimiInstallStepsPresent verifies that Install appends the installer URL,
// curl, and ca-certificates, but does NOT install Node.js.
func TestKimiInstallStepsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "uv tool install kimi-cli",
		"Dockerfile must install kimi-cli via uv")
	require.Contains(t, content, "curl",
		"Dockerfile must install curl")
	require.Contains(t, content, "ca-certificates",
		"Dockerfile must install ca-certificates")
	require.NotContains(t, content, "setup_22.x",
		"Dockerfile must NOT install Node.js (Kimi Code is a native binary)")
}

// TestKimiInstallPreCreatesDir verifies that Install pre-creates the
// ~/.kimi-code directory and sets ownership to the container user.
func TestKimiInstallPreCreatesDir(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "mkdir -p /home/testuser/.local/bin /home/testuser/.kimi-code",
		"Dockerfile must pre-create ~/.local/bin and ~/.kimi-code")
	require.Contains(t, content, "chown -R testuser:testuser /home/testuser/.local /home/testuser/.kimi-code",
		"Dockerfile must chown ~/.local and ~/.kimi-code to the container user")
}

// TestKimiCredentialPaths verifies that CredentialStorePath ends with ".kimi-code".
func TestKimiCredentialPaths(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	storePath := a.CredentialStorePath()
	require.NotEmpty(t, storePath)
	require.Equal(t, ".kimi-code", filepath.Base(storePath),
		"CredentialStorePath must end with .kimi-code")
}

// TestKimiContainerMountPath verifies that ContainerMountPath equals
// "<homeDir>/.kimi-code".
func TestKimiContainerMountPath(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	want := "/home/testuser/.kimi-code"
	require.Equal(t, want, a.ContainerMountPath("/home/testuser"))
}

// ---------------------------------------------------------------------------
// Unit tests — Task 4.2: HasCredentials — all branches
// ---------------------------------------------------------------------------

// TestKimiHasCredentialsEmpty verifies that HasCredentials returns (false, nil)
// when the store directory exists but contains no credentials/ subdirectory.
func TestKimiHasCredentialsEmpty(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	// store dir exists but has no credentials/ subdir
	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.False(t, hasCreds,
		"HasCredentials must return false when credentials/ subdir does not exist")
}

// TestKimiHasCredentialsAbsentDir verifies that HasCredentials returns
// (false, nil) when the store path does not exist at all.
func TestKimiHasCredentialsAbsentDir(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "does-not-exist")

	hasCreds, err := a.HasCredentials(nonExistent)
	require.NoError(t, err)
	require.False(t, hasCreds,
		"HasCredentials must return false for absent store path")
}

// TestKimiHasCredentialsNoCredentialsSubdir verifies that HasCredentials
// returns (false, nil) when the store path exists but has no credentials/
// subdirectory (testing the subdir-specific check explicitly).
func TestKimiHasCredentialsNoCredentialsSubdir(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	// Create a file in the top-level store dir — this must NOT count as credentials
	// because Kimi Code checks credentials/ subdir, not the top-level dir.
	err = os.WriteFile(filepath.Join(tmpDir, "some-top-level-file.json"), []byte(`{"token":"test"}`), 0o600)
	require.NoError(t, err)

	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.False(t, hasCreds,
		"HasCredentials must return false when credentials/ subdir is absent, even if top-level files exist")
}

// TestKimiHasCredentialsEmptyFileIgnored verifies that a zero-length file
// inside credentials/ does not count as credentials.
func TestKimiHasCredentialsEmptyFileIgnored(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	credDir := filepath.Join(tmpDir, "credentials")
	require.NoError(t, os.MkdirAll(credDir, 0o700))
	err = os.WriteFile(filepath.Join(credDir, "empty"), []byte{}, 0o600)
	require.NoError(t, err)

	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.False(t, hasCreds,
		"a zero-length file in credentials/ must not count as credentials")
}

// TestKimiHasCredentialsPresent verifies that HasCredentials returns (true, nil)
// when a non-empty file exists inside the credentials/ subdirectory.
func TestKimiHasCredentialsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	credDir := filepath.Join(tmpDir, "credentials")
	require.NoError(t, os.MkdirAll(credDir, 0o700))
	err = os.WriteFile(filepath.Join(credDir, "token.json"), []byte(`{"access_token":"abc123"}`), 0o600)
	require.NoError(t, err)

	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.True(t, hasCreds,
		"HasCredentials must return true when credentials/ contains a non-empty file")
}

// TestKimiHasCredentialsReadError verifies that HasCredentials returns
// (false, error) when a filesystem error other than "not exists" occurs.
// Using a regular file as the storePath makes os.ReadDir on credentials/
// fail with a "not a directory" error.
func TestKimiHasCredentialsReadError(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	// Create a regular file and use it as storePath. When HasCredentials tries
	// to ReadDir(<storePath>/credentials), it will fail because <storePath> is
	// a file, not a directory — resulting in a non-IsNotExist error.
	tmpDir := t.TempDir()
	notADir := filepath.Join(tmpDir, "fakedir")
	err = os.WriteFile(notADir, []byte("not a directory"), 0o644)
	require.NoError(t, err)

	// Create a "credentials" entry inside the fake dir path — os.ReadDir will
	// fail because notADir is a regular file, not a directory.
	// Actually: filepath.Join(notADir, "credentials") is fine as a path, but
	// os.ReadDir will fail when trying to read notADir/credentials because
	// notADir itself is a file. We need the error to NOT be IsNotExist.
	// The cleanest approach: create a dir named like storePath, then put a
	// regular file at storePath/credentials (so ReadDir on it fails).
	storeDir := filepath.Join(tmpDir, "store")
	require.NoError(t, os.MkdirAll(storeDir, 0o700))
	// Put a regular file where credentials/ dir is expected
	credPath := filepath.Join(storeDir, "credentials")
	err = os.WriteFile(credPath, []byte("not a directory"), 0o644)
	require.NoError(t, err)

	hasCreds, credErr := a.HasCredentials(storeDir)
	require.False(t, hasCreds)
	require.Error(t, credErr)
	require.Contains(t, credErr.Error(), "checking kimi-code credentials")
}

// ---------------------------------------------------------------------------
// Unit tests — Task 4.3: SummaryInfo, DefaultAgents exclusion, AdditionalMounter
// ---------------------------------------------------------------------------

// TestKimiSummaryInfoReturnsNil verifies that the Kimi agent's SummaryInfo
// method returns (nil, nil) since it has no additional session summary info.
func TestKimiSummaryInfoReturnsNil(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err, "kimi agent must be registered")

	info, err := a.SummaryInfo(context.Background(), nil, "")
	require.NoError(t, err)
	require.Nil(t, info)
}

// TestKimiNotInDefaultAgents verifies that constants.DefaultAgents does NOT
// contain "kimi-code" — the Kimi Code agent is opt-in only.
func TestKimiNotInDefaultAgents(t *testing.T) {
	require.False(t, strings.Contains(constants.DefaultAgents, "kimi-code"),
		"constants.DefaultAgents must NOT contain 'kimi-code' — it is opt-in only")
}

// TestKimiNotAdditionalMounter verifies that the Kimi Code agent does not
// implement the optional AdditionalMounter interface (single credential directory).
func TestKimiNotAdditionalMounter(t *testing.T) {
	a, err := agent.Lookup(constants.KimiCodeAgentName)
	require.NoError(t, err)

	_, ok := a.(agent.AdditionalMounter)
	require.False(t, ok, "kimi agent must not implement AdditionalMounter")
}

// ---------------------------------------------------------------------------
// Property-based tests — Task 4.4: Agent ID is stable
// ---------------------------------------------------------------------------

// Feature: kimi-code-agent, Property 1: Agent ID is stable
func TestPropertyKimiAgentIDIsStable(t *testing.T) {
	// Validates: Requirement 1.1, Requirement 1.2
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KimiCodeAgentName)
		require.NoError(rt, err, "kimi agent must be registered under constants.KimiCodeAgentName")

		id := a.ID()
		require.Equal(rt, constants.KimiCodeAgentName, id,
			"ID() must always return constants.KimiCodeAgentName (%q)", constants.KimiCodeAgentName)
	})
}

// ---------------------------------------------------------------------------
// Property-based tests — Task 4.5: Container mount path tracks homeDir
// ---------------------------------------------------------------------------

// Feature: kimi-code-agent, Property 2: Container mount path tracks the runtime home directory
func TestPropertyKimiContainerMountPath(t *testing.T) {
	// Validates: Requirement 3.2
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KimiCodeAgentName)
		require.NoError(rt, err, "kimi agent must be registered")

		homeDir := rapid.StringMatching("/[a-zA-Z][a-zA-Z0-9_/.-]*").Draw(rt, "homeDir")
		mountPath := a.ContainerMountPath(homeDir)
		wantPath := filepath.Join(homeDir, ".kimi-code")

		require.Equal(rt, wantPath, mountPath,
			"ContainerMountPath(%q) must return %q", homeDir, wantPath)
	})
}

// ---------------------------------------------------------------------------
// Property-based tests — Task 4.6: HasCredentials consistent with credentials/ subdir state
// ---------------------------------------------------------------------------

// Feature: kimi-code-agent, Property 3: Credential presence check is consistent with credentials/ subdirectory state
func TestPropertyKimiCredentialPresenceConsistent(t *testing.T) {
	// Validates: Requirement 4.2, Requirement 4.3
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KimiCodeAgentName)
		require.NoError(rt, err, "kimi agent must be registered")

		tmpDir := t.TempDir()
		credDir := filepath.Join(tmpDir, "credentials")
		require.NoError(rt, os.MkdirAll(credDir, 0o700))

		hasFile := rapid.Bool().Draw(rt, "hasFile")
		fileSize := rapid.IntRange(0, 512).Draw(rt, "fileSize")

		if hasFile {
			content := make([]byte, fileSize)
			err := os.WriteFile(filepath.Join(credDir, "token.json"), content, 0o600)
			require.NoError(rt, err, "failed to create test credential file")
		}

		hasCreds, err := a.HasCredentials(tmpDir)
		require.NoError(rt, err, "HasCredentials must not error for a valid directory")

		wantHasCreds := hasFile && fileSize > 0
		require.Equal(rt, wantHasCreds, hasCreds,
			"HasCredentials must return true iff credentials/ contains a non-empty file")
	})
}

// Feature: kimi-code-agent, Property 3 (absent path variant): HasCredentials returns false for absent store path
func TestPropertyKimiCredentialAbsentDirectory(t *testing.T) {
	// Validates: Requirement 4.1
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KimiCodeAgentName)
		require.NoError(rt, err, "kimi agent must be registered")

		tmpDir := t.TempDir()
		nonExistent := filepath.Join(tmpDir, "does-not-exist")

		hasCreds, err := a.HasCredentials(nonExistent)
		require.NoError(rt, err, "HasCredentials must return (false, nil) for absent directory")
		require.False(rt, hasCreds, "HasCredentials must return false for absent directory")
	})
}

// ---------------------------------------------------------------------------
// Property-based tests — Task 4.7: Install includes installer URL and no Node.js
// ---------------------------------------------------------------------------

// Feature: kimi-code-agent, Property 4: Install always emits the Kimi Code installer URL and does not install Node.js
func TestPropertyKimiInstallIncludesInstallerNotNode(t *testing.T) {
	// Validates: Requirement 2.1, Requirement 2.4
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KimiCodeAgentName)
		require.NoError(rt, err, "kimi agent must be registered")

		b := newTestBuilder()
		markNode := rapid.Bool().Draw(rt, "markNode")
		if markNode {
			b.MarkNodeInstalled()
		}
		nodeStateBefore := b.IsNodeInstalled()

		a.Install(b)
		content := b.Build()

		require.Contains(rt, content, "uv tool install kimi-cli",
			"Dockerfile must install kimi-cli via uv")
		require.NotContains(rt, content, "setup_22.x",
			"Dockerfile must NOT include a Node.js setup step")

		// Install must not alter IsNodeInstalled state
		require.Equal(rt, nodeStateBefore, b.IsNodeInstalled(),
			"Install must not change the IsNodeInstalled state of the builder")
	})
}

// ---------------------------------------------------------------------------
// Property-based tests — Task 4.8: Install pre-creates .kimi-code with correct ownership
// ---------------------------------------------------------------------------

// Feature: kimi-code-agent, Property 5: Install always pre-creates the kimi-code directory with correct ownership
func TestPropertyKimiInstallPreCreatesDir(t *testing.T) {
	// Validates: Requirement 9.1, Requirement 9.2, Requirement 9.3, Requirement 9.5
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.KimiCodeAgentName)
		require.NoError(rt, err, "kimi agent must be registered")

		username := rapid.StringMatching("[a-z][a-z0-9_-]{0,15}").Draw(rt, "username")
		homeDir := rapid.StringMatching("/[a-zA-Z][a-zA-Z0-9_/.-]*").Draw(rt, "homeDir")

		b := docker.NewBaseImageBuilder(
			&hostinfo.Info{Username: username, HomeDir: homeDir, UID: 1000, GID: 1000},
			docker.UserStrategyCreate, "",
			"",
		)
		a.Install(b)
		content := b.Build()

		wantMkdir := "mkdir -p " + homeDir + "/.local/bin " + homeDir + "/.kimi-code"
		wantChown := "chown -R " + username + ":" + username + " " + homeDir + "/.local " + homeDir + "/.kimi-code"

		require.Contains(rt, content, wantMkdir,
			"Dockerfile must pre-create %s/.kimi-code", homeDir)
		require.Contains(rt, content, wantChown,
			"Dockerfile must chown %s/.kimi-code to %s", homeDir, username)
	})
}
