//go:build integration

package docker_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"
	sshpkg "github.com/koudis/bootstrap-ai-coding/internal/ssh"
)

// ----------------------------------------------------------------------------
// 16.6 TestSSHHostKeyStableAcrossRebuild
// Validates: Req 13.3
// ----------------------------------------------------------------------------

func TestSSHHostKeyStableAcrossRebuild(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	hostKeyPriv, hostKeyPub, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err)

	_, userPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err)

	u, err := user.Current()
	require.NoError(t, err)
	uid, err := strconv.Atoi(u.Uid)
	require.NoError(t, err)
	gid, err := strconv.Atoi(u.Gid)
	require.NoError(t, err)

	info := &hostinfo.Info{
		Username: u.Username,
		HomeDir:  u.HomeDir,
		UID:      uid,
		GID:      gid,
	}

	projectDir := t.TempDir()
	dirName := filepath.Base(projectDir)
	containerName := constants.ContainerNamePrefix + sanitize(dirName)
	imageTag := containerName + ":latest"

	port, err := findFreePort()
	require.NoError(t, err)

	client, err := docker.NewClient()
	require.NoError(t, err)

	buildAndGetFingerprint := func() string {
		t.Helper()

		strategy := docker.UserStrategyCreate
		conflictingUser := ""
		conflictingImageUser, err := docker.FindConflictingUser(ctx, client, info.UID, info.GID)
		require.NoError(t, err, "checking base image for UID/GID conflicts")
		if conflictingImageUser != nil {
			strategy = docker.UserStrategyRename
			conflictingUser = conflictingImageUser.Username
		}

		builder := docker.NewBaseImageBuilder(
			info,
			strategy, conflictingUser,
			"",
		)

		// Build base image
		baseSpec := docker.ContainerSpec{
			Name:       containerName,
			ImageTag:   constants.BaseImageTag,
			Dockerfile: builder.Build(),
			Labels:     map[string]string{"bac.managed": "true"},
			HostInfo: info,
		}

		_, err = docker.BuildImage(ctx, client, baseSpec, false)
		require.NoError(t, err, "building base image")

		// Build instance image
		instanceBuilder := docker.NewInstanceImageBuilder(
			info,
			userPubKey,
			hostKeyPriv, hostKeyPub,
			2222, false,
		)
		instanceBuilder.Finalize()
		spec := docker.ContainerSpec{
			Name:       containerName,
			ImageTag:   imageTag,
			Dockerfile: instanceBuilder.Build(),
			Mounts: []docker.Mount{
				{HostPath: projectDir, ContainerPath: constants.WorkspaceMountPath},
			},
			SSHPort: port,
			Labels:  map[string]string{"bac.managed": "true"},
			HostInfo: info,
		}

		_, err = docker.BuildImage(ctx, client, spec, false)
		require.NoError(t, err, "building instance image")

		return hostKeyPub
	}

	fingerprint1 := buildAndGetFingerprint()
	fingerprint2 := buildAndGetFingerprint()

	require.Equal(t, fingerprint1, fingerprint2,
		"SSH host key fingerprint must be stable across rebuilds")

	t.Cleanup(func() {
		cleanCtx := context.Background()
		images, _ := docker.ListBACImages(cleanCtx, client)
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if tag == imageTag {
					_, _ = client.ImageRemove(cleanCtx, img.ID, forceRemoveOpts())
				}
			}
		}
	})
}

// ----------------------------------------------------------------------------
// 16.5 TestSSHPortPersistenceAcrossRestarts
// Validates: Req 12.2
// ----------------------------------------------------------------------------

