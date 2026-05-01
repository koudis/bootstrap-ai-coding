package ssh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	internalssh "github.com/koudis/bootstrap-ai-coding/internal/ssh"
)

// Feature: bootstrap-ai-coding, Property 9: Public key discovery respects precedence order
func TestDiscoverPublicKeyPrecedence(t *testing.T) {
	// Unique content per file so we can assert which one was returned.
	const (
		flagContent    = "ssh-ed25519 FLAGKEY flag-key\n"
		ed25519Content = "ssh-ed25519 ED25519KEY ed25519-key\n"
		rsaContent     = "ssh-rsa RSAKEY rsa-key\n"
	)

	// Set up a temp home directory once for the whole property run so we
	// control ~/.ssh/. rapid.T does not expose TempDir/Setenv, so we use
	// the outer *testing.T for environment setup.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	sshDir := filepath.Join(tmpHome, ".ssh")
	require.NoError(t, os.MkdirAll(sshDir, 0o700))

	// A stable path for the flag key file (outside ~/.ssh).
	flagKeyPath := filepath.Join(tmpHome, "flag_key.pub")

	ed25519Path := filepath.Join(sshDir, "id_ed25519.pub")
	rsaPath := filepath.Join(sshDir, "id_rsa.pub")

	rapid.Check(t, func(rt *rapid.T) {
		// Draw booleans for which candidate files exist.
		flagFileExists := rapid.Bool().Draw(rt, "flagFileExists")
		ed25519Exists := rapid.Bool().Draw(rt, "ed25519Exists")
		rsaExists := rapid.Bool().Draw(rt, "rsaExists")

		// Reset the filesystem state for this iteration.
		_ = os.Remove(flagKeyPath)
		_ = os.Remove(ed25519Path)
		_ = os.Remove(rsaPath)

		if flagFileExists {
			require.NoError(rt, os.WriteFile(flagKeyPath, []byte(flagContent), 0o600))
		}
		if ed25519Exists {
			require.NoError(rt, os.WriteFile(ed25519Path, []byte(ed25519Content), 0o600))
		}
		if rsaExists {
			require.NoError(rt, os.WriteFile(rsaPath, []byte(rsaContent), 0o600))
		}

		// Determine the flag value: non-empty only when the flag file was created.
		sshKeyFlag := ""
		if flagFileExists {
			sshKeyFlag = flagKeyPath
		}

		// Determine expected result based on precedence:
		//   1. sshKeyFlag (if non-empty and file exists)
		//   2. ~/.ssh/id_ed25519.pub (if exists)
		//   3. ~/.ssh/id_rsa.pub (if exists)
		//   4. error (none found)
		type expectation struct {
			content string
			hasErr  bool
		}
		var expected expectation
		switch {
		case flagFileExists: // sshKeyFlag is non-empty and the file exists
			expected = expectation{content: flagContent}
		case ed25519Exists:
			expected = expectation{content: ed25519Content}
		case rsaExists:
			expected = expectation{content: rsaContent}
		default:
			expected = expectation{hasErr: true}
		}

		got, err := internalssh.DiscoverPublicKey(sshKeyFlag)

		if expected.hasErr {
			require.Error(rt, err,
				"expected an error when no key files exist (flag=%q, ed25519=%v, rsa=%v)",
				sshKeyFlag, ed25519Exists, rsaExists)
		} else {
			require.NoError(rt, err,
				"unexpected error (flag=%q, ed25519=%v, rsa=%v): %v",
				sshKeyFlag, ed25519Exists, rsaExists, err)
			require.Equal(rt, expected.content, got,
				"wrong key returned (flag=%q, ed25519=%v, rsa=%v)",
				sshKeyFlag, ed25519Exists, rsaExists)
		}
	})
}

// TestDiscoverPublicKeyNoKeyFound verifies the error message when no key is available.
func TestDiscoverPublicKeyNoKeyFound(t *testing.T) {
	// Point HOME at an empty temp dir so no default keys exist.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	_, err := internalssh.DiscoverPublicKey("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no SSH public key found")
}

// TestGenerateHostKeyPairProducesValidKeys verifies that GenerateHostKeyPair
// returns a non-empty PEM private key and an authorized_keys-format public key.
func TestGenerateHostKeyPairProducesValidKeys(t *testing.T) {
	priv, pub, err := internalssh.GenerateHostKeyPair()
	require.NoError(t, err)
	require.NotEmpty(t, priv, "private key must not be empty")
	require.NotEmpty(t, pub, "public key must not be empty")
	require.Contains(t, priv, "-----BEGIN OPENSSH PRIVATE KEY-----",
		"private key must be PEM-encoded")
	require.Contains(t, pub, "ssh-ed25519",
		"public key must be an ed25519 authorized_keys entry")
}

// TestGenerateHostKeyPairIsUnique verifies that two successive calls produce
// different key pairs (collision resistance).
func TestGenerateHostKeyPairIsUnique(t *testing.T) {
	priv1, pub1, err := internalssh.GenerateHostKeyPair()
	require.NoError(t, err)

	priv2, pub2, err := internalssh.GenerateHostKeyPair()
	require.NoError(t, err)

	require.NotEqual(t, priv1, priv2, "successive key pairs must differ (private)")
	require.NotEqual(t, pub1, pub2, "successive key pairs must differ (public)")
}
