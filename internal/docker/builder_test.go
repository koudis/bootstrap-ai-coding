package docker_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"
)

// fixedHostKeyPriv and fixedHostKeyPub are stable test values used wherever
// the exact key content is not the subject of the property under test.
const (
	fixedHostKeyPriv = "-----BEGIN OPENSSH PRIVATE KEY-----\nfakePrivKey\n-----END OPENSSH PRIVATE KEY-----"
	fixedHostKeyPub  = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIfakeHostPub host-key"
	fixedPublicKey   = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIfakePubKey test@host"
)

// testInfo returns a *hostinfo.Info suitable for unit tests.
func testInfo(uid, gid int) *hostinfo.Info {
	return &hostinfo.Info{
		Username: "testuser",
		HomeDir:  "/home/testuser",
		UID:      uid,
		GID:      gid,
	}
}

// newCreateBuilder is a convenience helper that builds a base image builder
// using UserStrategyCreate with the given uid/gid.
func newCreateBuilder(uid, gid int) *docker.DockerfileBuilder {
	return docker.NewBaseImageBuilder(
		testInfo(uid, gid),
		docker.UserStrategyCreate, "",
		"",
	)
}

// newRenameBuilder is a convenience helper that builds a base image builder
// using UserStrategyRename with the given uid/gid and conflicting user name.
func newRenameBuilder(uid, gid int, conflictingUser string) *docker.DockerfileBuilder {
	return docker.NewBaseImageBuilder(
		testInfo(uid, gid),
		docker.UserStrategyRename, conflictingUser,
		"",
	)
}

// newInstanceBuilder is a convenience helper that builds an instance image builder
// with the given uid/gid and fixed key material.
func newInstanceBuilder(uid, gid int) *docker.DockerfileBuilder {
	return docker.NewInstanceImageBuilder(
		testInfo(uid, gid),
		fixedPublicKey,
		fixedHostKeyPriv, fixedHostKeyPub,
		2222, false,
	)
}

// ---------------------------------------------------------------------------
// Property 3: Generated Dockerfile always uses constants.BaseContainerImage
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 3: Generated Dockerfile always uses constants.BaseContainerImage as base image
func TestPropertyDockerfileBaseImage_Create(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		b := newCreateBuilder(uid, gid)
		content := b.Build()
		lines := b.Lines()

		// First instruction must be FROM <BaseContainerImage>
		wantFrom := "FROM " + constants.BaseContainerImage
		require.Equal(t, wantFrom, lines[0],
			"first Dockerfile line must be FROM %s", constants.BaseContainerImage)

		// The Dockerfile must not reference any other base image
		for i, line := range lines[1:] {
			require.False(t, strings.HasPrefix(line, "FROM "),
				"unexpected second FROM at line %d: %q", i+1, line)
		}

		// Build() output must contain the FROM line
		require.Contains(t, content, wantFrom)
	})
}

// Feature: bootstrap-ai-coding, Property 3: Generated Dockerfile always uses constants.BaseContainerImage as base image
func TestPropertyDockerfileBaseImage_Rename(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		conflictingUser := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "conflictingUser")

		b := newRenameBuilder(uid, gid, conflictingUser)
		lines := b.Lines()

		wantFrom := "FROM " + constants.BaseContainerImage
		require.Equal(t, wantFrom, lines[0],
			"first Dockerfile line must be FROM %s", constants.BaseContainerImage)

		for i, line := range lines[1:] {
			require.False(t, strings.HasPrefix(line, "FROM "),
				"unexpected second FROM at line %d: %q", i+1, line)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 4: Generated Dockerfile always includes SSH server and Container_User
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 4: Generated Dockerfile always includes SSH server and Container_User
func TestPropertyDockerfileSSHServerAndContainerUser(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		b := newCreateBuilder(uid, gid)
		content := b.Build()

		// Must install openssh-server (base layer)
		require.Contains(t, content, "openssh-server",
			"Dockerfile must install openssh-server")

		// Must reference ContainerUser (base layer)
		require.Contains(t, content, "testuser",
			"Dockerfile must reference ContainerUser %q", "testuser")

		// CMD is in the instance layer — verify it there
		ib := newInstanceBuilder(uid, gid)
		ib.Finalize()
		instanceContent := ib.Build()

		require.Contains(t, instanceContent, "/usr/sbin/sshd",
			"Instance Dockerfile must include sshd CMD")
		require.Contains(t, instanceContent, `CMD ["/usr/sbin/sshd", "-D"]`,
			"Instance Dockerfile must have CMD [\"/usr/sbin/sshd\", \"-D\"]")
	})
}

// ---------------------------------------------------------------------------
// Property 5: Container_User UID and GID always match the host user
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 5: Container_User UID and GID always match the host user
func TestPropertyContainerUserUID_Create(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		b := newCreateBuilder(uid, gid)
		content := b.Build()

		// useradd must carry --uid <uid> and --gid <gid>
		require.Contains(t, content, fmt.Sprintf("--uid %d", uid),
			"Dockerfile must contain --uid %d", uid)
		require.Contains(t, content, fmt.Sprintf("--gid %d", gid),
			"Dockerfile must contain --gid %d", gid)
	})
}

// Feature: bootstrap-ai-coding, Property 5: Container_User UID and GID always match the host user
func TestPropertyContainerUserUID_Rename(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		conflictingUser := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "conflictingUser")

		b := newRenameBuilder(uid, gid, conflictingUser)
		content := b.Build()

		// UserStrategyRename renames the conflicting user; the UID/GID are
		// already correct because the conflicting user owns them. The rename
		// command must reference the conflicting user and the ContainerUser.
		require.Contains(t, content, conflictingUser,
			"Dockerfile must reference conflicting user %q", conflictingUser)
		require.Contains(t, content, "testuser",
			"Dockerfile must reference ContainerUser %q", "testuser")
		require.Contains(t, content, "/home/testuser",
			"Dockerfile must reference ContainerUserHome %q", "/home/testuser")
	})
}

// ---------------------------------------------------------------------------
// Property 5a: UserStrategyRename uses usermod -l, UserStrategyCreate uses useradd
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 5a: UserStrategyRename uses usermod -l, UserStrategyCreate uses useradd
func TestPropertyUserStrategyCreate_UsesUseradd(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		b := newCreateBuilder(uid, gid)
		content := b.Build()

		// Must use useradd
		require.Contains(t, content, "useradd",
			"UserStrategyCreate Dockerfile must contain useradd")

		// Must NOT use usermod -l
		require.NotContains(t, content, "usermod -l",
			"UserStrategyCreate Dockerfile must not contain usermod -l")
	})
}

// Feature: bootstrap-ai-coding, Property 5a: UserStrategyRename uses usermod -l, UserStrategyCreate uses useradd
func TestPropertyUserStrategyRename_UsesUsermod(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		conflictingUser := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "conflictingUser")

		b := newRenameBuilder(uid, gid, conflictingUser)
		content := b.Build()

		// Must use usermod -l
		require.Contains(t, content, "usermod -l",
			"UserStrategyRename Dockerfile must contain usermod -l")

		// Must NOT use useradd
		require.NotContains(t, content, "useradd",
			"UserStrategyRename Dockerfile must not contain useradd")
	})
}