func TestSSHPortPersistenceAcrossRestarts(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	containerName, sshPort, client, cleanup := startContainerFromSharedImage(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	originalPort := sshPort

	err := docker.StopContainer(ctx, client, containerName)
	require.NoError(t, err, "stopping container")

	err = docker.StartContainer(ctx, client, containerName)
	require.NoError(t, err, "restarting container")

	err = docker.WaitForSSH(ctx, "127.0.0.1", originalPort, 30*time.Second)
	require.NoError(t, err, "waiting for SSH after restart on original port %d", originalPort)

	addr := fmt.Sprintf("127.0.0.1:%d", originalPort)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	require.NoError(t, err, "expected SSH port %d to be reachable after restart", originalPort)
	conn.Close()
}

// ----------------------------------------------------------------------------
// 16.10 TestKnownHostsEntriesLifecycle
// Validates: Req 18.1–18.2, 18.7
// ----------------------------------------------------------------------------

func TestKnownHostsEntriesLifecycle(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	_, sshPort, _, cleanup := startContainerFromSharedImage(t)

	_, hostPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err)

	err = sshpkg.SyncKnownHosts(sshPort, hostPubKey, false)
	require.NoError(t, err, "SyncKnownHosts should succeed")

	khPath := filepath.Join(tempHome, ".ssh", "known_hosts")
	data, err := os.ReadFile(khPath)
	require.NoError(t, err, "known_hosts file should exist")
	content := string(data)

	localhostEntry := fmt.Sprintf("[localhost]:%d", sshPort)
	loopbackEntry := fmt.Sprintf("127.0.0.1:%d", sshPort)
	require.True(t, strings.Contains(content, localhostEntry),
		"known_hosts should contain [localhost]:%d entry", sshPort)
	require.True(t, strings.Contains(content, loopbackEntry),
		"known_hosts should contain 127.0.0.1:%d entry", sshPort)

	cleanup()

	err = sshpkg.RemoveKnownHostsEntries(sshPort)
	require.NoError(t, err, "RemoveKnownHostsEntries should succeed")

	data, err = os.ReadFile(khPath)
	require.NoError(t, err, "known_hosts file should still exist after removal")
	content = string(data)

	require.False(t, strings.Contains(content, localhostEntry),
		"known_hosts should NOT contain [localhost]:%d entry after removal", sshPort)
	require.False(t, strings.Contains(content, loopbackEntry),
		"known_hosts should NOT contain 127.0.0.1:%d entry after removal", sshPort)
}

// ----------------------------------------------------------------------------
// 16.11 TestSSHConfigEntryLifecycle
// Validates: Req 19.1–19.2, 19.7
// ----------------------------------------------------------------------------

func TestSSHConfigEntryLifecycle(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	containerName, sshPort, _, cleanup := startContainerFromSharedImage(t)

	info, err := hostinfo.Current()
	require.NoError(t, err)

	err = sshpkg.SyncSSHConfig(containerName, sshPort, info.Username, false)
	require.NoError(t, err, "SyncSSHConfig should succeed")

	cfgPath := filepath.Join(tempHome, ".ssh", "config")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err, "ssh config file should exist")
	content := string(data)

	hostLine := fmt.Sprintf("Host %s", containerName)
	portLine := fmt.Sprintf("Port %d", sshPort)
	userLine := fmt.Sprintf("User %s", info.Username)
	hostnameLine := "HostName localhost"

	require.True(t, strings.Contains(content, hostLine),
		"ssh config should contain 'Host %s'", containerName)
	require.True(t, strings.Contains(content, portLine),
		"ssh config should contain 'Port %d'", sshPort)
	require.True(t, strings.Contains(content, userLine),
		"ssh config should contain 'User %s'", info.Username)
	require.True(t, strings.Contains(content, hostnameLine),
		"ssh config should contain 'HostName localhost'")

	cleanup()

	err = sshpkg.RemoveSSHConfigEntry(containerName)
	require.NoError(t, err, "RemoveSSHConfigEntry should succeed")

	data, err = os.ReadFile(cfgPath)
	require.NoError(t, err, "ssh config file should still exist after removal")
	content = string(data)

	require.False(t, strings.Contains(content, hostLine),
		"ssh config should NOT contain 'Host %s' after removal", containerName)
}

// ----------------------------------------------------------------------------
// TestLoginShellLandsInWorkspace
// Validates: Req 27.1, 27.2
// ----------------------------------------------------------------------------

