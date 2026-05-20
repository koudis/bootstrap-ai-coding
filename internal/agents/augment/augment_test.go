// Package augment_test contains property-based and unit tests for the Augment
// Code agent module. The blank import of the augment package triggers its
// init() function, which registers the augmentAgent with the global agent
// registry.
package augment_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
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
// Property 45: Augment Code agent ID is stable
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 45: Augment Code agent ID is stable
func TestPropertyAugmentAgentIDIsStable(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.AugmentCodeAgentName)
		require.NoError(rt, err, "augment agent must be registered under constants.AugmentCodeAgentName")

		id := a.ID()
		require.Equal(rt, constants.AugmentCodeAgentName, id,
			"ID() must always return constants.AugmentCodeAgentName (%q)", constants.AugmentCodeAgentName)
	})
}

// ---------------------------------------------------------------------------
// Property 46: Augment Code credential presence check is consistent
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 46: Augment Code credential presence check is consistent
func TestPropertyAugmentCredentialPresenceConsistent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.AugmentCodeAgentName)
		require.NoError(rt, err, "augment agent must be registered")

		tmpDir := t.TempDir()

		hasFile := rapid.Bool().Draw(rt, "hasFile")
		isEmpty := rapid.Bool().Draw(rt, "isEmpty")

		if hasFile {
			content := []byte("token-data")
			if isEmpty {
				content = []byte{}
			}
			err := os.WriteFile(filepath.Join(tmpDir, "auth.json"), content, 0o600)
			require.NoError(rt, err, "failed to create test credential file")
		}

		hasCreds, err := a.HasCredentials(tmpDir)
		require.NoError(rt, err, "HasCredentials must not error for a valid directory")

		wantCreds := hasFile && !isEmpty
		require.Equal(rt, wantCreds, hasCreds,
			"HasCredentials must return true iff a non-empty file exists in the store path")
	})
}

// Feature: bootstrap-ai-coding, Property 46b: Augment Code HasCredentials returns false for absent directory
func TestPropertyAugmentCredentialAbsentDirectory(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.AugmentCodeAgentName)
		require.NoError(rt, err, "augment agent must be registered")

		// Use a path that does not exist.
		tmpDir := t.TempDir()
		nonExistent := filepath.Join(tmpDir, "does-not-exist")

		hasCreds, err := a.HasCredentials(nonExistent)
		require.NoError(rt, err, "HasCredentials must return (false, nil) for absent directory")
		require.False(rt, hasCreds, "HasCredentials must return false for absent directory")
	})
}

// ---------------------------------------------------------------------------
// Property 47: Augment Code container mount path is always constants.ContainerUserHome/.augment
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 47: Augment Code container mount path is always constants.ContainerUserHome/.augment
func TestPropertyAugmentContainerMountPath(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.AugmentCodeAgentName)
		require.NoError(rt, err, "augment agent must be registered")

		mountPath := a.ContainerMountPath("/home/testuser")
		wantPath := "/home/testuser/.augment"

		require.Equal(rt, wantPath, mountPath,
			"ContainerMountPath() must always return %q", wantPath)
	})
}

// ---------------------------------------------------------------------------
// Property 48: Augment Code Dockerfile steps include Node.js 22+ and auggie package
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 48: Augment Code Dockerfile steps include Node.js 22+ and auggie package
func TestPropertyAugmentInstallIncludesNodeAndPackage(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.AugmentCodeAgentName)
		require.NoError(rt, err, "augment agent must be registered")

		b := newTestBuilder()
		a.Install(b)
		content := b.Build()

		require.Contains(rt, content, "setup_22.x",
			"Dockerfile must include Node.js 22 setup step")
		require.Contains(rt, content, "@augmentcode/auggie",
			"Dockerfile must include @augmentcode/auggie installation step")
	})
}

// ---------------------------------------------------------------------------
// Property 49: Augment Code agent is registered and satisfies the Agent interface
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 49: Augment Code agent is registered and satisfies the Agent interface
func TestPropertyAugmentAgentSatisfiesInterface(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.AugmentCodeAgentName)
		require.NoError(rt, err, "augment agent must be registered under constants.AugmentCodeAgentName")
		require.NotNil(rt, a)

		// Verify all six interface methods are callable without panicking.
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
// Unit tests — AC-1, AC-2, AC-3, AC-4, AC-6
// ---------------------------------------------------------------------------

// TestAugmentAgentRegistered verifies that the blank import causes the augment
// agent to self-register and that agent.Lookup succeeds for constants.AugmentCodeAgentName.
// Validates: AC-1
func TestAugmentAgentRegistered(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err, "augment agent must be registered under constants.AugmentCodeAgentName")
	require.NotNil(t, a)
	require.Equal(t, constants.AugmentCodeAgentName, a.ID())
}

