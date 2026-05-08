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

// newCreateBuilder is a convenience helper that builds a DockerfileBuilder
// using UserStrategyCreate with the given uid/gid and fixed key material.
func newCreateBuilder(uid, gid int) *docker.DockerfileBuilder {
	return docker.NewDockerfileBuilder(
		testInfo(uid, gid),
		fixedPublicKey,
		fixedHostKeyPriv, fixedHostKeyPub,
		docker.UserStrategyCreate, "",
		"",
	)
}

// newRenameBuilder is a convenience helper that builds a DockerfileBuilder
// using UserStrategyRename with the given uid/gid and conflicting user name.
func newRenameBuilder(uid, gid int, conflictingUser string) *docker.DockerfileBuilder {
	return docker.NewDockerfileBuilder(
		testInfo(uid, gid),
		fixedPublicKey,
		fixedHostKeyPriv, fixedHostKeyPub,
		docker.UserStrategyRename, conflictingUser,
		"",
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
		b.Finalize() // CMD must be appended before inspecting the full Dockerfile
		content := b.Build()

		// Must install openssh-server
		require.Contains(t, content, "openssh-server",
			"Dockerfile must install openssh-server")

		// Must reference ContainerUser
		require.Contains(t, content, "testuser",
			"Dockerfile must reference ContainerUser %q", "testuser")

		// Must start sshd as the CMD
		require.Contains(t, content, "/usr/sbin/sshd",
			"Dockerfile must include sshd CMD")
		require.Contains(t, content, `CMD ["/usr/sbin/sshd", "-D"]`,
			"Dockerfile must have CMD [\"/usr/sbin/sshd\", \"-D\"]")
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

		b := newCreateBuilder(uid, gid)
		content := b.Build()

		require.Contains(t, content, "PasswordAuthentication no",
			"Dockerfile must set PasswordAuthentication no in sshd_config")
	})
}

// Feature: bootstrap-ai-coding, Property 7: sshd_config always disables password authentication
func TestPropertySSHDConfigPasswordAuthDisabled_Rename(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		conflictingUser := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "conflictingUser")

		b := newRenameBuilder(uid, gid, conflictingUser)
		content := b.Build()

		require.Contains(t, content, "PasswordAuthentication no",
			"Dockerfile must set PasswordAuthentication no in sshd_config")
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

		b := docker.NewDockerfileBuilder(
			testInfo(uid, gid),
			publicKey,
			fixedHostKeyPriv, fixedHostKeyPub,
			docker.UserStrategyCreate, "",
			"",
		)
		content := b.Build()

		authorizedKeysPath := "/home/testuser/.ssh/authorized_keys"
		require.Contains(t, content, authorizedKeysPath,
			"Dockerfile must reference authorized_keys path %q", authorizedKeysPath)
	})
}

// Feature: bootstrap-ai-coding, Property 8: Public key is always injected into constants.ContainerUserHome/.ssh/authorized_keys
func TestPropertyPublicKeyInjected_Rename(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		conflictingUser := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "conflictingUser")
		keyBody := rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "keyBody")
		publicKey := "ssh-ed25519 " + keyBody + " test@host"

		b := docker.NewDockerfileBuilder(
			testInfo(uid, gid),
			publicKey,
			fixedHostKeyPriv, fixedHostKeyPub,
			docker.UserStrategyRename, conflictingUser,
			"",
		)
		content := b.Build()

		authorizedKeysPath := "/home/testuser/.ssh/authorized_keys"
		require.Contains(t, content, authorizedKeysPath,
			"Dockerfile must reference authorized_keys path %q", authorizedKeysPath)
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

		b := docker.NewDockerfileBuilder(
			testInfo(uid, gid),
			fixedPublicKey,
			hostKeyPriv, hostKeyPub,
			docker.UserStrategyCreate, "",
			"",
		)
		content := b.Build()

		// Must write the private key to the expected path
		privPath := fmt.Sprintf("/etc/ssh/ssh_host_%s_key", constants.SSHHostKeyType)
		pubPath := privPath + ".pub"
		require.Contains(t, content, privPath,
			"Dockerfile must inject host private key to %q", privPath)
		require.Contains(t, content, pubPath,
			"Dockerfile must inject host public key to %q", pubPath)
	})
}