func TestLoginShellLandsInWorkspace(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	_, sshPort, _, cleanup := startContainerFromSharedImage(t)
	t.Cleanup(cleanup)

	info, err := hostinfo.Current()
	require.NoError(t, err, "getting host info")

	// Parse the user private key that was baked into the shared image.
	signer, err := gossh.ParsePrivateKey([]byte(sharedUserPrivKey))
	require.NoError(t, err, "parsing user private key for SSH auth")

	// Connect via SSH — the real path a user takes.
	config := &gossh.ClientConfig{
		User:            info.Username,
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	addr := fmt.Sprintf("127.0.0.1:%d", sshPort)

	// Retry SSH dial — sshd may need a moment after the TCP port becomes reachable.
	var client *gossh.Client
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		client, err = gossh.Dial("tcp", addr, config)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, err, "SSH dial to %s", addr)
	defer client.Close()

	session, err := client.NewSession()
	require.NoError(t, err, "creating SSH session")
	defer session.Close()

	var stdout bytes.Buffer
	session.Stdout = &stdout

	// Use "bash -l -c pwd" to simulate the login shell that SSH spawns for
	// interactive sessions. A plain "ssh host command" uses a non-login shell
	// and won't source /etc/profile.d/*.sh.
	err = session.Run("bash -l -c pwd")
	require.NoError(t, err, "running pwd over SSH login shell")

	// Profile scripts (e.g. dbus-keyring.sh) may emit output before pwd.
	// Extract only the last line which is the pwd result.
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	lastLine := lines[len(lines)-1]
	require.Equal(t, constants.WorkspaceMountPath, lastLine,
		"SSH login shell working directory should be %s", constants.WorkspaceMountPath)
}

// ----------------------------------------------------------------------------
// TestLoginShellFallsBackWhenWorkspaceMissing
// Validates: Req 27.3
// ----------------------------------------------------------------------------

func TestLoginShellFallsBackWhenWorkspaceMissing(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	buildSharedImage(t)

	ctx := context.Background()

	dirName := fmt.Sprintf("no-ws-%d", time.Now().UnixNano())
	containerName := constants.ContainerNamePrefix + sanitize(dirName)

	port, err := findFreePort()
	require.NoError(t, err, "finding free port")

	// Create container WITHOUT a /workspace mount — the directory won't exist.
	spec := docker.ContainerSpec{
		Name:           containerName,
		ImageTag:       sharedImageTag,
		Mounts:         []docker.Mount{}, // no /workspace
		SSHPort:        port,
		Labels:         map[string]string{"bac.managed": "true"},
		HostInfo:       sharedHostInfo,
		HostNetworkOff: true,
	}

	_, err = docker.CreateContainer(ctx, sharedClient, spec)
	require.NoError(t, err, "creating container without /workspace mount")

	t.Cleanup(func() {
		cleanCtx := context.Background()
		_ = docker.StopContainer(cleanCtx, sharedClient, containerName)
		_ = docker.RemoveContainer(cleanCtx, sharedClient, containerName)
	})

	err = docker.StartContainer(ctx, sharedClient, containerName)
	require.NoError(t, err, "starting container")

	err = docker.WaitForSSH(ctx, "127.0.0.1", port, 60*time.Second)
	require.NoError(t, err, "waiting for SSH to be ready")

	info, err := hostinfo.Current()
	require.NoError(t, err, "getting host info")

	signer, err := gossh.ParsePrivateKey([]byte(sharedUserPrivKey))
	require.NoError(t, err, "parsing user private key")

	config := &gossh.ClientConfig{
		User:            info.Username,
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)

	var sshClient *gossh.Client
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		sshClient, err = gossh.Dial("tcp", addr, config)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	require.NoError(t, err, "SSH dial to %s", addr)
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	require.NoError(t, err, "creating SSH session")
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// Login shell must succeed even though /workspace doesn't exist.
	err = session.Run("bash -l -c pwd")
	require.NoError(t, err, "login shell must not fail when /workspace is missing")

	// Working directory should fall back to home dir.
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	lastLine := lines[len(lines)-1]
	require.Equal(t, info.HomeDir, lastLine,
		"working directory should fall back to home when /workspace is missing")

	// No errors on stderr from the profile script.
	require.Empty(t, stderr.String(),
		"profile script must not emit errors when /workspace is missing")
}
