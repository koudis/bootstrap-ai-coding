// Package pathutil provides shared path helpers with zero internal dependencies.
package pathutil

import (
	"os"
	"path/filepath"
)

// ExpandHome expands a leading "~/" to the user's home directory.
// If the path does not start with "~/", it is returned unchanged.
func ExpandHome(p string) string {
	if len(p) >= 2 && p[:2] == "~/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
