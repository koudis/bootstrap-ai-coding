// Package opencode_test contains property-based and unit tests for the OpenCode
// AI coding agent module. The blank import of the opencode package triggers its
// init() function, which registers the opencodeAgent with the global agent
// registry.
package opencode_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/opencode"
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
// Property 1: OpenCode agent ID is stable
// ---------------------------------------------------------------------------

// Feature: opencode-agent, Property 1: OpenCode agent ID is stable
func TestPropertyOpenCodeAgentIDIsStable(t *testing.T) {
	// Validates: Requirements 1.1, 1.2, 1.3
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.OpenCodeAgentName)
		require.NoError(rt, err, "opencode agent must be registered under constants.OpenCodeAgentName")

		id := a.ID()
		require.Equal(rt, constants.OpenCodeAgentName, id,
			"ID() must always return constants.OpenCodeAgentName (%q)", constants.OpenCodeAgentName)
	})
}

// ---------------------------------------------------------------------------
// Property 2: OpenCode credential presence check is consistent
// ---------------------------------------------------------------------------

// Feature: opencode-agent, Property 2: OpenCode credential presence check is consistent
func TestPropertyOpenCodeCredentialPresenceConsistent(t *testing.T) {
	// Validates: Requirements 4.1, 4.2, 4.3, 4.4
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.OpenCodeAgentName)
		require.NoError(rt, err, "opencode agent must be registered")

		tmpDir := t.TempDir()

		hasFile := rapid.Bool().Draw(rt, "hasFile")

		if hasFile {
			// Generate content of varying length: 0 means empty file, >0 means non-empty
			contentLen := rapid.IntRange(0, 100).Draw(rt, "contentLen")
			var content []byte
			if contentLen > 0 {
				content = []byte(strings.Repeat("x", contentLen))
			}
			err := os.WriteFile(filepath.Join(tmpDir, "auth.json"), content, 0o600)
			require.NoError(rt, err, "failed to create test credential file")

			hasCreds, credErr := a.HasCredentials(tmpDir)
			require.NoError(rt, credErr, "HasCredentials must not error for a valid directory")

			// OpenCode checks file SIZE > 0 (unlike Codex which just checks existence)
			if contentLen > 0 {
				require.True(rt, hasCreds,
					"HasCredentials must return true when auth.json exists with size > 0")
			} else {
				require.False(rt, hasCreds,
					"HasCredentials must return false when auth.json exists with size == 0")
			}
		} else {
			// No auth.json file
			hasCreds, credErr := a.HasCredentials(tmpDir)
			require.NoError(rt, credErr, "HasCredentials must not error for absent file")
			require.False(rt, hasCreds,
				"HasCredentials must return false when auth.json does not exist")
		}
	})
}

// Feature: opencode-agent, Property 2: OpenCode credential presence check is consistent
func TestPropertyOpenCodeCredentialAbsentDirectory(t *testing.T) {
	// Validates: Requirements 4.1, 4.2, 4.3, 4.4
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.OpenCodeAgentName)
		require.NoError(rt, err, "opencode agent must be registered")

		// Use a path that does not exist.
		tmpDir := t.TempDir()
		nonExistent := filepath.Join(tmpDir, "does-not-exist")

		hasCreds, credErr := a.HasCredentials(nonExistent)
		require.NoError(rt, credErr, "HasCredentials must return (false, nil) for absent directory")
		require.False(rt, hasCreds, "HasCredentials must return false for absent directory")
	})
}

// ---------------------------------------------------------------------------
// Property 3: OpenCode container mount path uses runtime-provided home directory
// ---------------------------------------------------------------------------

// Feature: opencode-agent, Property 3: OpenCode container mount path uses runtime-provided home directory
func TestPropertyOpenCodeContainerMountPath(t *testing.T) {
	// Validates: Requirements 3.2
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.OpenCodeAgentName)
		require.NoError(rt, err, "opencode agent must be registered")

		homeDir := rapid.StringMatching("/[a-zA-Z][a-zA-Z0-9_/.-]*").Draw(rt, "homeDir")
		mountPath := a.ContainerMountPath(homeDir)
		wantPath := filepath.Join(homeDir, ".local", "share", "opencode")

		require.Equal(rt, wantPath, mountPath,
			"ContainerMountPath(%q) must return %q", homeDir, wantPath)
	})
}

// ---------------------------------------------------------------------------
// Property 4: OpenCode Dockerfile steps include Node.js 22 and opencode-ai package
// ---------------------------------------------------------------------------