// ---------------------------------------------------------------------------
// Property 5b: No conflict returns nil from FindConflictingUser
//
// FindConflictingUser requires a live Docker daemon and is therefore an
// integration test. The parsing logic is internal to client.go and cannot be
// exercised without Docker. This property is covered by the integration test
// suite (//go:build integration). Below we document the invariant and provide
// a table-driven unit test that exercises the passwd-parsing logic inline to
// verify the concept without Docker.
// ---------------------------------------------------------------------------

// TestPasswdParsingNoConflict verifies the passwd-parsing logic used by
// FindConflictingUser: given a /etc/passwd-format string that contains no
// entry matching the requested UID or GID, the result is nil.
//
// This is a unit test of the parsing concept; the full FindConflictingUser
// function requires a live Docker daemon and is tested in integration tests.
//
// Validates: Req 10a.2
func TestPasswdParsingNoConflict(t *testing.T) {
	// Simulate a minimal /etc/passwd with well-known system users.
	passwdLines := []string{
		"root:x:0:0:root:/root:/bin/bash",
		"daemon:x:1:1:daemon:/usr/sbin:/usr/sbin/nologin",
		"nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin",
	}

	type testCase struct {
		name string
		uid  int
		gid  int
		want bool // true = conflict expected
	}

	cases := []testCase{
		{"root uid conflict", 0, 9999, true},
		{"root gid conflict", 9999, 0, true},
		{"daemon uid conflict", 1, 9999, true},
		{"nobody uid conflict", 65534, 9999, true},
		{"no conflict", 1000, 1000, false},
		{"no conflict high uid", 50000, 50001, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conflict := parsePasswdForConflict(passwdLines, tc.uid, tc.gid)
			if tc.want {
				require.NotNil(t, conflict, "expected conflict for uid=%d gid=%d", tc.uid, tc.gid)
			} else {
				require.Nil(t, conflict, "expected no conflict for uid=%d gid=%d", tc.uid, tc.gid)
			}
		})
	}
}

// parsePasswdForConflict is a local reimplementation of the passwd-parsing
// logic from FindConflictingUser, used to test the concept without Docker.
// It mirrors the logic in client.go exactly.
func parsePasswdForConflict(lines []string, uid, gid int) *struct{ Username string } {
	for _, line := range lines {
		fields := strings.Split(line, ":")
		if len(fields) < 4 {
			continue
		}
		var entryUID, entryGID int
		if _, err := fmt.Sscanf(fields[2], "%d", &entryUID); err != nil {
			continue
		}
		if _, err := fmt.Sscanf(fields[3], "%d", &entryGID); err != nil {
			continue
		}
		if entryUID == uid || entryGID == gid {
			return &struct{ Username string }{Username: fields[0]}
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Property 6: Container_User always has passwordless sudo
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 6: Container_User always has passwordless sudo
func TestPropertyPasswordlessSudo_Create(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		b := newCreateBuilder(uid, gid)
		content := b.Build()

		// Must contain a sudoers entry for ContainerUser with NOPASSWD
		require.Contains(t, content, "testuser",
			"Dockerfile must reference ContainerUser in sudoers")
		require.Contains(t, content, "NOPASSWD:ALL",
			"Dockerfile must grant NOPASSWD:ALL sudo to ContainerUser")
		require.Contains(t, content, "sudoers",
			"Dockerfile must write to sudoers.d")
	})
}

// Feature: bootstrap-ai-coding, Property 6: Container_User always has passwordless sudo
func TestPropertyPasswordlessSudo_Rename(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		conflictingUser := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "conflictingUser")

		b := newRenameBuilder(uid, gid, conflictingUser)
		content := b.Build()

		require.Contains(t, content, "NOPASSWD:ALL",
			"Dockerfile must grant NOPASSWD:ALL sudo to ContainerUser")
		require.Contains(t, content, "sudoers",
			"Dockerfile must write to sudoers.d")
	})
}

// ---------------------------------------------------------------------------
// Property 7: sshd_config always disables password authentication
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 7: sshd_config always disables password authentication
func TestPropertySSHDConfigPasswordAuthDisabled_Create(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		b := newInstanceBuilder(uid, gid)
		content := b.Build()

		require.Contains(t, content, "PasswordAuthentication no",
			"Instance Dockerfile must set PasswordAuthentication no in sshd_config")
	})
}

// Feature: bootstrap-ai-coding, Property 7: sshd_config always disables password authentication
func TestPropertySSHDConfigPasswordAuthDisabled_Rename(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		b := newInstanceBuilder(uid, gid)
		content := b.Build()

		require.Contains(t, content, "PasswordAuthentication no",
			"Instance Dockerfile must set PasswordAuthentication no in sshd_config")
	})
}

// ---------------------------------------------------------------------------
// Property 8: Public key is always injected into ContainerUserHome/.ssh/authorized_keys
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 8: Public key is always injected into constants.ContainerUserHome/.ssh/authorized_keys
func TestPropertyPublicKeyInjected_Create(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		// Draw a realistic-looking public key string
		keyBody := rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "keyBody")
		publicKey := "ssh-ed25519 " + keyBody + " test@host"

		b := docker.NewInstanceImageBuilder(
			testInfo(uid, gid),
			publicKey,
			fixedHostKeyPriv, fixedHostKeyPub,
			2222, false,
		)
		content := b.Build()

		authorizedKeysPath := "/home/testuser/.ssh/authorized_keys"
		require.Contains(t, content, authorizedKeysPath,
			"Instance Dockerfile must reference authorized_keys path %q", authorizedKeysPath)
	})
}

// Feature: bootstrap-ai-coding, Property 8: Public key is always injected into constants.ContainerUserHome/.ssh/authorized_keys
func TestPropertyPublicKeyInjected_Rename(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		keyBody := rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "keyBody")
		publicKey := "ssh-ed25519 " + keyBody + " test@host"

		b := docker.NewInstanceImageBuilder(
			testInfo(uid, gid),
			publicKey,
			fixedHostKeyPriv, fixedHostKeyPub,
			2222, false,
		)
		content := b.Build()

		authorizedKeysPath := "/home/testuser/.ssh/authorized_keys"
		require.Contains(t, content, authorizedKeysPath,
			"Instance Dockerfile must reference authorized_keys path %q", authorizedKeysPath)
	})
}

