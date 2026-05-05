// Package naming derives human-readable, collision-resistant Docker container
// names from project paths. All names are prefixed with constants.ContainerNamePrefix.
package naming

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/pathutil"
)

// consecutiveDashes matches two or more consecutive dashes.
var consecutiveDashes = regexp.MustCompile(`-{2,}`)

// SanitizeNameComponent lowercases s and replaces any character outside
// [a-z0-9.-] with '-', collapses consecutive '-', and trims leading/trailing '-'.
// The '_' character is reserved as the parent/dirname separator and is excluded
// from the allowed set.
func SanitizeNameComponent(s string) string {
	s = strings.ToLower(s)

	// Replace every character that is not [a-z0-9.-] with '-'.
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	s = b.String()

	// Collapse consecutive dashes.
	s = consecutiveDashes.ReplaceAllString(s, "-")

	// Trim leading and trailing dashes.
	s = strings.Trim(s, "-")

	return s
}

// ContainerName returns the first candidate container name not present in
// existingNames. existingNames should contain only bac-prefixed container names
// already on the host (obtained via docker.ListBACContainers).
//
// The algorithm (Req 5.1):
//  1. Resolve projectPath to an absolute path.
//  2. Extract dirname (last component) and parentdir (second-to-last). If the
//     project is at the filesystem root, parentdir is "root".
//  3. Sanitize each component with SanitizeNameComponent.
//  4. Try candidates in order, checking only against bac-prefixed names in
//     existingNames:
//     - Candidate 1: bac-<dirname>
//     - Candidate 2: bac-<parentdir>_<dirname>
//     - Candidate 3+: bac-<parentdir>_<dirname>-2, -3, … until free
//  5. Return the first free candidate.
//
// A candidate is considered "taken" only if it appears in existingNames AND
// does NOT have a corresponding Tool_Data_Dir entry. If a candidate has a
// Tool_Data_Dir entry it was previously assigned to some project — if the
// algorithm reaches it, that project is the current one, so it is returned
// directly (Req 5.1d: the name is persisted via the Tool_Data_Dir).
func ContainerName(projectPath string, existingNames []string) (string, error) {
	abs, err := filepath.Abs(projectPath)
	if err != nil {
		return "", fmt.Errorf("resolving absolute path: %w", err)
	}

	// Build a set of existing bac-prefixed Docker container names for O(1) lookup.
	existing := make(map[string]struct{}, len(existingNames))
	for _, n := range existingNames {
		if strings.HasPrefix(n, constants.ContainerNamePrefix) {
			existing[n] = struct{}{}
		}
	}

	// Build a set of names that already have a Tool_Data_Dir. These names are
	// "claimed" by a known project. If the algorithm produces such a name, it
	// belongs to the current project (or a previous one) and should be returned
	// rather than skipped — the caller will reconcile via the data dir path.
	knownDataDirs := knownDataDirNames()

	// Extract dirname and parentdir.
	dirname := filepath.Base(abs)
	parent := filepath.Dir(abs)
	var parentdir string
	if parent == abs || parent == "/" || parent == "." {
		// Project is at the filesystem root.
		parentdir = "root"
	} else {
		parentdir = filepath.Base(parent)
	}

	// Sanitize both components.
	sanitizedDir := SanitizeNameComponent(dirname)
	sanitizedParent := SanitizeNameComponent(parentdir)

	// isTaken returns true only when a candidate is in use by a Docker container
	// that does NOT have a Tool_Data_Dir (i.e. it belongs to an unrelated project
	// or an externally created container, not to the current project).
	isTaken := func(candidate string) bool {
		if _, inDocker := existing[candidate]; !inDocker {
			return false // not a running/stopped container at all
		}
		if _, hasDataDir := knownDataDirs[candidate]; hasDataDir {
			// The container has a Tool_Data_Dir — it was assigned by this tool.
			// Return it: the caller will use the data dir to confirm ownership.
			return false
		}
		return true // container exists but has no data dir — externally created
	}

	// Candidate 1: bac-<dirname>
	candidate1 := constants.ContainerNamePrefix + sanitizedDir
	if !isTaken(candidate1) {
		return candidate1, nil
	}

	// Candidate 2: bac-<parentdir>_<dirname>
	base := constants.ContainerNamePrefix + sanitizedParent +
		constants.ContainerNameParentSep + sanitizedDir
	if !isTaken(base) {
		return base, nil
	}

	// Candidates 3+: bac-<parentdir>_<dirname>-2, -3, …
	for counter := 2; ; counter++ {
		candidate := base + constants.ContainerNameCounterSep + strconv.Itoa(counter)
		if !isTaken(candidate) {
			return candidate, nil
		}
	}
}

// knownDataDirNames returns a set of container names that have an existing
// Tool_Data_Dir entry under constants.ToolDataDirRoot. These names were
// previously assigned by this tool and must not be treated as collisions.
func knownDataDirNames() map[string]struct{} {
	root := pathutil.ExpandHome(constants.ToolDataDirRoot)
	entries, err := os.ReadDir(root)
	if err != nil {
		return map[string]struct{}{}
	}
	m := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			m[e.Name()] = struct{}{}
		}
	}
	return m
}

