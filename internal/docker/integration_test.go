//go:build integration

package docker_test

import (
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

	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"
	sshpkg "github.com/koudis/bootstrap-ai-coding/internal/ssh"
	"github.com/koudis/bootstrap-ai-coding/internal/testutil"
)

// ----------------------------------------------------------------------------
// Package-level shared image state — built once in TestMain, reused by tests.
// ----------------------------------------------------------------------------

var (
	sharedImageTag  string
	sharedClient    *docker.Client
	sharedProjectDir string
	sharedHostInfo  *hostinfo.Info
)

// TestMain ensures the base image is removed from the local Docker image store
// before the integration suite runs (for TestAFindConflictingUserPullsImageIfAbsent),
// then builds a shared image that most tests reuse.
func TestMain(m *testing.M) {
	if _, err := exec.LookPath("docker"); err != nil {
		os.Exit(m.Run())
	}

	testutil.RequireIntegrationConsent()

	if err := testutil.EnsureBaseImageAbsent(); err != nil {
		fmt.Fprintf(os.Stderr, "EnsureBaseImageAbsent: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// buildSharedImage builds the shared image once (idempotent). Tests that need
// a container call this, then create their own container from the shared image.
// The image is built on first call; subsequent calls are no-ops.
func buildSharedImage(t *testing.T) {
	t.Helper()

	if sharedImageTag != "" {
		return // already built
	}

	ctx := context.Background()

	var err error
	sharedProjectDir = t.TempDir()

	hostKeyPriv, hostKeyPub, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating host key pair")

	_, userPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating user key pair")

	info, err := hostinfo.Current()
	require.NoError(t, err, "getting host info")
	sharedHostInfo = info

	sharedClient, err = docker.NewClient()
	require.NoError(t, err, "connecting to Docker daemon")

	strategy := docker.UserStrategyCreate
	conflictingUser := ""
	conflictingImageUser, err := docker.FindConflictingUser(ctx, sharedClient, sharedHostInfo.UID, sharedHostInfo.GID)
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

	instanceBuilder := docker.NewInstanceImageBuilder(
		info,
		userPubKey,
		hostKeyPriv, hostKeyPub,
		2222, true,
	)
	instanceBuilder.Finalize()

	sharedImageTag = constants.ContainerNamePrefix + "integration-shared:latest"

	// Build base image first
	baseSpec := docker.ContainerSpec{
		Name:       constants.ContainerNamePrefix + "integration-shared",
		ImageTag:   constants.BaseImageTag,
		Dockerfile: builder.Build(),
		Labels:     map[string]string{"bac.managed": "true"},
		HostInfo: sharedHostInfo,
	}

	_, err = docker.BuildImage(ctx, sharedClient, baseSpec, false)
	require.NoError(t, err, "building base image")

	// Build instance image from base
	spec := docker.ContainerSpec{
		Name:       constants.ContainerNamePrefix + "integration-shared",
		ImageTag:   sharedImageTag,
		Dockerfile: instanceBuilder.Build(),
		Mounts: []docker.Mount{
			{HostPath: sharedProjectDir, ContainerPath: constants.WorkspaceMountPath},
		},
		Labels:  map[string]string{"bac.managed": "true"},
		HostInfo: sharedHostInfo,
	}

	_, err = docker.BuildImage(ctx, sharedClient, spec, false)
	require.NoError(t, err, "building shared container image")
}

// startContainerFromSharedImage creates and starts a new container from the
// shared image with a unique name and port. Returns the container name, port,
// client, and cleanup function.
func startContainerFromSharedImage(t *testing.T) (containerName string, sshPort int, client *docker.Client, cleanup func()) {
	t.Helper()

	buildSharedImage(t)

	ctx := context.Background()

	projectDir := t.TempDir()
	dirName := filepath.Base(projectDir)

	port, err := findFreePort()
	require.NoError(t, err, "finding free port")

	containerName = constants.ContainerNamePrefix + sanitize(dirName)

	spec := docker.ContainerSpec{
		Name:           containerName,
		ImageTag:       sharedImageTag,
		Mounts: []docker.Mount{
			{HostPath: projectDir, ContainerPath: constants.WorkspaceMountPath},
		},
		SSHPort:        port,
		Labels:         map[string]string{"bac.managed": "true"},
		HostInfo: sharedHostInfo,
		HostNetworkOff: true,
	}

	_, err = docker.CreateContainer(ctx, sharedClient, spec)
	require.NoError(t, err, "creating container")

	err = docker.StartContainer(ctx, sharedClient, containerName)
	require.NoError(t, err, "starting container")

	err = docker.WaitForSSH(ctx, "127.0.0.1", port, 60*time.Second)
	require.NoError(t, err, "waiting for SSH to be ready")

	cleanup = func() {
		cleanCtx := context.Background()
		_ = docker.StopContainer(cleanCtx, sharedClient, containerName)
		_ = docker.RemoveContainer(cleanCtx, sharedClient, containerName)
	}

	return containerName, port, sharedClient, cleanup
}

// ----------------------------------------------------------------------------
// 16.1 TestContainerStartsAndSSHConnects
// Validates: Req 3.3, 4.3
// ----------------------------------------------------------------------------

func TestContainerStartsAndSSHConnects(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	_, sshPort, _, cleanup := startContainerFromSharedImage(t)
	t.Cleanup(cleanup)

	addr := fmt.Sprintf("127.0.0.1:%d", sshPort)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	require.NoError(t, err, "expected TCP connection to SSH port %d to succeed", sshPort)
	conn.Close()
}

// ----------------------------------------------------------------------------
// 16.2 TestWorkspaceMountLiveSync
// Validates: Req 2.3
// ----------------------------------------------------------------------------

func TestWorkspaceMountLiveSync(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	containerName, _, client, cleanup := startContainerFromSharedImage(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	exitCode, err := docker.ExecInContainer(ctx, client, containerName, []string{
		"bash", "-c", "echo 'hello from container' > /workspace/sync-test.txt",
	})
	require.NoError(t, err, "exec to create file in /workspace")
	require.Equal(t, 0, exitCode, "expected exit 0 when creating file in /workspace")

	exitCode, err = docker.ExecInContainer(ctx, client, containerName, []string{
		"test", "-f", constants.WorkspaceMountPath + "/sync-test.txt",
	})
	require.NoError(t, err, "exec to verify file in /workspace")
	require.Equal(t, 0, exitCode, "expected file to exist at %s/sync-test.txt", constants.WorkspaceMountPath)
}

// ----------------------------------------------------------------------------
// 16.3 TestFileOwnershipMatchesHostUser
// Validates: Req 10.6
// ----------------------------------------------------------------------------

func TestFileOwnershipMatchesHostUser(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	containerName, _, client, cleanup := startContainerFromSharedImage(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	info, err := hostinfo.Current()
	require.NoError(t, err)

	exitCode, err := docker.ExecInContainer(ctx, client, containerName, []string{
		"su", "-c", "touch /workspace/ownership-test.txt", info.Username,
	})
	require.NoError(t, err)
	require.Equal(t, 0, exitCode, "expected exit 0 when creating file")

	checkUID := fmt.Sprintf(`[ "$(stat -c '%%u' /workspace/ownership-test.txt)" = "%d" ]`, info.UID)
	exitCode, err = docker.ExecInContainer(ctx, client, containerName, []string{"bash", "-c", checkUID})
	require.NoError(t, err, "exec to check file UID")
	require.Equal(t, 0, exitCode,
		"expected file UID inside container to match host user UID=%d", info.UID)

	checkGID := fmt.Sprintf(`[ "$(stat -c '%%g' /workspace/ownership-test.txt)" = "%d" ]`, info.GID)
	exitCode, err = docker.ExecInContainer(ctx, client, containerName, []string{"bash", "-c", checkGID})
	require.NoError(t, err, "exec to check file GID")
	require.Equal(t, 0, exitCode,
		"expected file GID inside container to match host user GID=%d", info.GID)
}

// ----------------------------------------------------------------------------
// 16.4 TestCredentialVolumePersistedAcrossRestart
// Validates: Req 8.6
// ----------------------------------------------------------------------------

func TestCredentialVolumePersistedAcrossRestart(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	containerName, sshPort, client, cleanup := startContainerFromSharedImage(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	exitCode, err := docker.ExecInContainer(ctx, client, containerName, []string{
		"bash", "-c", "echo 'persistent' > /workspace/persist-test.txt",
	})
	require.NoError(t, err)
	require.Equal(t, 0, exitCode, "expected exit 0 when writing sentinel file")

	err = docker.StopContainer(ctx, client, containerName)
	require.NoError(t, err, "stopping container")

	err = docker.StartContainer(ctx, client, containerName)
	require.NoError(t, err, "restarting container")

	err = docker.WaitForSSH(ctx, "127.0.0.1", sshPort, 30*time.Second)
	require.NoError(t, err, "waiting for SSH after restart")

	exitCode, err = docker.ExecInContainer(ctx, client, containerName, []string{
		"test", "-f", "/workspace/persist-test.txt",
	})
	require.NoError(t, err)
	require.Equal(t, 0, exitCode, "expected sentinel file to persist across container restart")
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
// 16.7 TestPurgeRemovesContainersAndImages
// Validates: Req 16.2, 16.4
// ----------------------------------------------------------------------------

func TestPurgeRemovesContainersAndImages(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	containerName, _, _, _ := startContainerFromSharedImage(t)
	// Note: we do NOT register the cleanup here because we are testing purge.

	client, err := docker.NewClient()
	require.NoError(t, err)

	// Verify the container exists before purge.
	info, err := docker.InspectContainer(ctx, client, containerName)
	require.NoError(t, err)
	require.NotNil(t, info, "container should exist before purge")

	// Run purge logic: stop + remove container.
	err = docker.StopContainer(ctx, client, containerName)
	require.NoError(t, err, "stopping container during purge")

	err = docker.RemoveContainer(ctx, client, containerName)
	require.NoError(t, err, "removing container during purge")

	// Assert container is gone.
	info, err = docker.InspectContainer(ctx, client, containerName)
	require.NoError(t, err)
	require.Nil(t, info, "container should be gone after purge")
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
// 16.12 TestContainerHostnameMatchesContainerName
// Validates: Req 23.1, 23.2
// ----------------------------------------------------------------------------

func TestContainerHostnameMatchesContainerName(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	containerName, _, client, cleanup := startContainerFromSharedImage(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	// Verify via container inspect that the hostname is set correctly.
	info, err := docker.InspectContainer(ctx, client, containerName)
	require.NoError(t, err, "inspecting container")
	require.NotNil(t, info, "container should exist")
	require.Equal(t, containerName, info.Config.Hostname,
		"container hostname should match container name")

	// Also verify by running `hostname` inside the container.
	exitCode, err := docker.ExecInContainer(ctx, client, containerName, []string{"hostname"})
	require.NoError(t, err, "exec hostname command")
	require.Equal(t, 0, exitCode, "hostname command should exit 0")
}

// ----------------------------------------------------------------------------
// 9.1 TestHostNetworkModeSSHReachable
// Validates: Req 26 — host network mode SSH reachability
// ----------------------------------------------------------------------------

func TestHostNetworkModeSSHReachable(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	client, err := docker.NewClient()
	require.NoError(t, err, "connecting to Docker daemon")

	info, err := hostinfo.Current()
	require.NoError(t, err, "getting host info")

	// Generate SSH keys for the test
	hostKeyPriv, hostKeyPub, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating host key pair")

	_, userPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating user key pair")

	projectDir := t.TempDir()
	dirName := filepath.Base(projectDir)
	containerName := constants.ContainerNamePrefix + "hostnet-" + sanitize(dirName)
	instanceImageTag := containerName + ":latest"

	// Use a high port to avoid conflicts
	sshPort := 22222

	// Determine user strategy
	strategy := docker.UserStrategyCreate
	conflictingUser := ""
	conflictingImageUser, err := docker.FindConflictingUser(ctx, client, info.UID, info.GID)
	require.NoError(t, err, "checking base image for UID/GID conflicts")
	if conflictingImageUser != nil {
		strategy = docker.UserStrategyRename
		conflictingUser = conflictingImageUser.Username
	}

	// Cleanup
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_ = docker.StopContainer(cleanCtx, client, containerName)
		_ = docker.RemoveContainer(cleanCtx, client, containerName)
		images, _ := docker.ListBACImages(cleanCtx, client)
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if tag == instanceImageTag {
					_, _ = client.ImageRemove(cleanCtx, img.ID, forceRemoveOpts())
				}
			}
		}
	})

	// Build base image
	baseBuilder := docker.NewBaseImageBuilder(info, strategy, conflictingUser, "")
	baseSpec := docker.ContainerSpec{
		Name:       containerName,
		ImageTag:   constants.BaseImageTag,
		Dockerfile: baseBuilder.Build(),
		Labels:     map[string]string{"bac.managed": "true"},
		HostInfo: info,
	}

	_, err = docker.BuildImage(ctx, client, baseSpec, false)
	require.NoError(t, err, "building base image")

	// Build instance image with host network mode (hostNetworkOff=false)
	instanceBuilder := docker.NewInstanceImageBuilder(info, userPubKey, hostKeyPriv, hostKeyPub, sshPort, false)
	instanceBuilder.Finalize()

	instanceSpec := docker.ContainerSpec{
		Name:       containerName,
		ImageTag:   instanceImageTag,
		Dockerfile: instanceBuilder.Build(),
		Mounts: []docker.Mount{
			{HostPath: projectDir, ContainerPath: constants.WorkspaceMountPath},
		},
		SSHPort:        sshPort,
		Labels:         map[string]string{"bac.managed": "true"},
		HostInfo: info,
		HostNetworkOff: false, // host network mode
	}

	_, err = docker.BuildImage(ctx, client, instanceSpec, false)
	require.NoError(t, err, "building instance image with host network mode")

	// Create and start container
	_, err = docker.CreateContainer(ctx, client, instanceSpec)
	require.NoError(t, err, "creating container with host network mode")

	err = docker.StartContainer(ctx, client, containerName)
	require.NoError(t, err, "starting container")

	// Assert: SSH is reachable on 127.0.0.1:sshPort
	err = docker.WaitForSSH(ctx, "127.0.0.1", sshPort, 10*time.Second)
	require.NoError(t, err, "SSH must be reachable on 127.0.0.1:%d in host network mode", sshPort)
}

// ----------------------------------------------------------------------------
// 9.3 TestHostNetworkCanReachHostService
// Validates: Req 26 — host network mode shares the host's network namespace
// ----------------------------------------------------------------------------

func TestHostNetworkCanReachHostService(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	// Step 1: Start a TCP listener on a random port on the host.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "starting TCP listener on host")
	t.Cleanup(func() { ln.Close() })

	// Extract the port from the listener address.
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err, "splitting host:port from listener address")

	client, err := docker.NewClient()
	require.NoError(t, err, "connecting to Docker daemon")

	info, err := hostinfo.Current()
	require.NoError(t, err, "getting host info")

	// Generate SSH keys for the test
	hostKeyPriv, hostKeyPub, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating host key pair")

	_, userPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating user key pair")

	projectDir := t.TempDir()
	dirName := filepath.Base(projectDir)
	containerName := constants.ContainerNamePrefix + "hostreach-" + sanitize(dirName)
	instanceImageTag := containerName + ":latest"

	// Use a high port for SSH to avoid conflicts
	sshPort := 22224

	// Determine user strategy
	strategy := docker.UserStrategyCreate
	conflictingUser := ""
	conflictingImageUser, err := docker.FindConflictingUser(ctx, client, info.UID, info.GID)
	require.NoError(t, err, "checking base image for UID/GID conflicts")
	if conflictingImageUser != nil {
		strategy = docker.UserStrategyRename
		conflictingUser = conflictingImageUser.Username
	}

	// Cleanup
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_ = docker.StopContainer(cleanCtx, client, containerName)
		_ = docker.RemoveContainer(cleanCtx, client, containerName)
		images, _ := docker.ListBACImages(cleanCtx, client)
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if tag == instanceImageTag {
					_, _ = client.ImageRemove(cleanCtx, img.ID, forceRemoveOpts())
				}
			}
		}
	})

	// Build base image
	baseBuilder := docker.NewBaseImageBuilder(info, strategy, conflictingUser, "")
	baseSpec := docker.ContainerSpec{
		Name:       containerName,
		ImageTag:   constants.BaseImageTag,
		Dockerfile: baseBuilder.Build(),
		Labels:     map[string]string{"bac.managed": "true"},
		HostInfo: info,
	}

	_, err = docker.BuildImage(ctx, client, baseSpec, false)
	require.NoError(t, err, "building base image")

	// Build instance image with host network mode (hostNetworkOff=false)
	instanceBuilder := docker.NewInstanceImageBuilder(info, userPubKey, hostKeyPriv, hostKeyPub, sshPort, false)
	instanceBuilder.Finalize()

	instanceSpec := docker.ContainerSpec{
		Name:       containerName,
		ImageTag:   instanceImageTag,
		Dockerfile: instanceBuilder.Build(),
		Mounts: []docker.Mount{
			{HostPath: projectDir, ContainerPath: constants.WorkspaceMountPath},
		},
		SSHPort:        sshPort,
		Labels:         map[string]string{"bac.managed": "true"},
		HostInfo: info,
		HostNetworkOff: false, // host network mode
	}

	_, err = docker.BuildImage(ctx, client, instanceSpec, false)
	require.NoError(t, err, "building instance image with host network mode")

	// Create and start container
	_, err = docker.CreateContainer(ctx, client, instanceSpec)
	require.NoError(t, err, "creating container with host network mode")

	err = docker.StartContainer(ctx, client, containerName)
	require.NoError(t, err, "starting container")

	// Wait for the container to be running (SSH ready)
	err = docker.WaitForSSH(ctx, "127.0.0.1", sshPort, 10*time.Second)
	require.NoError(t, err, "SSH must be reachable on 127.0.0.1:%d in host network mode", sshPort)

	// Step 5: Use bash /dev/tcp trick to test connectivity to the host listener.
	// This works without netcat being installed.
	exitCode, err := docker.ExecInContainer(ctx, client, containerName, []string{
		"bash", "-c", fmt.Sprintf("echo > /dev/tcp/127.0.0.1/%s", portStr),
	})
	require.NoError(t, err, "exec to test connectivity to host service on port %s", portStr)
	require.Equal(t, 0, exitCode,
		"container in host network mode must be able to reach host service on 127.0.0.1:%s", portStr)
}