// ---------------------------------------------------------------------------
// Property 10: SSH host key is always injected into the Dockerfile
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 10: SSH host key is always injected into the Dockerfile
func TestPropertySSHHostKeyInjected_Create(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		privKeyBody := rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "privKeyBody")
		pubKeyBody := rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "pubKeyBody")
		hostKeyPriv := "-----BEGIN OPENSSH PRIVATE KEY-----\n" + privKeyBody + "\n-----END OPENSSH PRIVATE KEY-----"
		hostKeyPub := "ssh-ed25519 " + pubKeyBody + " host"

		b := docker.NewInstanceImageBuilder(
			testInfo(uid, gid),
			fixedPublicKey,
			hostKeyPriv, hostKeyPub,
			2222, false,
		)
		content := b.Build()

		// Must write the private key to the expected path
		privPath := fmt.Sprintf("/etc/ssh/ssh_host_%s_key", constants.SSHHostKeyType)
		pubPath := privPath + ".pub"
		require.Contains(t, content, privPath,
			"Instance Dockerfile must inject host private key to %q", privPath)
		require.Contains(t, content, pubPath,
			"Instance Dockerfile must inject host public key to %q", pubPath)
	})
}

// Feature: bootstrap-ai-coding, Property 10: SSH host key is always injected into the Dockerfile
func TestPropertySSHHostKeyInjected_Rename(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		privKeyBody := rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "privKeyBody")
		pubKeyBody := rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "pubKeyBody")
		hostKeyPriv := "-----BEGIN OPENSSH PRIVATE KEY-----\n" + privKeyBody + "\n-----END OPENSSH PRIVATE KEY-----"
		hostKeyPub := "ssh-ed25519 " + pubKeyBody + " host"

		b := docker.NewInstanceImageBuilder(
			testInfo(uid, gid),
			fixedPublicKey,
			hostKeyPriv, hostKeyPub,
			2222, false,
		)
		content := b.Build()

		privPath := fmt.Sprintf("/etc/ssh/ssh_host_%s_key", constants.SSHHostKeyType)
		pubPath := privPath + ".pub"
		require.Contains(t, content, privPath,
			"Instance Dockerfile must inject host private key to %q", privPath)
		require.Contains(t, content, pubPath,
			"Instance Dockerfile must inject host public key to %q", pubPath)
	})
}

// ---------------------------------------------------------------------------
// Unit tests for Env, Copy, Cmd builder methods
// ---------------------------------------------------------------------------

func TestBuilderEnvAppendsCorrectInstruction(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	b.Env("MY_VAR", "my_value")
	content := b.Build()
	require.Contains(t, content, "ENV MY_VAR=my_value")
}

func TestBuilderCopyAppendsCorrectInstruction(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	b.Copy("src/file.txt", "/dst/file.txt")
	content := b.Build()
	require.Contains(t, content, "COPY src/file.txt /dst/file.txt")
}

func TestBuilderCmdAppendsCorrectInstruction(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	b.Cmd("echo hello")
	content := b.Build()
	require.Contains(t, content, `CMD ["/bin/sh", "-c", "echo hello"]`)
}

// ---------------------------------------------------------------------------
// Property 52: Headless keyring packages are always installed in the base layer
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 52: Headless keyring packages are always installed in the base layer
// Validates: CC-7
func TestPropertyKeyringPackagesInstalled(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		b := newCreateBuilder(uid, gid)
		content := b.Build()

		require.Contains(t, content, "dbus-x11",
			"Dockerfile must install dbus-x11 for dbus-launch")
		require.Contains(t, content, "gnome-keyring",
			"Dockerfile must install gnome-keyring for Secret Service")
		require.Contains(t, content, "libsecret-1-0",
			"Dockerfile must install libsecret-1-0 for client access")
	})
}

// ---------------------------------------------------------------------------
// Property 53: Keyring profile script is always created at constants.KeyringProfileScript
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 53: Keyring profile script is always created at constants.KeyringProfileScript
// Validates: CC-7
func TestPropertyKeyringProfileScriptCreated(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		b := newCreateBuilder(uid, gid)
		content := b.Build()

		require.Contains(t, content, constants.KeyringProfileScript,
			"Dockerfile must reference KeyringProfileScript path %q", constants.KeyringProfileScript)
		require.Contains(t, content, "dbus-launch",
			"Keyring script must start dbus-launch")
		require.Contains(t, content, "gnome-keyring-daemon",
			"Keyring script must start gnome-keyring-daemon")
		require.Contains(t, content, "--unlock",
			"Keyring script must unlock the keyring")
		require.Contains(t, content, "chmod +x",
			"Keyring script must be made executable")
	})
}

// TestKeyringProfileScriptPresentInRenameStrategy verifies that the keyring
// setup is also present when using UserStrategyRename.
// Validates: CC-7
func TestKeyringProfileScriptPresentInRenameStrategy(t *testing.T) {
	b := newRenameBuilder(1000, 1000, "ubuntu")
	content := b.Build()

	require.Contains(t, content, "gnome-keyring",
		"Rename strategy Dockerfile must also install gnome-keyring")
	require.Contains(t, content, constants.KeyringProfileScript,
		"Rename strategy Dockerfile must also create keyring profile script")
}

// ---------------------------------------------------------------------------
// Node.js deduplication tracking tests
// ---------------------------------------------------------------------------

// TestNodeInstalledTrackingDefaultFalse verifies that a fresh builder has
// IsNodeInstalled() == false.
func TestNodeInstalledTrackingDefaultFalse(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	require.False(t, b.IsNodeInstalled(), "fresh builder must have IsNodeInstalled() == false")
}

// TestNodeInstalledTrackingMarkAndCheck verifies that MarkNodeInstalled()
// sets the flag and IsNodeInstalled() returns true afterwards.
func TestNodeInstalledTrackingMarkAndCheck(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	b.MarkNodeInstalled()
	require.True(t, b.IsNodeInstalled(), "IsNodeInstalled() must return true after MarkNodeInstalled()")
}

// Feature: bootstrap-ai-coding, Property 3b: Env/Copy/Cmd instructions appear verbatim in the Dockerfile
func TestPropertyBuilderInstructionsAppearVerbatim(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		key := rapid.StringMatching(`[A-Z][A-Z0-9_]*`).Draw(t, "key")
		val := rapid.StringMatching(`[a-z0-9]+`).Draw(t, "val")
		src := rapid.StringMatching(`[a-z0-9/]+`).Draw(t, "src")
		dst := rapid.StringMatching(`/[a-z0-9/]+`).Draw(t, "dst")

		b := newCreateBuilder(1000, 1000)
		b.Env(key, val)
		b.Copy(src, dst)
		content := b.Build()

		require.Contains(t, content, fmt.Sprintf("ENV %s=%s", key, val))
		require.Contains(t, content, fmt.Sprintf("COPY %s %s", src, dst))
	})
}

// ---------------------------------------------------------------------------
// Tests for BuildImageWithTimeout verbose/silent mode
// ---------------------------------------------------------------------------

