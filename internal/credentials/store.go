// Package credentials handles per-agent credential store path resolution
// and directory creation. It is agent-agnostic.
package credentials

import (
	"os"
	"path/filepath"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
)

// Resolve returns the effective credential store path for an agent.
// If override is non-empty it takes precedence over the agent's default.
func Resolve(agentDefault, override string) string {
	if override != "" {
		return override
	}
	return expandHome(agentDefault)
}

// EnsureDir creates the directory at path (and all parents) if it does not
// already exist, using constants.ToolDataDirPerm.
func EnsureDir(path string) error {
	return os.MkdirAll(path, constants.ToolDataDirPerm)
}

func expandHome(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
