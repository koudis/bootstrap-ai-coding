package ssh_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	internalssh "github.com/koudis/bootstrap-ai-coding/internal/ssh"
)

// sshConfigStanzaText returns a minimal SSH config stanza string for the given
// host name and port, matching the format produced by buildStanza in ssh_config.go.
func sshConfigStanzaText(name string, port int) string {
	return fmt.Sprintf("Host %s\n    HostName localhost\n    Port %d\n    User %s\n    StrictHostKeyChecking yes\n",
		name, port, constants.ContainerUser)
}

// writeSSHConfigFile creates ~/.ssh/config in tmpHome with the provided content.
func writeSSHConfigFile(t *testing.T, tmpHome string, content string) {
	t.Helper()
	sshDir := filepath.Join(tmpHome, ".ssh")
	require.NoError(t, os.MkdirAll(sshDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(sshDir, "config"), []byte(content), 0o600))
}

// readSSHConfigFile reads ~/.ssh/config from tmpHome.
// Returns an empty string if the file does not exist.
func readSSHConfigFile(t *testing.T, tmpHome string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(tmpHome, ".ssh", "config"))
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return string(data)
}

// Feature: bootstrap-ai-coding, Property 39: SyncSSHConfig never modifies unrelated SSH config entries
func TestSyncSSHConfigPreservesUnrelatedEntries(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		// Draw N unrelated stanzas whose Host values do NOT start with "bac-".
		n := rapid.IntRange(0, 3).Draw(rt, "numUnrelated")

		// Fixed set of non-bac host names to draw from.
		nonBacNames := []string{"myserver", "work-laptop", "staging-host"}

		var initialContent strings.Builder
		usedNames := make([]string, 0, n)
		for i := 0; i < n; i++ {
			name := nonBacNames[i%len(nonBacNames)] + fmt.Sprintf("-%d", i)
			port := 22 + i
			initialContent.WriteString(sshConfigStanzaText(name, port))
			if i < n-1 {
				initialContent.WriteByte('\n')
			}
			usedNames = append(usedNames, name)
		}

		if n > 0 {
			writeSSHConfigFile(t, tmpHome, initialContent.String())
		}

		// Call SyncSSHConfig with a bac- prefixed container name.
		err := internalssh.SyncSSHConfig("bac-testproject", 2222, false)
		require.NoError(rt, err)

		// Read the file back and assert all N original stanzas are still present.
		content := readSSHConfigFile(t, tmpHome)
		for _, name := range usedNames {
			require.Contains(rt, content, fmt.Sprintf("Host %s", name),
				"SyncSSHConfig removed unrelated stanza for host %q", name)
		}
	})
}

// Feature: bootstrap-ai-coding, Property 40: RemoveSSHConfigEntry only removes the entry for the given container name
func TestRemoveSSHConfigEntryOnlyRemovesTarget(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		// Draw 2–3 distinct container names.
		numContainers := rapid.IntRange(2, 3).Draw(rt, "numContainers")

		// Build distinct names using an index suffix to avoid collisions.
		baseIdx := rapid.IntRange(0, 100).Draw(rt, "baseIdx")
		names := make([]string, numContainers)
		for i := 0; i < numContainers; i++ {
			names[i] = fmt.Sprintf("bac-container-%d", baseIdx+i)
		}

		// Write SSH config entries for each container.
		var sb strings.Builder
		for i, name := range names {
			sb.WriteString(sshConfigStanzaText(name, 2222+i))
			if i < numContainers-1 {
				sb.WriteByte('\n')
			}
		}
		writeSSHConfigFile(t, tmpHome, sb.String())

		// Remove the first container's entry.
		targetName := names[0]
		err := internalssh.RemoveSSHConfigEntry(targetName)
		require.NoError(rt, err)

		content := readSSHConfigFile(t, tmpHome)

		// Assert the target stanza is gone.
		require.NotContains(rt, content, fmt.Sprintf("Host %s", targetName),
			"RemoveSSHConfigEntry did not remove the target stanza for %q", targetName)

		// Assert all other stanzas are still present.
		for _, name := range names[1:] {
			require.Contains(rt, content, fmt.Sprintf("Host %s", name),
				"RemoveSSHConfigEntry removed non-target stanza for %q", name)
		}
	})
}