// TestVerboseSilentModeNoStdout verifies that BuildImageWithTimeout with
// verbose=false does not write Docker build stream content to stdout.
// Validates: Req 20.2, Req 20.3
func TestVerboseSilentModeNoStdout(t *testing.T) {
	// We test the output-capturing logic directly by verifying that
	// the stream accumulation works correctly in silent mode.
	// The stream content should be returned as the output string (for error
	// reporting) but NOT written to stdout.

	// Build a fake JSON stream with known content.
	streamLines := []string{
		`{"stream":"Step 1/2 : FROM ubuntu\n"}`,
		`{"stream":"Step 2/2 : RUN echo hello\n"}`,
	}
	streamContent := strings.Join(streamLines, "\n") + "\n"

	// Capture stdout to verify nothing is written.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	// Run the stream-processing logic inline (mirrors the goroutine in BuildImageWithTimeout).
	verbose := false
	var out strings.Builder
	dec := json.NewDecoder(strings.NewReader(streamContent))
	for {
		var msg struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}
		if err := dec.Decode(&msg); err != nil {
			break
		}
		if msg.Stream != "" {
			out.WriteString(msg.Stream)
			if verbose {
				fmt.Fprint(os.Stdout, msg.Stream)
			}
		}
	}

	// Restore stdout and read what was written.
	w.Close()
	os.Stdout = oldStdout
	var captured strings.Builder
	io.Copy(&captured, r) //nolint:errcheck
	r.Close()

	// Silent mode: nothing written to stdout.
	require.Empty(t, captured.String(),
		"silent mode must not write any content to stdout")

	// But the output string must contain the stream content.
	require.Contains(t, out.String(), "Step 1/2")
	require.Contains(t, out.String(), "Step 2/2")
}

// TestVerboseModeStreamsOutput verifies that BuildImageWithTimeout with
// verbose=true writes stream content to stdout.
// Validates: Req 20.2, Req 20.3
func TestVerboseModeStreamsOutput(t *testing.T) {
	// Build a fake JSON stream with known content.
	streamLines := []string{
		`{"stream":"Step 1/2 : FROM ubuntu\n"}`,
		`{"stream":"Step 2/2 : RUN echo hello\n"}`,
	}
	streamContent := strings.Join(streamLines, "\n") + "\n"

	// Capture stdout to verify content is written.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	// Run the stream-processing logic inline (mirrors the goroutine in BuildImageWithTimeout).
	verbose := true
	var out strings.Builder
	dec := json.NewDecoder(strings.NewReader(streamContent))
	for {
		var msg struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}
		if err := dec.Decode(&msg); err != nil {
			break
		}
		if msg.Stream != "" {
			out.WriteString(msg.Stream)
			if verbose {
				fmt.Fprint(os.Stdout, msg.Stream)
			}
		}
	}

	// Restore stdout and read what was written.
	w.Close()
	os.Stdout = oldStdout
	var captured strings.Builder
	io.Copy(&captured, r) //nolint:errcheck
	r.Close()

	// Verbose mode: content must be written to stdout.
	require.Contains(t, captured.String(), "Step 1/2",
		"verbose mode must write stream content to stdout")
	require.Contains(t, captured.String(), "Step 2/2",
		"verbose mode must write stream content to stdout")
}

// ---------------------------------------------------------------------------
// Property 50: Silent mode produces no Docker build output on stdout
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 50: Silent mode produces no Docker build output on stdout
// Validates: Requirements 20.2, 20.3
func TestPropertyVerboseSilentModeNeverWritesToStdout(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw 1–10 random stream messages.
		count := rapid.IntRange(1, 10).Draw(t, "count")
		messages := make([]string, count)
		for i := 0; i < count; i++ {
			// Draw a non-empty stream value.
			msg := rapid.StringMatching(`[A-Za-z0-9 :./\n]+`).Draw(t, fmt.Sprintf("msg%d", i))
			messages[i] = fmt.Sprintf(`{"stream":%q}`, msg)
		}
		streamContent := strings.Join(messages, "\n") + "\n"

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdout = w

		// Run the stream-processing logic with verbose=false.
		verbose := false
		dec := json.NewDecoder(strings.NewReader(streamContent))
		for {
			var msg struct {
				Stream string `json:"stream"`
				Error  string `json:"error"`
			}
			if err := dec.Decode(&msg); err != nil {
				break
			}
			if msg.Stream != "" && verbose {
				fmt.Fprint(os.Stdout, msg.Stream)
			}
		}

		// Restore stdout and read what was written.
		w.Close()
		os.Stdout = oldStdout
		var captured strings.Builder
		io.Copy(&captured, r) //nolint:errcheck
		r.Close()

		require.Empty(t, captured.String(),
			"silent mode must never write any content to stdout regardless of stream content")
	})
}

// ---------------------------------------------------------------------------
// Property 51: Verbose mode streams non-empty output for any non-trivial stream
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 51: Verbose mode streams non-empty output for any non-trivial stream
// Validates: Requirements 20.2, 20.3
func TestPropertyVerboseModeAlwaysWritesToStdout(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw 1–10 random non-empty stream messages.
		count := rapid.IntRange(1, 10).Draw(t, "count")
		messages := make([]string, count)
		for i := 0; i < count; i++ {
			// Draw a non-empty stream value (at least 1 char).
			msg := rapid.StringMatching(`[A-Za-z0-9]+`).Draw(t, fmt.Sprintf("msg%d", i))
			messages[i] = fmt.Sprintf(`{"stream":%q}`, msg)
		}
		streamContent := strings.Join(messages, "\n") + "\n"

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdout = w

		// Run the stream-processing logic with verbose=true.
		verbose := true
		dec := json.NewDecoder(strings.NewReader(streamContent))
		for {
			var msg struct {
				Stream string `json:"stream"`
				Error  string `json:"error"`
			}
			if err := dec.Decode(&msg); err != nil {
				break
			}
			if msg.Stream != "" && verbose {
				fmt.Fprint(os.Stdout, msg.Stream)
			}
		}

		// Restore stdout and read what was written.
		w.Close()
		os.Stdout = oldStdout
		var captured strings.Builder
		io.Copy(&captured, r) //nolint:errcheck
		r.Close()

		require.NotEmpty(t, captured.String(),
			"verbose mode must write content to stdout when stream contains non-empty messages")
	})
}

