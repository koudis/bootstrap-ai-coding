package credentials_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/credentials"
)

// Feature: bootstrap-ai-coding, Property 14: Credential store path resolution respects override precedence
func TestPropertyCredentialResolveOverridePrecedence(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		agentDefault := rapid.String().Draw(rt, "agentDefault")
		override := rapid.OneOf(rapid.Just(""), rapid.String()).Draw(rt, "override")

		result := credentials.Resolve(agentDefault, override)

		if override != "" {
			require.Equal(rt, override, result,
				"non-empty override must be returned as-is (agentDefault=%q, override=%q)",
				agentDefault, override)
		} else {
			// override is empty: result must equal the expanded agentDefault
			expected := expandHome(agentDefault)
			require.Equal(rt, expected, result,
				"empty override must return expanded agentDefault (agentDefault=%q)",
				agentDefault)
		}
	})
}

// Feature: bootstrap-ai-coding, Property 15: Credential store directory is always created before mounting
func TestPropertyCredentialEnsureDirCreatesWithCorrectPerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits not applicable on Windows")
	}

	rapid.Check(t, func(rt *rapid.T) {
		base := t.TempDir()
		subdir := rapid.StringMatching(`[a-z][a-z0-9-]*`).Draw(rt, "subdir")
		path := filepath.Join(base, subdir)

		err := credentials.EnsureDir(path)
		require.NoError(rt, err,
			"EnsureDir must not return an error for path %q", path)

		info, statErr := os.Stat(path)
		require.NoError(rt, statErr,
			"directory must exist after EnsureDir (path=%q)", path)
		require.True(rt, info.IsDir(),
			"path must be a directory after EnsureDir (path=%q)", path)
		require.Equal(rt, os.FileMode(constants.ToolDataDirPerm), info.Mode().Perm(),
			"directory must have mode %04o (path=%q)", constants.ToolDataDirPerm, path)
	})
}

// expandHome mirrors the unexported expandHome logic in credentials/store.go
// so Property 14 can compute the expected value independently.
func expandHome(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

// TestResolveExpandsHomeTilde verifies that Resolve expands a "~/" prefix in
// the agentDefault path when no override is provided.
func TestResolveExpandsHomeTilde(t *testing.T) {
	result := credentials.Resolve("~/.myagent", "")
	require.NotContains(t, result, "~/",
		"Resolve must expand ~/ in agentDefault, got %q", result)
	require.Contains(t, result, ".myagent")
}

// TestResolveNoTildePassthrough verifies that a path without "~/" is returned as-is.
func TestResolveNoTildePassthrough(t *testing.T) {
	result := credentials.Resolve("/absolute/path/.myagent", "")
	require.Equal(t, "/absolute/path/.myagent", result)
}

// TestEnsureDirIdempotent verifies that calling EnsureDir twice on the same
// path does not return an error.
func TestEnsureDirIdempotent(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "subdir")

	require.NoError(t, credentials.EnsureDir(path))
	require.NoError(t, credentials.EnsureDir(path), "second call must not error")
}
