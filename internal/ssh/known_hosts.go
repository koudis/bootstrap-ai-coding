package ssh

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
)

// knownHostsPath returns the absolute path to the Known_Hosts_File (~/.ssh/known_hosts).
func knownHostsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	// Expand constants.KnownHostsFile ("~/.ssh/known_hosts") — strip the "~/" prefix.
	rel := strings.TrimPrefix(constants.KnownHostsFile, "~/")
	return filepath.Join(home, rel), nil
}

// buildKnownHostsLine returns a single Known_Hosts_Entry line for the given
// host pattern prefix, port, and authorized_keys-format public key.
// e.g. buildKnownHostsLine("[localhost]", 2222, "ssh-ed25519 AAAA...")
func buildKnownHostsLine(pattern string, port int, hostPubKey string) string {
	return fmt.Sprintf("%s:%d %s", pattern, port, strings.TrimSpace(hostPubKey))
}

// buildKnownHostsLines returns one Known_Hosts_Entry line per entry in
// constants.KnownHostsPatterns for the given port and public key.
func buildKnownHostsLines(port int, hostPubKey string) []string {
	lines := make([]string, len(constants.KnownHostsPatterns))
	for i, pattern := range constants.KnownHostsPatterns {
		lines[i] = buildKnownHostsLine(pattern, port, hostPubKey)
	}
	return lines
}

// matchesPort reports whether a known_hosts line is a Known_Hosts_Entry for
// the given port under any of the constants.KnownHostsPatterns.
func matchesPort(line string, port int) bool {
	for _, pattern := range constants.KnownHostsPatterns {
		if strings.HasPrefix(line, fmt.Sprintf("%s:%d ", pattern, port)) {
			return true
		}
	}
	return false
}

// readKnownHostsLines reads all lines from the Known_Hosts_File.
// Returns nil (not an error) if the file does not exist.
func readKnownHostsLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening known_hosts: %w", err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading known_hosts: %w", err)
	}
	return lines, nil
}

// writeKnownHostsLines writes lines to the Known_Hosts_File, creating it with
// constants.ToolDataFilePerm if it does not exist. The ~/.ssh directory is
// created with constants.SSHDirPerm if absent.
func writeKnownHostsLines(path string, lines []string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, constants.SSHDirPerm); err != nil {
		return fmt.Errorf("creating ~/.ssh directory: %w", err)
	}

	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}

	if err := os.WriteFile(path, []byte(content), constants.ToolDataFilePerm); err != nil {
		return fmt.Errorf("writing known_hosts: %w", err)
	}
	return nil
}

// appendKnownHostsLines appends lines to the Known_Hosts_File, creating it
// with constants.ToolDataFilePerm (and ~/.ssh with constants.SSHDirPerm) if absent.
func appendKnownHostsLines(path string, newLines []string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, constants.SSHDirPerm); err != nil {
		return fmt.Errorf("creating ~/.ssh directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, constants.ToolDataFilePerm)
	if err != nil {
		return fmt.Errorf("opening known_hosts for append: %w", err)
	}
	defer f.Close()

	for _, line := range newLines {
		if _, err := fmt.Fprintln(f, line); err != nil {
			return fmt.Errorf("writing known_hosts entry: %w", err)
		}
	}
	return nil
}

// SyncKnownHosts ensures the Known_Hosts_File has correct Known_Hosts_Entries
// for the given SSH port and host public key. One entry is written per pattern
// in constants.KnownHostsPatterns.
//
// If noUpdate is true, a notice is printed and the function returns immediately
// without touching the file.
//
// The hostPubKey parameter must be in authorized_keys format (e.g.
// "ssh-ed25519 AAAA...").
func SyncKnownHosts(port int, hostPubKey string, noUpdate bool) error {
	if noUpdate {
		fmt.Println("known_hosts management is disabled (--no-update-known-hosts).")
		return nil
	}

	khPath, err := knownHostsPath()
	if err != nil {
		return err
	}

	wantLines := buildKnownHostsLines(port, hostPubKey)

	existingLines, err := readKnownHostsLines(khPath)
	if err != nil {
		return err
	}

	// Build a map of pattern prefix → existing entry for this port.
	existing := make(map[string]string, len(constants.KnownHostsPatterns))
	for _, line := range existingLines {
		trimmed := strings.TrimSpace(line)
		for _, pattern := range constants.KnownHostsPatterns {
			if strings.HasPrefix(trimmed, fmt.Sprintf("%s:%d ", pattern, port)) {
				existing[pattern] = trimmed
				break
			}
		}
	}

	noneExist := len(existing) == 0
	allMatch := func() bool {
		if len(existing) != len(wantLines) {
			return false
		}
		for i, pattern := range constants.KnownHostsPatterns {
			if existing[pattern] != wantLines[i] {
				return false
			}
		}
		return true
	}()

	switch {
	case noneExist:
		// Append all Known_Hosts_Entries.
		return appendKnownHostsLines(khPath, wantLines)

	case allMatch:
		// No-op: all entries are already correct.
		return nil

	default:
		// One or more entries exist but do not match — prompt the user.
		fmt.Printf("known_hosts entries for port %d do not match the stored host key. Replace them? [y/N] ", port)
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer != "y" {
			fmt.Printf("Warning: known_hosts entries for port %d were not updated. SSH may warn about a changed host key.\n", port)
			return nil
		}

		// Remove all stale entries for this port and append correct ones.
		var kept []string
		for _, line := range existingLines {
			if !matchesPort(strings.TrimSpace(line), port) {
				kept = append(kept, line)
			}
		}
		kept = append(kept, wantLines...)
		if err := writeKnownHostsLines(khPath, kept); err != nil {
			return err
		}
		fmt.Printf("known_hosts entries for port %d updated.\n", port)
		return nil
	}
}

// RemoveKnownHostsEntries removes all Known_Hosts_Entries matching any pattern
// in constants.KnownHostsPatterns for the given port from the Known_Hosts_File.
// It is a no-op if the file does not exist.
func RemoveKnownHostsEntries(port int) error {
	khPath, err := knownHostsPath()
	if err != nil {
		return err
	}

	lines, err := readKnownHostsLines(khPath)
	if err != nil {
		return err
	}
	if lines == nil {
		// File does not exist — nothing to do.
		return nil
	}

	var kept []string
	for _, line := range lines {
		if !matchesPort(strings.TrimSpace(line), port) {
			kept = append(kept, line)
		}
	}

	// Only rewrite if something was actually removed.
	if len(kept) == len(lines) {
		return nil
	}

	return writeKnownHostsLines(khPath, kept)
}