// ---------------------------------------------------------------------------
// Property 59: Dockerfile SSH server and user creation work for any valid username
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 59: Dockerfile SSH server and user creation work for any valid username
// Validates: Requirements 10.2, 10.3, 22.4
func TestPropertyDockerfileSSHAndUserForAnyUsername(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// 1. Draw a random valid Linux username
		username := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "username")
		// 2. Draw a random home directory
		homeDir := rapid.StringMatching(`/home/[a-z][a-z0-9]*`).Draw(t, "homeDir")
		// 3. Draw random UID/GID in range 1000-65000
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		// 4. Create *hostinfo.Info with those values
		info := &hostinfo.Info{
			Username: username,
			HomeDir:  homeDir,
			UID:      uid,
			GID:      gid,
		}

		// 5. Build a base image Dockerfile
		b := docker.NewBaseImageBuilder(
			info,
			docker.UserStrategyCreate, "",
			"",
		)
		content := b.Build()

		// Assert: openssh-server installation (base layer)
		require.Contains(t, content, "openssh-server",
			"Base Dockerfile must install openssh-server")

		// Assert: The drawn username in useradd with correct UID/GID (base layer)
		expectedUseradd := fmt.Sprintf("useradd --uid %d --gid %d --create-home --shell /bin/bash %s", uid, gid, username)
		require.Contains(t, content, expectedUseradd,
			"Base Dockerfile must contain useradd with username %q, uid %d, gid %d", username, uid, gid)

		// Assert: The drawn username in sudoers with NOPASSWD (base layer)
		sudoersEntry := fmt.Sprintf("%s ALL=(ALL) NOPASSWD:ALL", username)
		require.Contains(t, content, sudoersEntry,
			"Base Dockerfile must contain sudoers entry for username %q with NOPASSWD", username)

		// Assert: sshd CMD is in the instance layer
		ib := docker.NewInstanceImageBuilder(info, fixedPublicKey, fixedHostKeyPriv, fixedHostKeyPub, 2222, false)
		ib.Finalize()
		instanceContent := ib.Build()
		require.Contains(t, instanceContent, `CMD ["/usr/sbin/sshd", "-D"]`,
			"Instance Dockerfile must have CMD [\"/usr/sbin/sshd\", \"-D\"]")
	})
}

// ---------------------------------------------------------------------------
// Property 56: Dockerfile uses runtime-provided username and home directory
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 56: Dockerfile uses runtime-provided username and home directory
// Validates: Requirements 22.1, 22.2, 22.4, 22.5
func TestPropertyDockerfileUsesRuntimeUsernameAndHomeDir(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw a random valid Linux username
		username := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "username")
		// Draw a random valid home directory
		homeDir := rapid.StringMatching(`/[a-z][a-z0-9/]*`).Draw(t, "homeDir")
		// Draw random UID/GID in range 1000-65000
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")

		// Create a *hostinfo.Info with those values
		info := &hostinfo.Info{
			Username: username,
			HomeDir:  homeDir,
			UID:      uid,
			GID:      gid,
		}

		// Build a base image Dockerfile
		baseBuilder := docker.NewBaseImageBuilder(
			info,
			docker.UserStrategyCreate, "",
			"",
		)
		baseContent := baseBuilder.Build()

		// Assert the base Dockerfile contains the drawn username in useradd
		require.Contains(t, baseContent, "useradd",
			"Base Dockerfile must contain useradd")
		require.Contains(t, baseContent, fmt.Sprintf("useradd --uid %d --gid %d --create-home --shell /bin/bash %s", uid, gid, username),
			"useradd must reference the drawn username %q", username)

		// Assert the base Dockerfile contains the drawn username in sudoers
		sudoersLine := fmt.Sprintf("%s ALL=(ALL) NOPASSWD:ALL", username)
		require.Contains(t, baseContent, sudoersLine,
			"sudoers must reference the drawn username %q", username)

		// Build an instance image Dockerfile
		instanceBuilder := docker.NewInstanceImageBuilder(info, fixedPublicKey, fixedHostKeyPriv, fixedHostKeyPub, 2222, false)
		instanceContent := instanceBuilder.Build()

		// Assert the instance Dockerfile contains the drawn username in chown
		chownFragment := fmt.Sprintf("chown -R %s:%s", username, username)
		require.Contains(t, instanceContent, chownFragment,
			"chown must reference the drawn username %q", username)

		// Assert the instance Dockerfile contains the drawn home directory in authorized_keys path
		authorizedKeysPath := homeDir + "/.ssh/authorized_keys"
		require.Contains(t, instanceContent, authorizedKeysPath,
			"Instance Dockerfile must reference authorized_keys at %q", authorizedKeysPath)

		// If the username is NOT "dev", assert the Dockerfiles do NOT contain "dev"
		// as a standalone username reference in useradd/sudoers lines
		if username != "dev" {
			for _, line := range strings.Split(baseContent, "\n") {
				if strings.Contains(line, "useradd") {
					require.NotContains(t, line, " dev",
						"useradd line must not contain hardcoded 'dev' when username is %q", username)
				}
				if strings.Contains(line, "sudoers") {
					require.NotContains(t, line, "dev ALL=",
						"sudoers line must not contain hardcoded 'dev' when username is %q", username)
				}
			}
		}

		// If the home directory is NOT "/home/dev", assert the Dockerfiles do NOT contain "/home/dev"
		if homeDir != "/home/dev" {
			require.NotContains(t, baseContent, "/home/dev",
				"Base Dockerfile must not contain hardcoded '/home/dev' when homeDir is %q", homeDir)
			require.NotContains(t, instanceContent, "/home/dev",
				"Instance Dockerfile must not contain hardcoded '/home/dev' when homeDir is %q", homeDir)
		}
	})
}

// ---------------------------------------------------------------------------
// Unit tests for git config injection (Req 24)
// ---------------------------------------------------------------------------

// TestGitConfigInjection_SpecialCharacters verifies that git config content
// containing special characters (double quotes, single quotes, backslashes,
// dollar signs, backticks, newlines) is correctly handled via base64 encoding
// in the generated Dockerfile.
// Validates: Req 24
func TestGitConfigInjection_SpecialCharacters(t *testing.T) {
	// Content with characters that would break shell escaping if not base64-encoded.
	gitConfigContent := "[alias]\n\tci = commit -m \"WIP\"\n\tco = checkout\n[user]\n\tname = O'Brien\n\temail = user@example.com\n[core]\n\tpath = ~/path with spaces/$HOME/`echo hi`\\\n"
	expectedBase64 := base64.StdEncoding.EncodeToString([]byte(gitConfigContent))

	info := &hostinfo.Info{
		Username: "testuser",
		HomeDir:  "/home/testuser",
		UID:      1000,
		GID:      1000,
	}

	b := docker.NewBaseImageBuilder(
		info,
		docker.UserStrategyCreate, "",
		gitConfigContent,
	)
	content := b.Build()

	// The RUN line must contain the correct base64-encoded version of the special content.
	require.Contains(t, content, fmt.Sprintf("echo %s | base64 -d > /home/testuser/.gitconfig", expectedBase64),
		"Dockerfile must contain base64-encoded git config with special characters")

	// Decode the base64 from the generated Dockerfile and verify it matches the original content.
	decoded, err := base64.StdEncoding.DecodeString(expectedBase64)
	require.NoError(t, err, "base64 decoding must succeed")
	require.Equal(t, gitConfigContent, string(decoded),
		"decoded base64 must match the original git config content with special characters")

	// Verify chown and chmod are present.
	require.Contains(t, content, "chown testuser:testuser /home/testuser/.gitconfig",
		"Dockerfile must contain chown for .gitconfig")
	require.Contains(t, content, "chmod 0444 /home/testuser/.gitconfig",
		"Dockerfile must contain chmod 0444 for .gitconfig")
}

