package ssh

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
)

// sshConfigPath returns the absolute path to the SSH_Config_File (~/.ssh/config).
func sshConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	rel := strings.TrimPrefix(constants.SSHConfigFile, "~/")
	return filepath.Join(home, rel), nil
}

// sshConfigStanza holds the parsed representation of a single Host stanza.
type sshConfigStanza struct {
	host  string
	lines []string // all lines of the stanza, including the "Host <name>" line
}

// parseSSHConfig reads the SSH config file and returns a slice of stanzas.
// Lines before the first Host directive are collected into a stanza with an
// empty host name (preamble). Returns nil (not an error) if the file does not
// exist.
func parseSSHConfig(path string) ([]sshConfigStanza, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opening SSH config: %w", err)
	}
	defer f.Close()

	var stanzas []sshConfigStanza
	var current *sshConfigStanza

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(strings.ToLower(trimmed), "host ") {
			// Start a new stanza.
			if current != nil {
				stanzas = append(stanzas, *current)
			}
			hostVal := strings.TrimSpace(trimmed[5:]) // everything after "Host "
			current = &sshConfigStanza{host: hostVal, lines: []string{line}}
		} else {
			if current == nil {
				// Preamble (lines before any Host directive).
				current = &sshConfigStanza{host: "", lines: []string{line}}
			} else {
				current.lines = append(current.lines, line)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading SSH config: %w", err)
	}
	if current != nil {
		stanzas = append(stanzas, *current)
	}
	return stanzas, nil
}

// writeSSHConfig serialises stanzas back to the SSH config file.
// The ~/.ssh directory is created with constants.SSHDirPerm if absent.
func writeSSHConfig(path string, stanzas []sshConfigStanza) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, constants.SSHDirPerm); err != nil {
		return fmt.Errorf("creating ~/.ssh directory: %w", err)
	}

	var sb strings.Builder
	for i, s := range stanzas {
		for _, line := range s.lines {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
		// Separate stanzas with a blank line, but not after the last one.
		if i < len(stanzas)-1 && s.host != "" {
			sb.WriteByte('\n')
		}
	}

	if err := os.WriteFile(path, []byte(sb.String()), constants.ToolDataFilePerm); err != nil {
		return fmt.Errorf("writing SSH config: %w", err)
	}
	return nil
}

// buildStanza returns the lines for a new Host stanza for the given container.
func buildStanza(containerName string, port int) sshConfigStanza {
	return sshConfigStanza{
		host: containerName,
		lines: []string{
			fmt.Sprintf("Host %s", containerName),
			fmt.Sprintf("    HostName localhost"),
			fmt.Sprintf("    Port %d", port),
			fmt.Sprintf("    User %s", constants.ContainerUser),
			fmt.Sprintf("    StrictHostKeyChecking yes"),
		},
	}
}

// stanzaMatches reports whether the stanza's fields match the expected values.
func stanzaMatches(s sshConfigStanza, containerName string, port int) bool {
	want := buildStanza(containerName, port)
	if len(s.lines) != len(want.lines) {
		return false
	}
	for i := range s.lines {
		if strings.TrimSpace(s.lines[i]) != strings.TrimSpace(want.lines[i]) {
			return false
		}
	}
	return true
}

// SyncSSHConfig ensures the SSH_Config_File has a correct Host stanza for
// containerName and port.
//
// If noUpdate is true, a notice is printed and the function returns immediately
// without touching the file.
//
// Behaviour:
//   - Absent stanza → append a new one.
//   - Present and matching → no-op.
//   - Present but stale → replace and print confirmation.
//
// Never modifies stanzas whose Host value does not equal containerName.
//
// Validates: Req 19.1–19.9
func SyncSSHConfig(containerName string, port int, noUpdate bool) error {
	if noUpdate {
		fmt.Println("SSH config management is disabled (--no-update-ssh-config).")
		return nil
	}

	cfgPath, err := sshConfigPath()
	if err != nil {
		return err
	}

	stanzas, err := parseSSHConfig(cfgPath)
	if err != nil {
		return err
	}

	// Find the index of the stanza for this container, if any.
	idx := -1
	for i, s := range stanzas {
		if s.host == containerName {
			idx = i
			break
		}
	}

	want := buildStanza(containerName, port)

	switch {
	case idx == -1:
		// Stanza absent — append.
		stanzas = append(stanzas, want)
		return writeSSHConfig(cfgPath, stanzas)

	case stanzaMatches(stanzas[idx], containerName, port):
		// Already correct — no-op.
		return nil

	default:
		// Stale stanza — replace.
		stanzas[idx] = want
		if err := writeSSHConfig(cfgPath, stanzas); err != nil {
			return err
		}
		fmt.Printf("SSH config entry for %s updated.\n", containerName)
		return nil
	}
}

// RemoveSSHConfigEntry removes the Host stanza for containerName from the
// SSH_Config_File. It is a no-op if the file or stanza is absent.
//
// Never modifies stanzas whose Host value does not equal containerName.
//
// Validates: Req 19.7
func RemoveSSHConfigEntry(containerName string) error {
	cfgPath, err := sshConfigPath()
	if err != nil {
		return err
	}

	stanzas, err := parseSSHConfig(cfgPath)
	if err != nil {
		return err
	}
	if stanzas == nil {
		return nil
	}

	var kept []sshConfigStanza
	for _, s := range stanzas {
		if s.host != containerName {
			kept = append(kept, s)
		}
	}

	if len(kept) == len(stanzas) {
		// Nothing removed — no-op.
		return nil
	}

	return writeSSHConfig(cfgPath, kept)
}

// RemoveAllBACSSHConfigEntries removes all Host stanzas whose Host value starts
// with constants.ContainerNamePrefix from the SSH_Config_File. It is a no-op if
// the file is absent.
//
// Validates: Req 19.8
func RemoveAllBACSSHConfigEntries() error {
	cfgPath, err := sshConfigPath()
	if err != nil {
		return err
	}

	stanzas, err := parseSSHConfig(cfgPath)
	if err != nil {
		return err
	}
	if stanzas == nil {
		return nil
	}

	var kept []sshConfigStanza
	for _, s := range stanzas {
		if !strings.HasPrefix(s.host, constants.ContainerNamePrefix) {
			kept = append(kept, s)
		}
	}

	if len(kept) == len(stanzas) {
		return nil
	}

	return writeSSHConfig(cfgPath, kept)
}