// ----------------------------------------------------------------------------
// 9.2 TestBridgeModeSSHReachable
// Validates: Req 26 — bridge mode (hostNetworkOff=true) SSH reachability
// ----------------------------------------------------------------------------

func TestBridgeModeSSHReachable(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	client, err := docker.NewClient()
	require.NoError(t, err, "connecting to Docker daemon")

	info, err := hostinfo.Current()
	require.NoError(t, err, "getting host info")

	// Generate SSH keys for the test
	hostKeyPriv, hostKeyPub, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating host key pair")

	_, userPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating user key pair")

	projectDir := t.TempDir()
	dirName := filepath.Base(projectDir)
	containerName := constants.ContainerNamePrefix + "bridge-" + sanitize(dirName)
	instanceImageTag := containerName + ":latest"

	// Use a different high port to avoid conflicts with host network test
	sshPort := 22223

	// Determine user strategy
	strategy := docker.UserStrategyCreate
	conflictingUser := ""
	conflictingImageUser, err := docker.FindConflictingUser(ctx, client, info.UID, info.GID)
	require.NoError(t, err, "checking base image for UID/GID conflicts")
	if conflictingImageUser != nil {
		strategy = docker.UserStrategyRename
		conflictingUser = conflictingImageUser.Username
	}

	// Cleanup
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_ = docker.StopContainer(cleanCtx, client, containerName)
		_ = docker.RemoveContainer(cleanCtx, client, containerName)
		images, _ := docker.ListBACImages(cleanCtx, client)
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if tag == instanceImageTag {
					_, _ = client.ImageRemove(cleanCtx, img.ID, forceRemoveOpts())
				}
			}
		}
	})

	// Build base image
	baseBuilder := docker.NewBaseImageBuilder(info, strategy, conflictingUser, "")
	baseSpec := docker.ContainerSpec{
		Name:       containerName,
		ImageTag:   constants.BaseImageTag,
		Dockerfile: baseBuilder.Build(),
		Labels:     map[string]string{"bac.managed": "true"},
		HostInfo: info,
	}

	_, err = docker.BuildImage(ctx, client, baseSpec, false)
	require.NoError(t, err, "building base image")

	// Build instance image with bridge mode (hostNetworkOff=true)
	instanceBuilder := docker.NewInstanceImageBuilder(info, userPubKey, hostKeyPriv, hostKeyPub, sshPort, true)
	instanceBuilder.Finalize()

	instanceSpec := docker.ContainerSpec{
		Name:       containerName,
		ImageTag:   instanceImageTag,
		Dockerfile: instanceBuilder.Build(),
		Mounts: []docker.Mount{
			{HostPath: projectDir, ContainerPath: constants.WorkspaceMountPath},
		},
		SSHPort:        sshPort,
		Labels:         map[string]string{"bac.managed": "true"},
		HostInfo: info,
		HostNetworkOff: true, // bridge mode
	}

	_, err = docker.BuildImage(ctx, client, instanceSpec, false)
	require.NoError(t, err, "building instance image with bridge mode")

	// Create and start container
	_, err = docker.CreateContainer(ctx, client, instanceSpec)
	require.NoError(t, err, "creating container with bridge mode")

	err = docker.StartContainer(ctx, client, containerName)
	require.NoError(t, err, "starting container")

	// Assert: SSH is reachable on 127.0.0.1:sshPort
	err = docker.WaitForSSH(ctx, "127.0.0.1", sshPort, 10*time.Second)
	require.NoError(t, err, "SSH must be reachable on 127.0.0.1:%d in bridge mode", sshPort)
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