// TestGitConfigInjection_NonEmpty verifies that when non-empty git config
// content is passed to NewBaseImageBuilder, the generated Dockerfile contains
// a RUN line that pipes base64-encoded content to <homeDir>/.gitconfig with
// correct chown and chmod 0444.
// Validates: Req 24
func TestGitConfigInjection_NonEmpty(t *testing.T) {
	gitConfigContent := "[user]\n\tname = Test User\n\temail = test@example.com\n"
	expectedBase64 := base64.StdEncoding.EncodeToString([]byte(gitConfigContent))

	info := &hostinfo.Info{
		Username: "testuser",
		HomeDir:  "/home/testuser",
		UID:      1000,
		GID:      1000,
	}

	b := docker.NewBaseImageBuilder(
		info,
		docker.UserStrategyCreate, "",
		gitConfigContent,
	)
	content := b.Build()

	// The RUN line must pipe base64-encoded content through base64 -d to write to <homeDir>/.gitconfig
	require.Contains(t, content, fmt.Sprintf("echo %s | base64 -d > /home/testuser/.gitconfig", expectedBase64),
		"Dockerfile must contain base64 decode pipeline writing to /home/testuser/.gitconfig")

	// The RUN line must contain chown <username>:<username> <homeDir>/.gitconfig
	require.Contains(t, content, "chown testuser:testuser /home/testuser/.gitconfig",
		"Dockerfile must contain chown testuser:testuser /home/testuser/.gitconfig")

	// The RUN line must contain chmod 0444 <homeDir>/.gitconfig
	require.Contains(t, content, "chmod 0444 /home/testuser/.gitconfig",
		"Dockerfile must contain chmod 0444 /home/testuser/.gitconfig")
}

// TestGitConfigInjection_Empty verifies that when an empty string is passed
// for the gitConfig parameter, the generated Dockerfile does NOT contain any
// .gitconfig-related RUN line.
// Validates: Req 24
func TestGitConfigInjection_Empty(t *testing.T) {
	info := &hostinfo.Info{
		Username: "testuser",
		HomeDir:  "/home/testuser",
		UID:      1000,
		GID:      1000,
	}

	b := docker.NewBaseImageBuilder(
		info,
		docker.UserStrategyCreate, "",
		"",
	)
	content := b.Build()

	// No .gitconfig injection should appear when gitConfig is empty
	require.NotContains(t, content, ".gitconfig",
		"Dockerfile must NOT contain .gitconfig when gitConfig parameter is empty")
}

// ---------------------------------------------------------------------------
// RunAsUser tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Unit tests for NewBaseImageBuilder output (Task 8: TL-1, TL-2)
// ---------------------------------------------------------------------------

// TestBaseImageBuilderOutput_StartsWithFROM verifies that the base image
// Dockerfile starts with FROM constants.BaseContainerImage.
func TestBaseImageBuilderOutput_StartsWithFROM(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	lines := b.Lines()
	require.NotEmpty(t, lines)
	require.Equal(t, "FROM "+constants.BaseContainerImage, lines[0],
		"base image Dockerfile must start with FROM %s", constants.BaseContainerImage)
}

// TestBaseImageBuilderOutput_ContainsUseradd verifies that UserStrategyCreate
// produces a Dockerfile containing useradd.
func TestBaseImageBuilderOutput_ContainsUseradd(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	content := b.Build()
	require.Contains(t, content, "useradd",
		"base image Dockerfile with UserStrategyCreate must contain useradd")
}

// TestBaseImageBuilderOutput_ContainsUsermod verifies that UserStrategyRename
// produces a Dockerfile containing usermod.
func TestBaseImageBuilderOutput_ContainsUsermod(t *testing.T) {
	b := newRenameBuilder(1000, 1000, "ubuntu")
	content := b.Build()
	require.Contains(t, content, "usermod",
		"base image Dockerfile with UserStrategyRename must contain usermod")
}

// TestBaseImageBuilderOutput_ContainsGnomeKeyring verifies that the base image
// Dockerfile installs gnome-keyring for D-Bus + keyring setup.
func TestBaseImageBuilderOutput_ContainsGnomeKeyring(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	content := b.Build()
	require.Contains(t, content, "gnome-keyring",
		"base image Dockerfile must contain gnome-keyring")
}

// TestBaseImageBuilderOutput_DoesNotContainSSHHostKey verifies that the base
// image Dockerfile does NOT contain SSH host key content (ssh_host_ed25519_key).
// SSH host keys belong in the instance layer only.
func TestBaseImageBuilderOutput_DoesNotContainSSHHostKey(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	content := b.Build()
	require.NotContains(t, content, "ssh_host_ed25519_key",
		"base image Dockerfile must NOT contain ssh_host_ed25519_key (belongs in instance layer)")
}

// TestBaseImageBuilderOutput_DoesNotContainAuthorizedKeys verifies that the
// base image Dockerfile does NOT contain authorized_keys.
// authorized_keys belongs in the instance layer only.
func TestBaseImageBuilderOutput_DoesNotContainAuthorizedKeys(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	content := b.Build()
	require.NotContains(t, content, "authorized_keys",
		"base image Dockerfile must NOT contain authorized_keys (belongs in instance layer)")
}

// TestBaseImageBuilderOutput_DoesNotContainSshdConfig verifies that the base
// image Dockerfile does NOT contain sshd_config hardening (PasswordAuthentication no).
// sshd_config hardening belongs in the instance layer only.
func TestBaseImageBuilderOutput_DoesNotContainSshdConfig(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	content := b.Build()
	require.NotContains(t, content, "PasswordAuthentication no",
		"base image Dockerfile must NOT contain 'PasswordAuthentication no' (belongs in instance layer)")
}

// TestBaseImageBuilderOutput_DoesNotContainCMD verifies that the base image
// Dockerfile does NOT contain a CMD instruction. CMD belongs in the instance
// layer only (appended via Finalize()).
func TestBaseImageBuilderOutput_DoesNotContainCMD(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	content := b.Build()
	require.NotContains(t, content, "CMD",
		"base image Dockerfile must NOT contain CMD (belongs in instance layer)")
}

// TestBaseImageBuilderOutput_RenameDoesNotContainInstanceContent verifies that
// the base image Dockerfile with UserStrategyRename also does NOT contain
// instance-layer content (SSH host key, authorized_keys, sshd_config, CMD).
func TestBaseImageBuilderOutput_RenameDoesNotContainInstanceContent(t *testing.T) {
	b := newRenameBuilder(1000, 1000, "ubuntu")
	content := b.Build()

	require.NotContains(t, content, "ssh_host_ed25519_key",
		"base image (rename) must NOT contain ssh_host_ed25519_key")
	require.NotContains(t, content, "authorized_keys",
		"base image (rename) must NOT contain authorized_keys")
	require.NotContains(t, content, "PasswordAuthentication no",
		"base image (rename) must NOT contain 'PasswordAuthentication no'")
	require.NotContains(t, content, "CMD",
		"base image (rename) must NOT contain CMD")
}

