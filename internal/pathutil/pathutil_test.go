package pathutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/pathutil"
)

// Feature: bootstrap-ai-coding, Property 26: ExpandHome never returns a path starting with ~/
func TestPropertyExpandHomeNeverReturnsTilde(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		input := rapid.String().Draw(t, "input")
		result := pathutil.ExpandHome(input)
		require.False(t,
			len(result) >= 2 && result[:2] == "~/",
			"ExpandHome(%q) = %q must not start with ~/", input, result)
	})
}

func TestExpandHomeNoTilde(t *testing.T) {
	require.Equal(t, "/absolute/path", pathutil.ExpandHome("/absolute/path"))
}

func TestExpandHomeRelativePath(t *testing.T) {
	require.Equal(t, "relative/path", pathutil.ExpandHome("relative/path"))
}

func TestExpandHomeTildeExpanded(t *testing.T) {
	result := pathutil.ExpandHome("~/somedir")
	require.NotContains(t, result, "~", "ExpandHome must expand ~ to the home directory")
	require.Contains(t, result, "somedir")
}

func TestExpandHomeTildeOnly(t *testing.T) {
	// "~" alone (no slash) should not be expanded.
	require.Equal(t, "~", pathutil.ExpandHome("~"))
}

func TestExpandHomeEmptyString(t *testing.T) {
	require.Equal(t, "", pathutil.ExpandHome(""))
}

func TestExpandHomeTildeSlashOnly(t *testing.T) {
	home, _ := os.UserHomeDir()
	result := pathutil.ExpandHome("~/")
	// filepath.Join(home, "") returns home without trailing slash.
	require.Equal(t, filepath.Join(home, ""), result)
}

func TestExpandHomeNestedPath(t *testing.T) {
	home, _ := os.UserHomeDir()
	result := pathutil.ExpandHome("~/.config/bootstrap-ai-coding")
	require.Equal(t, filepath.Join(home, ".config/bootstrap-ai-coding"), result)
}