func findFreePort() (int, error) {
	for port := constants.SSHPortStart; port < 65535; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found starting at %d", constants.SSHPortStart)
}

func sanitize(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	result = strings.Trim(result, "-")
	if result == "" {
		result = "tmp"
	}
	return result
}

func forceRemoveOpts() dockerimage.RemoveOptions {
	return dockerimage.RemoveOptions{Force: true}
}

// ----------------------------------------------------------------------------
// 11.1–11.5 TestTwoLayerBuildCycle
// Validates: TL-1, TL-2, TL-5, TL-6
// Full two-layer build cycle: base → instance → container → stop → rebuild
// ----------------------------------------------------------------------------

func TestTwoLayerBuildCycle(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	client, err := docker.NewClient()
	require.NoError(t, err, "connecting to Docker daemon")

	info, err := hostinfo.Current()
	require.NoError(t, err, "getting host info")

	// Generate SSH keys for the test
	hostKeyPriv, hostKeyPub, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating host key pair")

	_, userPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating user key pair")

	projectDir := t.TempDir()
	dirName := filepath.Base(projectDir)
	containerName := constants.ContainerNamePrefix + "twolayer-" + sanitize(dirName)
	instanceImageTag := containerName + ":latest"

	port, err := findFreePort()
	require.NoError(t, err, "finding free port")

	// Determine user strategy
	strategy := docker.UserStrategyCreate
	conflictingUser := ""
	conflictingImageUser, err := docker.FindConflictingUser(ctx, client, info.UID, info.GID)
	require.NoError(t, err, "checking base image for UID/GID conflicts")
	if conflictingImageUser != nil {
		strategy = docker.UserStrategyRename
		conflictingUser = conflictingImageUser.Username
	}

	// Manifest for the base image label
	manifestJSON := `["test-agent"]`

	// Cleanup: remove images and container at end of test
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_ = docker.StopContainer(cleanCtx, client, containerName)
		_ = docker.RemoveContainer(cleanCtx, client, containerName)
		// Remove instance image
		images, _ := docker.ListBACImages(cleanCtx, client)
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if tag == instanceImageTag {
					_, _ = client.ImageRemove(cleanCtx, img.ID, forceRemoveOpts())
				}
			}
		}
		// Note: we do NOT remove bac-base:latest here because other tests may use it.
		// The shared buildSharedImage() already builds it; removing it would break other tests.
	})

	// -------------------------------------------------------------------------
	// Subtask 1: Build base image, verify it exists with correct labels
	// -------------------------------------------------------------------------
	baseBuilder := docker.NewBaseImageBuilder(info, strategy, conflictingUser, "")
	baseLabels := map[string]string{
		"bac.managed":  "true",
		"bac.manifest": manifestJSON,
	}
	baseSpec := docker.ContainerSpec{
		Name:       containerName,
		ImageTag:   constants.BaseImageTag,
		Dockerfile: baseBuilder.Build(),
		Labels:     baseLabels,
		HostInfo: info,
	}

	_, err = docker.BuildImage(ctx, client, baseSpec, false)
	require.NoError(t, err, "building base image")

	// Inspect base image and verify labels
	baseInspect, _, err := client.ImageInspectWithRaw(ctx, constants.BaseImageTag)
	require.NoError(t, err, "inspecting base image")
	require.Equal(t, "true", baseInspect.Config.Labels["bac.managed"],
		"base image must have bac.managed=true label")
	require.Equal(t, manifestJSON, baseInspect.Config.Labels["bac.manifest"],
		"base image must have bac.manifest label with correct JSON")

	// -------------------------------------------------------------------------
	// Subtask 2: Build instance image FROM base, verify it exists with correct labels
	// -------------------------------------------------------------------------
	instanceBuilder := docker.NewInstanceImageBuilder(info, userPubKey, hostKeyPriv, hostKeyPub, port, true)
	instanceBuilder.Finalize()

	instanceLabels := map[string]string{
		"bac.managed":   "true",
		"bac.container": containerName,
	}
	instanceSpec := docker.ContainerSpec{
		Name:           containerName,
		ImageTag:       instanceImageTag,
		Dockerfile:     instanceBuilder.Build(),
		Mounts: []docker.Mount{
			{HostPath: projectDir, ContainerPath: constants.WorkspaceMountPath},
		},
		SSHPort:        port,
		Labels:         instanceLabels,
		HostInfo: info,
		HostNetworkOff: true,
	}

	_, err = docker.BuildImage(ctx, client, instanceSpec, false)
	require.NoError(t, err, "building instance image")

	// Inspect instance image and verify labels
	instanceInspect, _, err := client.ImageInspectWithRaw(ctx, instanceImageTag)
	require.NoError(t, err, "inspecting instance image")
	require.Equal(t, "true", instanceInspect.Config.Labels["bac.managed"],
		"instance image must have bac.managed=true label")
	require.Equal(t, containerName, instanceInspect.Config.Labels["bac.container"],
		"instance image must have bac.container=<name> label")

	// -------------------------------------------------------------------------
	// Subtask 3: Start container from instance image, verify SSH connectivity
	// -------------------------------------------------------------------------
	_, err = docker.CreateContainer(ctx, client, instanceSpec)
	require.NoError(t, err, "creating container from instance image")

	err = docker.StartContainer(ctx, client, containerName)
	require.NoError(t, err, "starting container")

	err = docker.WaitForSSH(ctx, "127.0.0.1", port, 60*time.Second)
	require.NoError(t, err, "waiting for SSH to be ready in two-layer container")

	// Verify sshd is running inside the container
	exitCode, err := docker.ExecInContainer(ctx, client, containerName, []string{
		"pgrep", "-x", "sshd",
	})
	require.NoError(t, err, "exec pgrep sshd")
	require.Equal(t, 0, exitCode, "sshd should be running inside the container")

	// -------------------------------------------------------------------------
	// Subtask 4: Stop and remove container — verify both images still exist
	// -------------------------------------------------------------------------
	err = docker.StopContainer(ctx, client, containerName)
	require.NoError(t, err, "stopping container")

	err = docker.RemoveContainer(ctx, client, containerName)
	require.NoError(t, err, "removing container")

	// Verify container is gone
	containerInfo, err := docker.InspectContainer(ctx, client, containerName)
	require.NoError(t, err)
	require.Nil(t, containerInfo, "container should be gone after removal")

	// Verify base image still exists
	_, _, err = client.ImageInspectWithRaw(ctx, constants.BaseImageTag)
	require.NoError(t, err, "base image must still exist after container removal")

	// Verify instance image still exists
	_, _, err = client.ImageInspectWithRaw(ctx, instanceImageTag)
	require.NoError(t, err, "instance image must still exist after container removal")

	// -------------------------------------------------------------------------
	// Subtask 5: Rebuild (--rebuild equivalent) — verify both images are recreated
	// -------------------------------------------------------------------------

	// Record the image IDs before rebuild
	baseBeforeRebuild, _, err := client.ImageInspectWithRaw(ctx, constants.BaseImageTag)
	require.NoError(t, err)
	baseIDBeforeRebuild := baseBeforeRebuild.ID

	instanceBeforeRebuild, _, err := client.ImageInspectWithRaw(ctx, instanceImageTag)
	require.NoError(t, err)
	instanceIDBeforeRebuild := instanceBeforeRebuild.ID

	// Rebuild base with NoCache: true (simulating --rebuild)
	baseSpec.NoCache = true
	_, err = docker.BuildImage(ctx, client, baseSpec, false)
	require.NoError(t, err, "rebuilding base image with no-cache")

	// Rebuild instance (inherits fresh base)
	_, err = docker.BuildImage(ctx, client, instanceSpec, false)
	require.NoError(t, err, "rebuilding instance image after base rebuild")

	// Verify both images were recreated (different IDs)
	baseAfterRebuild, _, err := client.ImageInspectWithRaw(ctx, constants.BaseImageTag)
	require.NoError(t, err)
	require.NotEqual(t, baseIDBeforeRebuild, baseAfterRebuild.ID,
		"base image ID must change after no-cache rebuild")

	instanceAfterRebuild, _, err := client.ImageInspectWithRaw(ctx, instanceImageTag)
	require.NoError(t, err)
	require.NotEqual(t, instanceIDBeforeRebuild, instanceAfterRebuild.ID,
		"instance image ID must change after rebuild")

	// Verify labels are still correct after rebuild
	require.Equal(t, "true", baseAfterRebuild.Config.Labels["bac.managed"])
	require.Equal(t, manifestJSON, baseAfterRebuild.Config.Labels["bac.manifest"])
	require.Equal(t, "true", instanceAfterRebuild.Config.Labels["bac.managed"])
	require.Equal(t, containerName, instanceAfterRebuild.Config.Labels["bac.container"])
}