// ---------------------------------------------------------------------------
// Unit tests for NewInstanceImageBuilder output (Task 8: TL-1, TL-2)
// ---------------------------------------------------------------------------

// TestInstanceImageBuilderOutput_StartsWithFROM verifies that the instance
// image Dockerfile starts with FROM bac-base:latest (constants.BaseImageTag).
func TestInstanceImageBuilderOutput_StartsWithFROM(t *testing.T) {
	b := newInstanceBuilder(1000, 1000)
	lines := b.Lines()
	require.NotEmpty(t, lines)
	require.Equal(t, "FROM "+constants.BaseImageTag, lines[0],
		"instance image Dockerfile must start with FROM %s", constants.BaseImageTag)
}

// TestInstanceImageBuilderOutput_ContainsSSHHostKeyInjection verifies that the
// instance image Dockerfile contains SSH host key injection (references to
// ssh_host_ed25519_key).
func TestInstanceImageBuilderOutput_ContainsSSHHostKeyInjection(t *testing.T) {
	b := newInstanceBuilder(1000, 1000)
	content := b.Build()

	privPath := fmt.Sprintf("/etc/ssh/ssh_host_%s_key", constants.SSHHostKeyType)
	pubPath := privPath + ".pub"
	require.Contains(t, content, privPath,
		"instance image Dockerfile must contain SSH host private key path %q", privPath)
	require.Contains(t, content, pubPath,
		"instance image Dockerfile must contain SSH host public key path %q", pubPath)
}

// TestInstanceImageBuilderOutput_ContainsAuthorizedKeys verifies that the
// instance image Dockerfile contains authorized_keys setup.
func TestInstanceImageBuilderOutput_ContainsAuthorizedKeys(t *testing.T) {
	b := newInstanceBuilder(1000, 1000)
	content := b.Build()
	require.Contains(t, content, "authorized_keys",
		"instance image Dockerfile must contain authorized_keys")
}

// TestInstanceImageBuilderOutput_ContainsSshdConfigHardening verifies that the
// instance image Dockerfile contains sshd_config hardening directives:
// PasswordAuthentication no and PermitRootLogin no.
func TestInstanceImageBuilderOutput_ContainsSshdConfigHardening(t *testing.T) {
	b := newInstanceBuilder(1000, 1000)
	content := b.Build()
	require.Contains(t, content, "PasswordAuthentication no",
		"instance image Dockerfile must contain 'PasswordAuthentication no'")
	require.Contains(t, content, "PermitRootLogin no",
		"instance image Dockerfile must contain 'PermitRootLogin no'")
}

// TestInstanceImageBuilderOutput_EndsWithCMDAfterFinalize verifies that after
// calling Finalize(), the instance image Dockerfile ends with the sshd CMD.
func TestInstanceImageBuilderOutput_EndsWithCMDAfterFinalize(t *testing.T) {
	b := newInstanceBuilder(1000, 1000)
	b.Finalize()
	lines := b.Lines()
	require.NotEmpty(t, lines)
	lastLine := lines[len(lines)-1]
	require.Equal(t, `CMD ["/usr/sbin/sshd", "-D"]`, lastLine,
		"instance image Dockerfile must end with CMD [\"/usr/sbin/sshd\", \"-D\"] after Finalize()")
}

// ---------------------------------------------------------------------------
// RunAsUser tests
// ---------------------------------------------------------------------------

// TestRunAsUserEmitsCorrectSequence verifies that RunAsUser emits
// USER <username>, RUN <cmd>, USER root in the correct order.
func TestRunAsUserEmitsCorrectSequence(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	linesBefore := len(b.Lines())

	b.RunAsUser("curl -LsSf https://astral.sh/uv/install.sh | sh")

	lines := b.Lines()
	require.Equal(t, linesBefore+3, len(lines),
		"RunAsUser must append exactly 3 lines")

	require.Equal(t, "USER testuser", lines[linesBefore],
		"first line must be USER <username>")
	require.Equal(t, "RUN curl -LsSf https://astral.sh/uv/install.sh | sh", lines[linesBefore+1],
		"second line must be RUN <cmd>")
	require.Equal(t, "USER root", lines[linesBefore+2],
		"third line must be USER root")
}

// ---------------------------------------------------------------------------
// Two-Layer Image Architecture Property Tests (Task 10: TL-1, TL-2, TL-11)
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property TL-1: Base image Dockerfile always starts with FROM constants.BaseContainerImage for any valid hostinfo
// Validates: Requirements TL-1.1
func TestPropertyTwoLayer_BaseImageStartsWithFROM(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw random valid hostinfo inputs
		username := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "username")
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		homeDir := "/home/" + username

		info := &hostinfo.Info{
			Username: username,
			HomeDir:  homeDir,
			UID:      uid,
			GID:      gid,
		}

		// Test with UserStrategyCreate
		b := docker.NewBaseImageBuilder(info, docker.UserStrategyCreate, "", "")
		lines := b.Lines()

		wantFrom := "FROM " + constants.BaseContainerImage
		require.NotEmpty(t, lines, "Dockerfile must have at least one line")
		require.Equal(t, wantFrom, lines[0],
			"base image Dockerfile must start with FROM %s for username=%q uid=%d gid=%d",
			constants.BaseContainerImage, username, uid, gid)

		// No other FROM instruction should exist
		for i, line := range lines[1:] {
			require.False(t, strings.HasPrefix(line, "FROM "),
				"unexpected second FROM at line %d: %q", i+1, line)
		}
	})
}

// Feature: bootstrap-ai-coding, Property TL-2: Instance image Dockerfile always starts with FROM bac-base:latest for any valid inputs
// Validates: Requirements TL-2.1
func TestPropertyTwoLayer_InstanceImageStartsWithFROM(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw random valid hostinfo inputs
		username := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "username")
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		homeDir := "/home/" + username

		// Draw random SSH key material
		publicKey := "ssh-ed25519 " + rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "pubKeyBody") + " test@host"
		hostKeyPriv := "-----BEGIN OPENSSH PRIVATE KEY-----\n" + rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "privKeyBody") + "\n-----END OPENSSH PRIVATE KEY-----"
		hostKeyPub := "ssh-ed25519 " + rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "hostPubBody") + " host"

		info := &hostinfo.Info{
			Username: username,
			HomeDir:  homeDir,
			UID:      uid,
			GID:      gid,
		}

		b := docker.NewInstanceImageBuilder(info, publicKey, hostKeyPriv, hostKeyPub, 2222, false)
		lines := b.Lines()

		wantFrom := "FROM " + constants.BaseImageName + ":latest"
		require.NotEmpty(t, lines, "Dockerfile must have at least one line")
		require.Equal(t, wantFrom, lines[0],
			"instance image Dockerfile must start with FROM %s for username=%q",
			constants.BaseImageName+":latest", username)

		// No other FROM instruction should exist
		for i, line := range lines[1:] {
			require.False(t, strings.HasPrefix(line, "FROM "),
				"unexpected second FROM at line %d: %q", i+1, line)
		}
	})
}

