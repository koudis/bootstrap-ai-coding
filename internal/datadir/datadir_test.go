package datadir_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/datadir"
)

// Feature: bootstrap-ai-coding, Property 11: SSH host key is stable across rebuilds
func TestHostKeyStableAcrossRebuilds(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Override HOME so New() resolves ToolDataDirRoot to a temp directory.
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		// Draw arbitrary key content strings.
		priv := rapid.String().Draw(rt, "priv")
		pub := rapid.String().Draw(rt, "pub")

		dd, err := datadir.New("test-container")
		require.NoError(rt, err)

		// Write the key pair once.
		err = dd.WriteHostKey(priv, pub)
		require.NoError(rt, err)

		// Read it back 3 times and assert all reads return identical values.
		for i := 0; i < 3; i++ {
			gotPriv, gotPub, err := dd.ReadHostKey()
			require.NoError(rt, err, "read %d failed", i+1)
			require.Equal(rt, priv, gotPriv, "private key mismatch on read %d", i+1)
			require.Equal(rt, pub, gotPub, "public key mismatch on read %d", i+1)
		}
	})
}

// Feature: bootstrap-ai-coding, Property 21: Persisted port round-trips correctly
func TestPortRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Override HOME so New() resolves ToolDataDirRoot to a temp directory.
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		port := rapid.IntRange(1024, 65535).Draw(rt, "port")

		dd, err := datadir.New("test-container")
		require.NoError(rt, err)

		err = dd.WritePort(port)
		require.NoError(rt, err)

		got, err := dd.ReadPort()
		require.NoError(rt, err)
		require.Equal(rt, port, got, "ReadPort() should return the same value written by WritePort()")
	})
}

// Feature: bootstrap-ai-coding, Property 24: Tool_Data_Dir is created with constants.ToolDataDirPerm permissions
func TestNewCreatesDirectoryWithCorrectPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not applicable on Windows")
	}

	rapid.Check(t, func(rt *rapid.T) {
		// Override HOME so New() resolves ToolDataDirRoot to a temp directory.
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		containerName := rapid.StringMatching(`[a-z][a-z0-9-]*`).Draw(rt, "containerName")

		dd, err := datadir.New(containerName)
		require.NoError(rt, err)

		info, err := os.Stat(dd.Path())
		require.NoError(rt, err)

		// Assert the directory was created with the correct permissions.
		gotMode := info.Mode().Perm()
		require.Equal(rt, os.FileMode(constants.ToolDataDirPerm), gotMode,
			"directory %q has mode %04o, want %04o", dd.Path(), gotMode, constants.ToolDataDirPerm)
	})
}

// TestWritePortFilePermission asserts that WritePort writes the port file with
// constants.ToolDataFilePerm.
// Validates: Req 13.4, 15.3
func TestWritePortFilePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not applicable on Windows")
	}

	t.Setenv("HOME", t.TempDir())

	dd, err := datadir.New("test-container")
	require.NoError(t, err)

	err = dd.WritePort(2222)
	require.NoError(t, err)

	info, err := os.Stat(dd.Path() + "/port")
	require.NoError(t, err)
	require.Equal(t, os.FileMode(constants.ToolDataFilePerm), info.Mode().Perm(),
		"port file has mode %04o, want %04o", info.Mode().Perm(), constants.ToolDataFilePerm)
}

// TestWriteHostKeyFilePermission asserts that WriteHostKey writes both key files
// with constants.ToolDataFilePerm.
// Validates: Req 13.4, 15.3
func TestWriteHostKeyFilePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not applicable on Windows")
	}

	t.Setenv("HOME", t.TempDir())

	dd, err := datadir.New("test-container")
	require.NoError(t, err)

	err = dd.WriteHostKey("priv", "pub")
	require.NoError(t, err)

	privPath := dd.Path() + "/ssh_host_" + constants.SSHHostKeyType + "_key"
	pubPath := privPath + ".pub"

	for _, p := range []string{privPath, pubPath} {
		info, err := os.Stat(p)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(constants.ToolDataFilePerm), info.Mode().Perm(),
			"file %q has mode %04o, want %04o", p, info.Mode().Perm(), constants.ToolDataFilePerm)
	}
}

// TestWriteManifestFilePermission asserts that WriteManifest writes manifest.json
// with constants.ToolDataFilePerm.
// Validates: Req 13.4, 15.3
func TestWriteManifestFilePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not applicable on Windows")
	}

	t.Setenv("HOME", t.TempDir())

	dd, err := datadir.New("test-container")
	require.NoError(t, err)

	err = dd.WriteManifest([]string{"claude-code"})
	require.NoError(t, err)

	info, err := os.Stat(dd.Path() + "/manifest.json")
	require.NoError(t, err)
	require.Equal(t, os.FileMode(constants.ToolDataFilePerm), info.Mode().Perm(),
		"manifest.json has mode %04o, want %04o", info.Mode().Perm(), constants.ToolDataFilePerm)
}

// TestReadManifestRoundTrip verifies that WriteManifest followed by ReadManifest
// returns the same agent IDs.
func TestReadManifestRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dd, err := datadir.New("test-container")
	require.NoError(t, err)

	// Not yet written — should return nil, nil.
	ids, err := dd.ReadManifest()
	require.NoError(t, err)
	require.Nil(t, ids)

	// Write and read back.
	want := []string{"claude-code", "aider"}
	require.NoError(t, dd.WriteManifest(want))

	got, err := dd.ReadManifest()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// Feature: bootstrap-ai-coding, Property 22: Manifest round-trips correctly for any agent ID list
func TestPropertyManifestRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tmpHome := t.TempDir()
		t.Setenv("HOME", tmpHome)

		ids := rapid.SliceOfN(
			rapid.StringMatching(`[a-z][a-z0-9-]*`),
			1, 5,
		).Draw(rt, "ids")

		dd, err := datadir.New("test-container")
		require.NoError(rt, err)

		require.NoError(rt, dd.WriteManifest(ids))

		got, err := dd.ReadManifest()
		require.NoError(rt, err)
		require.Equal(rt, ids, got, "ReadManifest must return the same IDs written by WriteManifest")
	})
}

// TestPurgeRoot verifies that PurgeRoot removes the entire Tool_Data_Dir root.
func TestPurgeRoot(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Create a data dir so the root exists.
	_, err := datadir.New("test-container")
	require.NoError(t, err)

	require.NoError(t, datadir.PurgeRoot())

	// After purge, ListContainerNames should return nil (root gone).
	names, err := datadir.ListContainerNames()
	require.NoError(t, err)
	require.Nil(t, names)
}

// TestListContainerNames verifies that ListContainerNames returns the names of
// all subdirectories created under the Tool_Data_Dir root.
func TestListContainerNames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// No data dirs yet — should return nil without error.
	names, err := datadir.ListContainerNames()
	require.NoError(t, err)
	require.Nil(t, names)

	// Create two data dirs.
	_, err = datadir.New("bac-alpha")
	require.NoError(t, err)
	_, err = datadir.New("bac-beta")
	require.NoError(t, err)

	names, err = datadir.ListContainerNames()
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"bac-alpha", "bac-beta"}, names)
}

// TestReadPortCorruptContent verifies that ReadPort returns an error when the
// port file contains non-numeric content.
func TestReadPortCorruptContent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dd, err := datadir.New("test-container")
	require.NoError(t, err)

	// Write garbage into the port file.
	require.NoError(t, os.WriteFile(filepath.Join(dd.Path(), "port"), []byte("not-a-number"), constants.ToolDataFilePerm))

	_, err = dd.ReadPort()
	require.Error(t, err, "ReadPort must error on non-numeric content")
}

// TestReadHostKeyMissingPubKey verifies that ReadHostKey returns an error when
// the private key file exists but the public key file is absent.
func TestReadHostKeyMissingPubKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dd, err := datadir.New("test-container")
	require.NoError(t, err)

	// Write only the private key file; leave the public key absent.
	privPath := filepath.Join(dd.Path(), "ssh_host_"+constants.SSHHostKeyType+"_key")
	require.NoError(t, os.WriteFile(privPath, []byte("fake-priv"), constants.ToolDataFilePerm))

	_, _, err = dd.ReadHostKey()
	require.Error(t, err, "ReadHostKey must error when public key file is missing")
}

// TestReadManifestCorruptJSON verifies that ReadManifest returns an error when
// the manifest file contains invalid JSON.
func TestReadManifestCorruptJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dd, err := datadir.New("test-container")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dd.Path(), "manifest.json"), []byte("{not-json}"), constants.ToolDataFilePerm))

	_, err = dd.ReadManifest()
	require.Error(t, err, "ReadManifest must error on invalid JSON")
}

// TestHostNetworkOffRoundTrip verifies that WriteHostNetworkOff followed by
// ReadHostNetworkOff returns the same boolean value.
func TestHostNetworkOffRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dd, err := datadir.New("test-container")
	require.NoError(t, err)

	// Not yet written — should return false, nil (default: host network ON).
	got, err := dd.ReadHostNetworkOff()
	require.NoError(t, err)
	require.False(t, got)

	// Write true and read back.
	require.NoError(t, dd.WriteHostNetworkOff(true))
	got, err = dd.ReadHostNetworkOff()
	require.NoError(t, err)
	require.True(t, got)

	// Write false and read back.
	require.NoError(t, dd.WriteHostNetworkOff(false))
	got, err = dd.ReadHostNetworkOff()
	require.NoError(t, err)
	require.False(t, got)
}

// TestWriteHostNetworkOffFilePermission asserts that WriteHostNetworkOff writes
// the file with constants.ToolDataFilePerm.
func TestWriteHostNetworkOffFilePermission(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not applicable on Windows")
	}

	t.Setenv("HOME", t.TempDir())

	dd, err := datadir.New("test-container")
	require.NoError(t, err)

	require.NoError(t, dd.WriteHostNetworkOff(true))

	info, err := os.Stat(filepath.Join(dd.Path(), "host_network_off"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(constants.ToolDataFilePerm), info.Mode().Perm(),
		"host_network_off file has mode %04o, want %04o", info.Mode().Perm(), constants.ToolDataFilePerm)
}

// TestReadHostNetworkOffCorruptContent verifies that ReadHostNetworkOff returns
// an error when the file contains non-boolean content.
func TestReadHostNetworkOffCorruptContent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dd, err := datadir.New("test-container")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(dd.Path(), "host_network_off"), []byte("not-a-bool"), constants.ToolDataFilePerm))

	_, err = dd.ReadHostNetworkOff()
	require.Error(t, err, "ReadHostNetworkOff must error on non-boolean content")
}

// TestExpandHomeNoTilde verifies that expandHome (via New) handles paths
// without a tilde prefix correctly. We exercise this indirectly by setting
// HOME to an absolute path and confirming New succeeds.
func TestNewWithAbsoluteHome(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	dd, err := datadir.New("bac-test")
	require.NoError(t, err)
	require.NotEmpty(t, dd.Path())
	require.Contains(t, dd.Path(), tmpHome)
}