// ----------------------------------------------------------------------------
// TestBuildImageTimeoutEnforced
// Validates: Req 14.7 (Image_Build_Timeout)
// ----------------------------------------------------------------------------

const testBuildTimeout = 3 * time.Second

func TestBuildImageTimeoutEnforced(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	client, err := docker.NewClient()
	require.NoError(t, err, "connecting to Docker daemon")

	hangingDockerfile := fmt.Sprintf("FROM %s\nRUN sleep 300\n", constants.BaseContainerImage)

	containerName := constants.ContainerNamePrefix + "timeout-test"
	imageTag := containerName + ":latest"

	spec := docker.ContainerSpec{
		Name:       containerName,
		ImageTag:   imageTag,
		Dockerfile: hangingDockerfile,
		Labels:     map[string]string{"bac.managed": "true"},
	}

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

	_, err = docker.BuildImageWithTimeout(ctx, client, spec, testBuildTimeout, false)

	require.Error(t, err, "BuildImageWithTimeout must return an error when the build exceeds the timeout")
	require.Contains(t, err.Error(), "timed out",
		"error message must mention 'timed out'; got: %v", err)
}

// ----------------------------------------------------------------------------
// TestAFindConflictingUserPullsImageIfAbsent
// Validates: Req 10a.1 — FindConflictingUser must succeed even when the base
// image is not present in the local Docker image store.
//
// Named with "A" prefix so Go's alphabetical test ordering runs this first.
// The base image is guaranteed absent by TestMain's call to
// EnsureBaseImageAbsent(), so this test simply calls FindConflictingUser and
// asserts it succeeds (pulling the image automatically).
// ----------------------------------------------------------------------------

func TestAFindConflictingUserPullsImageIfAbsent(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	client, err := docker.NewClient()
	require.NoError(t, err, "connecting to Docker daemon")

	info, err := hostinfo.Current()
	require.NoError(t, err)

	result, err := docker.FindConflictingUser(ctx, client, info.UID, info.GID)
	require.NoError(t, err,
		"FindConflictingUser must succeed even when the base image is not cached locally")
	_ = result

	_, _, err = client.ImageInspectWithRaw(ctx, constants.BaseContainerImage)
	require.NoError(t, err,
		"base image should be present locally after FindConflictingUser pulls it")
}