// Feature: bootstrap-ai-coding, Property 41: SyncSSHConfig is idempotent when entry matches
func TestSyncSSHConfigIdempotent(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		name := fmt.Sprintf("bac-idempotent-%d", rapid.IntRange(0, 999).Draw(rt, "idx"))
		port := rapid.IntRange(1024, 65535).Draw(rt, "port")

		// First call — file may not exist yet.
		err := internalssh.SyncSSHConfig(name, port, false)
		require.NoError(rt, err)

		contentAfterFirst := readSSHConfigFile(t, tmpHome)

		// Second call — entry already exists and matches, so it should be a no-op.
		err = internalssh.SyncSSHConfig(name, port, false)
		require.NoError(rt, err)

		contentAfterSecond := readSSHConfigFile(t, tmpHome)

		require.Equal(rt, contentAfterFirst, contentAfterSecond,
			"SyncSSHConfig is not idempotent: file changed between first and second call for %s:%d", name, port)
	})
}

// Feature: bootstrap-ai-coding, Property 42: RemoveAllBACSSHConfigEntries only removes bac- prefixed entries
func TestRemoveAllBACSSHConfigEntriesOnlyRemovesBacEntries(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		// Draw some bac- prefixed names and some non-bac names.
		numBac := rapid.IntRange(1, 3).Draw(rt, "numBac")
		numNonBac := rapid.IntRange(1, 3).Draw(rt, "numNonBac")

		bacNames := make([]string, numBac)
		for i := 0; i < numBac; i++ {
			bacNames[i] = fmt.Sprintf("%sproject-%d", constants.ContainerNamePrefix, i)
		}

		nonBacNames := make([]string, numNonBac)
		for i := 0; i < numNonBac; i++ {
			nonBacNames[i] = fmt.Sprintf("myserver-%d", i)
		}

		// Write SSH config entries for all of them.
		var sb strings.Builder
		allNames := append(bacNames, nonBacNames...)
		for i, name := range allNames {
			sb.WriteString(sshConfigStanzaText(name, 2222+i))
			if i < len(allNames)-1 {
				sb.WriteByte('\n')
			}
		}
		writeSSHConfigFile(t, tmpHome, sb.String())

		// Remove all bac- entries.
		err := internalssh.RemoveAllBACSSHConfigEntries()
		require.NoError(rt, err)

		content := readSSHConfigFile(t, tmpHome)

		// Assert all bac- entries are gone.
		for _, name := range bacNames {
			require.NotContains(rt, content, fmt.Sprintf("Host %s", name),
				"RemoveAllBACSSHConfigEntries did not remove bac- entry for %q", name)
		}

		// Assert all non-bac entries are still present.
		for _, name := range nonBacNames {
			require.Contains(rt, content, fmt.Sprintf("Host %s", name),
				"RemoveAllBACSSHConfigEntries removed non-bac entry for %q", name)
		}
	})
}

// ── Unit tests for ssh_config scenarios ──────────────────────────────────────

// TestSSHConfigEntryAddedOnStart verifies that when no stanza exists for the
// container, SyncSSHConfig appends a correctly-formed stanza.
// Validates: Req 19.1
func TestSSHConfigEntryAddedOnStart(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	const name = "bac-myproject"
	const port = 2222

	require.NoError(t, internalssh.SyncSSHConfig(name, port, false))

	content := readSSHConfigFile(t, tmpHome)
	require.Contains(t, content, fmt.Sprintf("Host %s", name))
	require.Contains(t, content, fmt.Sprintf("Port %d", port))
	require.Contains(t, content, fmt.Sprintf("User %s", constants.ContainerUser))
	require.Contains(t, content, "HostName localhost")
	require.Contains(t, content, "StrictHostKeyChecking yes")
}

// TestSSHConfigNoChangeWhenEntryMatches verifies that when a matching stanza
// already exists, SyncSSHConfig leaves the file byte-for-byte unchanged.
// Validates: Req 19.2
func TestSSHConfigNoChangeWhenEntryMatches(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	const name = "bac-myproject"
	const port = 2222

	// Write a pre-existing matching stanza.
	writeSSHConfigFile(t, tmpHome, sshConfigStanzaText(name, port))
	before := readSSHConfigFile(t, tmpHome)

	require.NoError(t, internalssh.SyncSSHConfig(name, port, false))

	after := readSSHConfigFile(t, tmpHome)
	require.Equal(t, before, after, "file must not change when entry already matches")
}

