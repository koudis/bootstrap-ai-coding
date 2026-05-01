package ssh_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	internalssh "github.com/koudis/bootstrap-ai-coding/internal/ssh"
)

// writeKnownHostsFile is a test helper that creates ~/.ssh/known_hosts in the
// given tmpHome directory with the provided lines.
func writeKnownHostsFile(t *testing.T, tmpHome string, lines []string) {
	t.Helper()
	sshDir := filepath.Join(tmpHome, ".ssh")
	require.NoError(t, os.MkdirAll(sshDir, 0o700))
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(content), 0o600))
}

// readKnownHostsFile is a test helper that reads ~/.ssh/known_hosts from tmpHome.
// Returns an empty string if the file does not exist.
func readKnownHostsFile(t *testing.T, tmpHome string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tmpHome, ".ssh", "known_hosts"))
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

// nonEmptyLines returns all non-empty lines from a newline-separated string.
func nonEmptyLines(s string) []string {
	var result []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}

// Feature: bootstrap-ai-coding, Property 36: SyncKnownHosts never modifies unrelated known_hosts entries
func TestSyncKnownHostsPreservesUnrelatedEntries(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		// Draw a port that will be managed by SyncKnownHosts.
		port := rapid.IntRange(2222, 9999).Draw(rt, "port")

		// Draw N unrelated entries (for different hosts, not matching the port patterns).
		n := rapid.IntRange(0, 5).Draw(rt, "numUnrelated")
		unrelated := make([]string, n)
		for i := 0; i < n; i++ {
			// Use github.com-style entries that will never match [localhost]:<port> or 127.0.0.1:<port>.
			unrelated[i] = fmt.Sprintf("github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA%04d unrelated-entry-%d", i, i)
		}

		// Write the unrelated entries to known_hosts.
		writeKnownHostsFile(t, tmpHome, unrelated)

		// Call SyncKnownHosts with a fresh key (file is empty for this port, so no interactive prompt).
		key := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestHostKey"
		err := internalssh.SyncKnownHosts(port, key, false)
		require.NoError(rt, err)

		// Read the file back and assert all N original entries are still present.
		content := readKnownHostsFile(t, tmpHome)
		for _, entry := range unrelated {
			require.Contains(rt, content, entry,
				"SyncKnownHosts removed unrelated entry %q", entry)
		}
	})
}

// Feature: bootstrap-ai-coding, Property 37: RemoveKnownHostsEntries only removes entries for the given port
func TestRemoveKnownHostsEntriesOnlyRemovesTargetPort(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		// Draw 2–3 distinct ports.
		numPorts := rapid.IntRange(2, 3).Draw(rt, "numPorts")
		// Use a base port and spread them out to avoid collisions.
		basePort := rapid.IntRange(2222, 9000).Draw(rt, "basePort")
		ports := make([]int, numPorts)
		for i := 0; i < numPorts; i++ {
			ports[i] = basePort + i*100
		}

		// Build known_hosts entries for each port (both [localhost] and 127.0.0.1 patterns).
		var allLines []string
		for _, p := range ports {
			allLines = append(allLines,
				fmt.Sprintf("[localhost]:%d ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKey%d", p, p),
				fmt.Sprintf("127.0.0.1:%d ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIKey%d", p, p),
			)
		}
		writeKnownHostsFile(t, tmpHome, allLines)

		// Pick the first port as the target to remove.
		targetPort := ports[0]
		err := internalssh.RemoveKnownHostsEntries(targetPort)
		require.NoError(rt, err)

		content := readKnownHostsFile(t, tmpHome)
		lines := nonEmptyLines(content)

		// Assert entries for targetPort are gone.
		for _, line := range lines {
			require.False(rt,
				strings.Contains(line, fmt.Sprintf("[localhost]:%d ", targetPort)) ||
					strings.Contains(line, fmt.Sprintf("127.0.0.1:%d ", targetPort)),
				"entry for target port %d should have been removed, but found: %q", targetPort, line)
		}

		// Assert entries for all other ports are still present.
		for _, p := range ports[1:] {
			localhostEntry := fmt.Sprintf("[localhost]:%d", p)
			loopbackEntry := fmt.Sprintf("127.0.0.1:%d", p)
			require.Contains(rt, content, localhostEntry,
				"entry for non-target port %d ([localhost]) should still be present", p)
			require.Contains(rt, content, loopbackEntry,
				"entry for non-target port %d (127.0.0.1) should still be present", p)
		}
	})
}

// Feature: bootstrap-ai-coding, Property 38: SyncKnownHosts is idempotent when key matches
func TestSyncKnownHostsIdempotent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		port := rapid.IntRange(2222, 9999).Draw(rt, "port")
		key := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIIdempotentTestKey"

		// First call — file does not exist yet.
		err := internalssh.SyncKnownHosts(port, key, false)
		require.NoError(rt, err)

		contentAfterFirst := readKnownHostsFile(t, tmpHome)

		// Second call — entries already exist and match, so it should be a no-op.
		err = internalssh.SyncKnownHosts(port, key, false)
		require.NoError(rt, err)

		contentAfterSecond := readKnownHostsFile(t, tmpHome)

		require.Equal(rt, contentAfterFirst, contentAfterSecond,
			"SyncKnownHosts is not idempotent: file changed between first and second call for port %d", port)
	})
}

// TestSyncKnownHostsNoUpdateSkipsFile verifies that when noUpdate=true,
// SyncKnownHosts does not create or modify the known_hosts file.
// Validates: Req 18.9
func TestSyncKnownHostsNoUpdateSkipsFile(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	err := internalssh.SyncKnownHosts(2222, "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKey", true)
	require.NoError(t, err)

	content := readKnownHostsFile(t, tmpHome)
	require.Empty(t, content, "known_hosts must not be created when noUpdate=true")
}

// TestRemoveKnownHostsEntriesNoopWhenFileAbsent verifies that
// RemoveKnownHostsEntries is a no-op when the known_hosts file does not exist.
func TestRemoveKnownHostsEntriesNoopWhenFileAbsent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// File does not exist — must not error.
	require.NoError(t, internalssh.RemoveKnownHostsEntries(2222))
}

// TestRemoveKnownHostsEntriesNoopWhenPortNotPresent verifies that
// RemoveKnownHostsEntries does not modify the file when the target port
// has no entries in it.
func TestRemoveKnownHostsEntriesNoopWhenPortNotPresent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	existing := []string{
		"github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIUnrelated",
	}
	writeKnownHostsFile(t, tmpHome, existing)
	before := readKnownHostsFile(t, tmpHome)

	require.NoError(t, internalssh.RemoveKnownHostsEntries(9999))

	after := readKnownHostsFile(t, tmpHome)
	require.Equal(t, before, after, "file must not change when port has no entries")
}

// TestSyncKnownHostsAppendsNewEntries verifies that SyncKnownHosts appends
// both [localhost] and 127.0.0.1 entries when the file is empty.
func TestSyncKnownHostsAppendsNewEntries(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	const port = 2222
	const key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAINewKey"

	require.NoError(t, internalssh.SyncKnownHosts(port, key, false))

	content := readKnownHostsFile(t, tmpHome)
	require.Contains(t, content, fmt.Sprintf("[localhost]:%d", port))
	require.Contains(t, content, fmt.Sprintf("127.0.0.1:%d", port))
	require.Contains(t, content, "ssh-ed25519")
}