// Feature: opencode-agent, Property 4: OpenCode Dockerfile steps include Node.js 22 and opencode-ai package
func TestPropertyOpenCodeInstallIncludesNodeAndPackage(t *testing.T) {
	// Validates: Requirements 2.1, 2.2, 2.3
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.OpenCodeAgentName)
		require.NoError(rt, err, "opencode agent must be registered")

		nodePreInstalled := rapid.Bool().Draw(rt, "nodePreInstalled")

		b := newTestBuilder()
		if nodePreInstalled {
			b.MarkNodeInstalled()
		}

		a.Install(b)
		content := b.Build()

		// Must always contain opencode-ai
		require.Contains(rt, content, "opencode-ai",
			"Dockerfile must include opencode-ai installation step")

		if !nodePreInstalled {
			// When Node.js is NOT pre-installed: must contain setup_22.x
			require.Contains(rt, content, "setup_22.x",
				"Dockerfile must include Node.js 22 setup step when not pre-installed")
		} else {
			// When Node.js IS pre-installed: must NOT contain setup_22.x
			require.NotContains(rt, content, "setup_22.x",
				"Dockerfile must NOT include Node.js 22 setup step when already pre-installed")
		}
	})
}

// ---------------------------------------------------------------------------
// Property 5: OpenCode additional mounts declare the config store
// ---------------------------------------------------------------------------

// Feature: opencode-agent, Property 5: OpenCode additional mounts declare the config store
func TestPropertyOpenCodeAdditionalMounts(t *testing.T) {
	// Validates: Requirements 3.3, 3.4
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.OpenCodeAgentName)
		require.NoError(rt, err, "opencode agent must be registered")

		homeDir := rapid.StringMatching("/[a-zA-Z][a-zA-Z0-9_/.-]*").Draw(rt, "homeDir")

		// Type-assert to AdditionalMounter
		mounter, ok := a.(agent.AdditionalMounter)
		require.True(rt, ok, "opencode agent must implement AdditionalMounter")

		mounts := mounter.AdditionalMounts(homeDir)
		require.Len(rt, mounts, 1,
			"AdditionalMounts must return exactly one mount")

		mount := mounts[0]
		wantContainerPath := filepath.Join(homeDir, ".config", "opencode")
		require.Equal(rt, wantContainerPath, mount.ContainerPath,
			"mount ContainerPath must be <homeDir>/.config/opencode")
		require.False(rt, mount.ReadOnly,
			"mount ReadOnly must be false")
		require.True(rt, strings.HasSuffix(mount.HostPath, filepath.Join(".config", "opencode")),
			"mount HostPath must end with .config/opencode, got %q", mount.HostPath)
	})
}

// ---------------------------------------------------------------------------
// Property 6: OpenCode agent satisfies the Agent interface without panicking
// ---------------------------------------------------------------------------

// Feature: opencode-agent, Property 6: OpenCode agent satisfies the Agent interface without panicking
func TestPropertyOpenCodeAgentSatisfiesInterface(t *testing.T) {
	// Validates: Requirements 1.1, 1.3, 6.1, 6.2
	rapid.Check(t, func(rt *rapid.T) {
		a, err := agent.Lookup(constants.OpenCodeAgentName)
		require.NoError(rt, err, "opencode agent must be registered under constants.OpenCodeAgentName")
		require.NotNil(rt, a)

		// Verify all interface methods are callable without panicking.
		id := a.ID()
		require.NotEmpty(rt, id, "agent ID must not be empty")

		b := newTestBuilder()
		a.Install(b)

		credPath := a.CredentialStorePath()
		require.NotEmpty(rt, credPath, "CredentialStorePath must not be empty")

		homeDir := rapid.StringMatching("/[a-zA-Z][a-zA-Z0-9_/.-]*").Draw(rt, "homeDir")
		mountPath := a.ContainerMountPath(homeDir)
		require.NotEmpty(rt, mountPath, "ContainerMountPath must not be empty")

		tmpDir := t.TempDir()
		_, credErr := a.HasCredentials(tmpDir)
		require.NoError(rt, credErr, "HasCredentials on a valid temp dir must not error")

		// Verify AdditionalMounter interface
		mounter, ok := a.(agent.AdditionalMounter)
		require.True(rt, ok, "opencode agent must implement AdditionalMounter")
		mounts := mounter.AdditionalMounts(homeDir)
		require.NotEmpty(rt, mounts, "AdditionalMounts must return at least one mount")

		// SummaryInfo with nil client (no Docker needed for this agent)
		info, summaryErr := a.SummaryInfo(context.Background(), nil, "")
		require.NoError(rt, summaryErr)
		require.Nil(rt, info)

		// HealthCheck requires a live Docker daemon — covered by integration tests.
	})
}

// ---------------------------------------------------------------------------
// Unit tests
// ---------------------------------------------------------------------------

// TestOpenCodeAgentRegistered verifies that the blank import causes the opencode
// agent to self-register and that agent.Lookup succeeds for constants.OpenCodeAgentName.
func TestOpenCodeAgentRegistered(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err, "opencode agent must be registered under constants.OpenCodeAgentName")
	require.NotNil(t, a)
	require.Equal(t, constants.OpenCodeAgentName, a.ID())
}

// TestOpenCodeHasCredentialsPresent verifies that HasCredentials returns (true, nil)
// when auth.json exists with content.
func TestOpenCodeHasCredentialsPresent(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	err = os.WriteFile(filepath.Join(tmpDir, "auth.json"), []byte(`{"token":"test"}`), 0o600)
	require.NoError(t, err)

	hasCreds, credErr := a.HasCredentials(tmpDir)
	require.NoError(t, credErr)
	require.True(t, hasCreds)
}