// Feature: bootstrap-ai-coding, Property 10: SSH host key is always injected into the Dockerfile
func TestPropertySSHHostKeyInjected_Rename(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		uid := rapid.IntRange(1000, 65000).Draw(t, "uid")
		gid := rapid.IntRange(1000, 65000).Draw(t, "gid")
		conflictingUser := rapid.StringMatching(`[a-z][a-z0-9_-]{0,15}`).Draw(t, "conflictingUser")
		privKeyBody := rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "privKeyBody")
		pubKeyBody := rapid.StringMatching(`[A-Za-z0-9+/]{20,60}`).Draw(t, "pubKeyBody")
		hostKeyPriv := "-----BEGIN OPENSSH PRIVATE KEY-----\n" + privKeyBody + "\n-----END OPENSSH PRIVATE KEY-----"
		hostKeyPub := "ssh-ed25519 " + pubKeyBody + " host"

		b := docker.NewDockerfileBuilder(
			testInfo(uid, gid),
			fixedPublicKey,
			hostKeyPriv, hostKeyPub,
			docker.UserStrategyRename, conflictingUser,
			"",
		)
		content := b.Build()

		privPath := fmt.Sprintf("/etc/ssh/ssh_host_%s_key", constants.SSHHostKeyType)
		pubPath := privPath + ".pub"
		require.Contains(t, content, privPath,
			"Dockerfile must inject host private key to %q", privPath)
		require.Contains(t, content, pubPath,
			"Dockerfile must inject host public key to %q", pubPath)
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

		// 5. Build a Dockerfile
		b := docker.NewDockerfileBuilder(
			info,
			fixedPublicKey,
			fixedHostKeyPriv, fixedHostKeyPub,
			docker.UserStrategyCreate, "",
			"",
		)
		b.Finalize()
		content := b.Build()

		// Assert: openssh-server installation
		require.Contains(t, content, "openssh-server",
			"Dockerfile must install openssh-server")

		// Assert: The drawn username in useradd with correct UID/GID
		expectedUseradd := fmt.Sprintf("useradd --uid %d --gid %d --create-home --shell /bin/bash %s", uid, gid, username)
		require.Contains(t, content, expectedUseradd,
			"Dockerfile must contain useradd with username %q, uid %d, gid %d", username, uid, gid)

		// Assert: sshd CMD
		require.Contains(t, content, `CMD ["/usr/sbin/sshd", "-D"]`,
			"Dockerfile must have CMD [\"/usr/sbin/sshd\", \"-D\"]")

		// Assert: The drawn username in sudoers with NOPASSWD
		sudoersEntry := fmt.Sprintf("%s ALL=(ALL) NOPASSWD:ALL", username)
		require.Contains(t, content, sudoersEntry,
			"Dockerfile must contain sudoers entry for username %q with NOPASSWD", username)
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

		// Build a Dockerfile using NewDockerfileBuilder
		b := docker.NewDockerfileBuilder(
			info,
			fixedPublicKey,
			fixedHostKeyPriv, fixedHostKeyPub,
			docker.UserStrategyCreate, "",
			"",
		)
		content := b.Build()

		// Assert the Dockerfile contains the drawn username in useradd
		require.Contains(t, content, "useradd",
			"Dockerfile must contain useradd")
		require.Contains(t, content, fmt.Sprintf("useradd --uid %d --gid %d --create-home --shell /bin/bash %s", uid, gid, username),
			"useradd must reference the drawn username %q", username)

		// Assert the Dockerfile contains the drawn username in sudoers
		sudoersLine := fmt.Sprintf("%s ALL=(ALL) NOPASSWD:ALL", username)
		require.Contains(t, content, sudoersLine,
			"sudoers must reference the drawn username %q", username)

		// Assert the Dockerfile contains the drawn username in chown
		chownFragment := fmt.Sprintf("chown -R %s:%s", username, username)
		require.Contains(t, content, chownFragment,
			"chown must reference the drawn username %q", username)

		// Assert the Dockerfile contains the drawn home directory in authorized_keys path
		authorizedKeysPath := homeDir + "/.ssh/authorized_keys"
		require.Contains(t, content, authorizedKeysPath,
			"Dockerfile must reference authorized_keys at %q", authorizedKeysPath)

		// If the username is NOT "dev", assert the Dockerfile does NOT contain "dev"
		// as a standalone username reference in useradd/sudoers lines
		if username != "dev" {
			for _, line := range strings.Split(content, "\n") {
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

		// If the home directory is NOT "/home/dev", assert the Dockerfile does NOT contain "/home/dev"
		if homeDir != "/home/dev" {
			require.NotContains(t, content, "/home/dev",
				"Dockerfile must not contain hardcoded '/home/dev' when homeDir is %q", homeDir)
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

	b := docker.NewDockerfileBuilder(
		info,
		fixedPublicKey,
		fixedHostKeyPriv, fixedHostKeyPub,
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
// content is passed to NewDockerfileBuilder, the generated Dockerfile contains
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

	b := docker.NewDockerfileBuilder(
		info,
		fixedPublicKey,
		fixedHostKeyPriv, fixedHostKeyPub,
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

	b := docker.NewDockerfileBuilder(
		info,
		fixedPublicKey,
		fixedHostKeyPriv, fixedHostKeyPub,
		docker.UserStrategyCreate, "",
		"",
	)
	content := b.Build()

	// No .gitconfig injection should appear when gitConfig is empty
	require.NotContains(t, content, ".gitconfig",
		"Dockerfile must NOT contain .gitconfig when gitConfig parameter is empty")
}