// TestSSHConfigStaleEntryReplaced verifies that when a stanza exists but has a
// different port, SyncSSHConfig replaces it with the correct values.
// Validates: Req 19.3
//
// NOTE: The production code replaces stale entries directly (no interactive
// prompt in the current implementation). This test exercises that path.
func TestSSHConfigStaleEntryReplaced(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	const name = "bac-myproject"
	const oldPort = 2222
	const newPort = 2223

	// Write a stanza with the old port.
	writeSSHConfigFile(t, tmpHome, sshConfigStanzaText(name, oldPort))

	require.NoError(t, internalssh.SyncSSHConfig(name, newPort, false))

	content := readSSHConfigFile(t, tmpHome)
	require.Contains(t, content, fmt.Sprintf("Port %d", newPort), "stale port must be replaced with new port")
	require.NotContains(t, content, fmt.Sprintf("Port %d", oldPort), "old port must no longer appear")
	// Only one Host stanza for this container should exist.
	require.Equal(t, 1, strings.Count(content, fmt.Sprintf("Host %s", name)),
		"exactly one stanza for the container must remain after replacement")
}

// TestSSHConfigEntryRemovedOnStopAndRemove verifies that RemoveSSHConfigEntry
// removes the stanza for the given container and leaves the file without it.
// Validates: Req 19.7
func TestSSHConfigEntryRemovedOnStopAndRemove(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	const name = "bac-myproject"
	const port = 2222

	writeSSHConfigFile(t, tmpHome, sshConfigStanzaText(name, port))

	require.NoError(t, internalssh.RemoveSSHConfigEntry(name))

	content := readSSHConfigFile(t, tmpHome)
	require.NotContains(t, content, fmt.Sprintf("Host %s", name),
		"stanza must be absent after RemoveSSHConfigEntry")
}

// TestSSHConfigSkippedWithNoUpdateFlag verifies that when noUpdate is true,
// SyncSSHConfig prints a notice and does not touch the SSH config file.
// Validates: Req 19.9
func TestSSHConfigSkippedWithNoUpdateFlag(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	const name = "bac-myproject"
	const port = 2222

	// File does not exist yet — noUpdate should prevent creation.
	require.NoError(t, internalssh.SyncSSHConfig(name, port, true))

	content := readSSHConfigFile(t, tmpHome)
	require.Empty(t, content, "SSH config file must not be created when noUpdate=true")
}


// TestRemoveSSHConfigEntryNoopWhenFileAbsent verifies that RemoveSSHConfigEntry
// is a no-op when the SSH config file does not exist.
func TestRemoveSSHConfigEntryNoopWhenFileAbsent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	require.NoError(t, internalssh.RemoveSSHConfigEntry("bac-myproject"))
}

// TestRemoveSSHConfigEntryNoopWhenStanzaAbsent verifies that RemoveSSHConfigEntry
// does not modify the file when the target stanza is not present.
func TestRemoveSSHConfigEntryNoopWhenStanzaAbsent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	writeSSHConfigFile(t, tmpHome, sshConfigStanzaText("bac-other", 2222))
	before := readSSHConfigFile(t, tmpHome)

	require.NoError(t, internalssh.RemoveSSHConfigEntry("bac-notpresent"))

	after := readSSHConfigFile(t, tmpHome)
	require.Equal(t, before, after, "file must not change when stanza is absent")
}

// TestRemoveAllBACSSHConfigEntriesNoopWhenFileAbsent verifies that
// RemoveAllBACSSHConfigEntries is a no-op when the SSH config file does not exist.
func TestRemoveAllBACSSHConfigEntriesNoopWhenFileAbsent(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	require.NoError(t, internalssh.RemoveAllBACSSHConfigEntries())
}

// TestRemoveAllBACSSHConfigEntriesNoopWhenNoBacEntries verifies that
// RemoveAllBACSSHConfigEntries does not modify the file when no bac- entries exist.
func TestRemoveAllBACSSHConfigEntriesNoopWhenNoBacEntries(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	writeSSHConfigFile(t, tmpHome, sshConfigStanzaText("myserver", 22))
	before := readSSHConfigFile(t, tmpHome)

	require.NoError(t, internalssh.RemoveAllBACSSHConfigEntries())

	after := readSSHConfigFile(t, tmpHome)
	require.Equal(t, before, after, "file must not change when no bac- entries exist")
}