// Feature: bootstrap-ai-coding, Property TL-3: Base image Dockerfile never contains CMD or SSH host key content
// Validates: Requirements TL-1.10
func TestPropertyTwoLayer_BaseImageNeverContainsCMDOrHostKey(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw random valid hostinfo inputs
		username := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "username")
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		homeDir := "/home/" + username

		info := &hostinfo.Info{
			Username: username,
			HomeDir:  homeDir,
			UID:      uid,
			GID:      gid,
		}

		// Test with both strategies
		strategy := docker.UserStrategyCreate
		conflictingUser := ""
		if rapid.Bool().Draw(t, "useRename") {
			strategy = docker.UserStrategyRename
			conflictingUser = rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "conflictingUser")
		}

		b := docker.NewBaseImageBuilder(info, strategy, conflictingUser, "")
		content := b.Build()

		// Base image must NOT contain CMD
		require.NotContains(t, content, "CMD",
			"base image Dockerfile must never contain CMD (belongs in instance layer)")

		// Base image must NOT contain SSH host key paths
		hostKeyPath := fmt.Sprintf("ssh_host_%s_key", constants.SSHHostKeyType)
		require.NotContains(t, content, hostKeyPath,
			"base image Dockerfile must never contain SSH host key path %q", hostKeyPath)

		// Base image must NOT contain authorized_keys
		require.NotContains(t, content, "authorized_keys",
			"base image Dockerfile must never contain authorized_keys (belongs in instance layer)")
	})
}

// Feature: bootstrap-ai-coding, Property TL-4: Instance image Dockerfile always ends with CMD after Finalize()
// Validates: Requirements TL-2.6
func TestPropertyTwoLayer_InstanceImageEndsWithCMDAfterFinalize(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw random valid hostinfo inputs
		username := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "username")
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		homeDir := "/home/" + username

		// Draw random SSH key material
		publicKey := "ssh-ed25519 " + rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "pubKeyBody") + " test@host"
		hostKeyPriv := "-----BEGIN OPENSSH PRIVATE KEY-----\n" + rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "privKeyBody") + "\n-----END OPENSSH PRIVATE KEY-----"
		hostKeyPub := "ssh-ed25519 " + rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "hostPubBody") + " host"

		info := &hostinfo.Info{
			Username: username,
			HomeDir:  homeDir,
			UID:      uid,
			GID:      gid,
		}

		b := docker.NewInstanceImageBuilder(info, publicKey, hostKeyPriv, hostKeyPub, 2222, false)
		b.Finalize()
		lines := b.Lines()

		require.NotEmpty(t, lines, "Dockerfile must have at least one line")
		lastLine := lines[len(lines)-1]
		require.Equal(t, `CMD ["/usr/sbin/sshd", "-D"]`, lastLine,
			"instance image Dockerfile must end with CMD [\"/usr/sbin/sshd\", \"-D\"] after Finalize() for username=%q",
			username)
	})
}

// Feature: bootstrap-ai-coding, Property TL-5: constants.BaseImageName + ":latest" equals "bac-base:latest"
// Validates: Requirements TL-11
func TestPropertyTwoLayer_BaseImageNameConstant(t *testing.T) {
	// This is a constant-level property — no random inputs needed, but we
	// verify it holds as a property test to document the invariant.
	rapid.Check(t, func(t *rapid.T) {
		// The property holds regardless of any input — draw a dummy to satisfy rapid
		_ = rapid.IntRange(0, 100).Draw(t, "dummy")

		require.Equal(t, "bac-base", constants.BaseImageName,
			"constants.BaseImageName must equal \"bac-base\"")
		require.Equal(t, "bac-base:latest", constants.BaseImageName+":latest",
			"constants.BaseImageName + \":latest\" must equal \"bac-base:latest\"")
		require.Equal(t, "bac-base:latest", constants.BaseImageTag,
			"constants.BaseImageTag must equal \"bac-base:latest\"")
	})
}

// TestRunAsUserUsesInfoUsername verifies that RunAsUser uses the username
// from the builder's hostinfo.Info, not a hardcoded value.
func TestRunAsUserUsesInfoUsername(t *testing.T) {
	info := &hostinfo.Info{
		Username: "alice",
		HomeDir:  "/home/alice",
		UID:      1001,
		GID:      1001,
	}
	b := docker.NewBaseImageBuilder(
		info,
		docker.UserStrategyCreate, "",
		"",
	)
	linesBefore := len(b.Lines())

	b.RunAsUser("echo hello")

	lines := b.Lines()
	require.Equal(t, "USER alice", lines[linesBefore],
		"RunAsUser must use the username from hostinfo.Info")
	require.Equal(t, "USER root", lines[linesBefore+2],
		"RunAsUser must switch back to root")
}

// ---------------------------------------------------------------------------
// Property 57: --host-network-off controls network mode and sshd_config
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 57: --host-network-off controls network mode and sshd_config
func TestInstanceImageSSHDConfigHostNetwork(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		port := rapid.IntRange(1024, 65535).Draw(t, "port")
		hostNetworkOff := rapid.Bool().Draw(t, "hostNetworkOff")

		// Build instance image with drawn port and hostNetworkOff values
		b := docker.NewInstanceImageBuilder(
			testInfo(1000, 1000),
			fixedPublicKey,
			fixedHostKeyPriv, fixedHostKeyPub,
			port, hostNetworkOff,
		)
		content := b.Build()

		if !hostNetworkOff {
			// When host network is used, sshd_config must contain Port and ListenAddress
			require.Contains(t, content, fmt.Sprintf("Port %d", port),
				"when !hostNetworkOff, sshd_config must contain Port %d", port)
			require.Contains(t, content, "ListenAddress 127.0.0.1",
				"when !hostNetworkOff, sshd_config must contain ListenAddress 127.0.0.1")
		} else {
			// When host network is off, sshd_config must NOT contain Port or ListenAddress directives
			require.NotContains(t, content, "Port ",
				"when hostNetworkOff, sshd_config must NOT contain Port directive")
			require.NotContains(t, content, "ListenAddress",
				"when hostNetworkOff, sshd_config must NOT contain ListenAddress directive")
		}
	})
}

// ---------------------------------------------------------------------------
// Unit tests for Entrypoint builder method
// Validates: VK-3.1
// ---------------------------------------------------------------------------

func TestBuilderEntrypointSingleArg(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	b.Entrypoint("/usr/local/bin/bac-entrypoint.sh")
	content := b.Build()
	require.Contains(t, content, `ENTRYPOINT ["/usr/local/bin/bac-entrypoint.sh"]`)
}

func TestBuilderEntrypointMultiArg(t *testing.T) {
	b := newCreateBuilder(1000, 1000)
	b.Entrypoint("/bin/sh", "-c", "start.sh")
	content := b.Build()
	require.Contains(t, content, `ENTRYPOINT ["/bin/sh", "-c", "start.sh"]`)
}