// TestOpenCodeHasCredentialsEmpty verifies that HasCredentials returns (false, nil)
// when the store directory exists but contains no auth.json file.
func TestOpenCodeHasCredentialsEmpty(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	hasCreds, credErr := a.HasCredentials(tmpDir)
	require.NoError(t, credErr)
	require.False(t, hasCreds)
}

// TestOpenCodeHasCredentialsAbsentFile verifies that HasCredentials returns
// (false, nil) when the directory exists but auth.json is not present.
func TestOpenCodeHasCredentialsAbsentFile(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	// Create some other file, not auth.json
	err = os.WriteFile(filepath.Join(tmpDir, "other.json"), []byte(`{}`), 0o600)
	require.NoError(t, err)

	hasCreds, credErr := a.HasCredentials(tmpDir)
	require.NoError(t, credErr)
	require.False(t, hasCreds)
}

// TestOpenCodeHasCredentialsZeroLength verifies that HasCredentials returns
// (false, nil) when auth.json exists but is zero-length.
func TestOpenCodeHasCredentialsZeroLength(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	err = os.WriteFile(filepath.Join(tmpDir, "auth.json"), []byte{}, 0o600)
	require.NoError(t, err)

	hasCreds, credErr := a.HasCredentials(tmpDir)
	require.NoError(t, credErr)
	require.False(t, hasCreds)
}

// TestOpenCodeHasCredentialsStatError verifies that HasCredentials returns
// (false, error) when a filesystem error other than "not exists" occurs.
func TestOpenCodeHasCredentialsStatError(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
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
	require.Contains(t, credErr.Error(), "checking opencode credentials")
}

// TestOpenCodeInstallNodeNotInstalled verifies that when IsNodeInstalled()
// returns false, OpenCode appends Node.js install steps and calls MarkNodeInstalled().
func TestOpenCodeInstallNodeNotInstalled(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	require.False(t, b.IsNodeInstalled(), "fresh builder must have IsNodeInstalled() == false")

	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "setup_22.x",
		"must install Node.js 22 when not already installed")
	require.Contains(t, content, "opencode-ai",
		"must always install the opencode-ai npm package")
	require.True(t, b.IsNodeInstalled(),
		"MarkNodeInstalled() must be called after Node.js installation")
}

// TestOpenCodeInstallNodeAlreadyInstalled verifies that when IsNodeInstalled()
// returns true, OpenCode skips Node.js install steps but still installs its npm package.
func TestOpenCodeInstallNodeAlreadyInstalled(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err)

	b := newTestBuilder()
	b.MarkNodeInstalled() // simulate a prior agent having installed Node.js
	require.True(t, b.IsNodeInstalled())

	a.Install(b)
	content := b.Build()

	// Must NOT contain the Node.js setup step
	require.NotContains(t, content, "setup_22.x",
		"must skip Node.js setup when already installed")

	// Must still install the npm package
	require.Contains(t, content, "opencode-ai",
		"must always install the opencode-ai npm package")
}

// TestOpenCodeContainerMountPath verifies that ContainerMountPath equals
// "<homeDir>/.local/share/opencode".
func TestOpenCodeContainerMountPath(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err)

	want := "/home/testuser/.local/share/opencode"
	require.Equal(t, want, a.ContainerMountPath("/home/testuser"))
}

// TestOpenCodeAdditionalMounts verifies that AdditionalMounts returns exactly
// one mount with the correct ContainerPath and ReadOnly=false.
func TestOpenCodeAdditionalMounts(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err)

	mounter, ok := a.(agent.AdditionalMounter)
	require.True(t, ok, "opencode agent must implement AdditionalMounter")

	mounts := mounter.AdditionalMounts("/home/testuser")
	require.Len(t, mounts, 1)

	mount := mounts[0]
	require.Equal(t, "/home/testuser/.config/opencode", mount.ContainerPath)
	require.False(t, mount.ReadOnly)
	require.True(t, strings.HasSuffix(mount.HostPath, filepath.Join(".config", "opencode")),
		"HostPath must end with .config/opencode, got %q", mount.HostPath)
}

// TestOpenCodeSummaryInfoReturnsNil verifies that the OpenCode agent's SummaryInfo
// method returns (nil, nil) since it has no additional session summary info.
func TestOpenCodeSummaryInfoReturnsNil(t *testing.T) {
	a, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err, "opencode agent must be registered")

	info, summaryErr := a.SummaryInfo(context.Background(), nil, "")
	require.NoError(t, summaryErr)
	require.Nil(t, info)
}

// TestOpenCodeInDefaultAgents verifies that constants.DefaultAgents contains
// "open-code" as part of the expanded five-agent default set.
func TestOpenCodeInDefaultAgents(t *testing.T) {
	require.True(t, strings.Contains(constants.DefaultAgents, constants.OpenCodeAgentName),
		"constants.DefaultAgents must contain %q — it is now a default agent", constants.OpenCodeAgentName)
}