// TestAugmentInstallStepsPresent verifies that Install appends RUN steps that
// install Node.js 22 (setup_22.x) and the @augmentcode/auggie npm package.
// Validates: AC-2
func TestAugmentInstallStepsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "setup_22.x",
		"Dockerfile must contain a Node.js 22 setup step")
	require.Contains(t, content, "@augmentcode/auggie",
		"Dockerfile must contain an @augmentcode/auggie installation step")
}

// TestAugmentCredentialPaths verifies that CredentialStorePath ends with ".augment".
// Validates: AC-3
func TestAugmentCredentialPaths(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err)

	storePath := a.CredentialStorePath()
	require.NotEmpty(t, storePath)
	require.Equal(t, ".augment", filepath.Base(storePath),
		"CredentialStorePath must end with .augment")
}

// TestAugmentContainerMountPath verifies that ContainerMountPath equals
// "<homeDir>/.augment".
// Validates: AC-4
func TestAugmentContainerMountPath(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err)

	want := "/home/testuser/.augment"
	require.Equal(t, want, a.ContainerMountPath("/home/testuser"))
}

// TestAugmentHasCredentialsEmpty verifies that HasCredentials returns (false, nil)
// when the store directory exists but contains no files.
// Validates: AC-4
func TestAugmentHasCredentialsEmpty(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.False(t, hasCreds)
}

// TestAugmentHasCredentialsAbsentDir verifies that HasCredentials returns
// (false, nil) when the store directory does not exist.
// Validates: AC-4
func TestAugmentHasCredentialsAbsentDir(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "does-not-exist")

	hasCreds, err := a.HasCredentials(nonExistent)
	require.NoError(t, err)
	require.False(t, hasCreds)
}

// TestAugmentHasCredentialsPresent verifies that HasCredentials returns (true, nil)
// when a non-empty file exists inside the store directory.
// Validates: AC-4
func TestAugmentHasCredentialsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	err = os.WriteFile(filepath.Join(tmpDir, "auth.json"), []byte(`{"token":"test"}`), 0o600)
	require.NoError(t, err)

	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.True(t, hasCreds)
}

// TestAugmentHasCredentialsEmptyFile verifies that HasCredentials returns
// (false, nil) when the store directory contains only empty files.
// Validates: AC-4
func TestAugmentHasCredentialsEmptyFile(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	err = os.WriteFile(filepath.Join(tmpDir, "empty.json"), []byte{}, 0o600)
	require.NoError(t, err)

	hasCreds, err := a.HasCredentials(tmpDir)
	require.NoError(t, err)
	require.False(t, hasCreds, "empty file should not count as credentials")
}

// ---------------------------------------------------------------------------
// SummaryInfo no-op test
// ---------------------------------------------------------------------------

// TestSummaryInfoReturnsNil verifies that the Augment Code agent's SummaryInfo
// method returns (nil, nil) since it has no additional session summary info.
// Validates: SI-6.2
func TestSummaryInfoReturnsNil(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err, "augment agent must be registered")

	info, err := a.SummaryInfo(context.Background(), nil, "")
	require.NoError(t, err)
	require.Nil(t, info)
}

// ---------------------------------------------------------------------------
// Node.js deduplication tests
// ---------------------------------------------------------------------------

// TestAugmentInstallNodeNotInstalled verifies that when IsNodeInstalled()
// returns false, Augment appends Node.js install steps and calls MarkNodeInstalled().
func TestAugmentInstallNodeNotInstalled(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	require.False(t, b.IsNodeInstalled(), "fresh builder must have IsNodeInstalled() == false")

	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "setup_22.x",
		"must install Node.js 22 when not already installed")
	require.Contains(t, content, "nodejs",
		"must install nodejs package when not already installed")
	require.Contains(t, content, "@augmentcode/auggie",
		"must always install the auggie npm package")
	require.True(t, b.IsNodeInstalled(),
		"MarkNodeInstalled() must be called after Node.js installation")
}

// TestAugmentInstallNodeAlreadyInstalled verifies that when IsNodeInstalled()
// returns true, Augment skips Node.js install steps but still installs its npm package.
func TestAugmentInstallNodeAlreadyInstalled(t *testing.T) {
	a, err := agent.Lookup(constants.AugmentCodeAgentName)
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
	require.Contains(t, content, "@augmentcode/auggie",
		"must always install the auggie npm package")

	// Must still install curl/ca-certificates/git (idempotent prereqs)
	require.Contains(t, content, "curl ca-certificates git",
		"must always install curl, ca-certificates, git")

	// Should have added exactly 2 lines (apt-get prereqs + npm install)
	linesAfter := len(b.Lines())
	require.Equal(t, linesBefore+2, linesAfter,
		"must add exactly 2 RUN steps when Node.js is already installed (prereqs + npm)")
}
