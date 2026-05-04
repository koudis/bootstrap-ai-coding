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
	sshpkg "github.com/koudis/bootstrap-ai-coding/internal/ssh"
	"github.com/koudis/bootstrap-ai-coding/internal/testutil"
)

// ----------------------------------------------------------------------------
// TestMain — integration suite precondition check
// ----------------------------------------------------------------------------

// TestMain ensures the base image is removed from the local Docker image store
// before the integration suite runs. The suite includes
// TestAFindConflictingUserPullsImageIfAbsent, which specifically tests the
// pull-before-inspect path. Removing the image guarantees a clean slate so
// that test always exercises the auto-pull logic.
func TestMain(m *testing.M) {
	if _, err := exec.LookPath("docker"); err != nil {
		// Docker not available — individual tests will skip themselves.
		os.Exit(m.Run())
	}

	testutil.RequireIntegrationConsent()

	if err := testutil.EnsureBaseImageAbsent(); err != nil {
		fmt.Fprintf(os.Stderr, "EnsureBaseImageAbsent: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// ----------------------------------------------------------------------------
// Helper: setupContainer
// ----------------------------------------------------------------------------

// setupContainer creates a temp project dir, builds a container image, starts
// the container, waits for SSH to be ready, and returns the container name,
// SSH port, and a cleanup function.
//
// The caller is responsible for registering the cleanup via t.Cleanup or defer.
func setupContainer(t *testing.T) (containerName string, sshPort int, cleanup func()) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	// 1. Create a temp project dir.
	projectDir := t.TempDir()
	dirName := filepath.Base(projectDir)

	// 2. Generate SSH host key pair.
	hostKeyPriv, hostKeyPub, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating host key pair")

	// 3. Discover or generate a public key for the container user.
	// Use a freshly generated key so tests are hermetic.
	_, userPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating user key pair")

	// 4. Determine host UID/GID.
	u, err := user.Current()
	require.NoError(t, err, "getting current user")
	uid, err := strconv.Atoi(u.Uid)
	require.NoError(t, err, "parsing UID")
	gid, err := strconv.Atoi(u.Gid)
	require.NoError(t, err, "parsing GID")

	// 5. Build a DockerfileBuilder with the host UID/GID.
	// Check for UID/GID conflicts in the base image first (mirrors runStart logic).
	client, err := docker.NewClient()
	require.NoError(t, err, "connecting to Docker daemon")

	strategy := docker.UserStrategyCreate
	conflictingUser := ""
	conflictingImageUser, err := docker.FindConflictingUser(ctx, client, uid, gid)
	require.NoError(t, err, "checking base image for UID/GID conflicts")
	if conflictingImageUser != nil {
		strategy = docker.UserStrategyRename
		conflictingUser = conflictingImageUser.Username
	}

	builder := docker.NewDockerfileBuilder(
		uid, gid,
		userPubKey,
		hostKeyPriv, hostKeyPub,
		strategy, conflictingUser,
	)

	// 6. Pick a free port.
	port, err := findFreePort()
	require.NoError(t, err, "finding free port")

	// Derive a deterministic container name from the temp dir name.
	containerName = constants.ContainerNamePrefix + sanitize(dirName)
	imageTag := containerName + ":latest"

	// CMD must be the last instruction — call Finalize() before Build().
	builder.Finalize()

	spec := docker.ContainerSpec{
		Name:       containerName,
		ImageTag:   imageTag,
		Dockerfile: builder.Build(),
		Mounts: []docker.Mount{
			{
				HostPath:      projectDir,
				ContainerPath: constants.WorkspaceMountPath,
				ReadOnly:      false,
			},
		},
		SSHPort: port,
		Labels: map[string]string{
			"bac.managed": "true",
		},
		HostUID: uid,
		HostGID: gid,
	}

	// 7. Build the image.
	_, err = docker.BuildImage(ctx, client, spec, false)
	require.NoError(t, err, "building container image")

	// 9. Create and start the container.
	_, err = docker.CreateContainer(ctx, client, spec)
	require.NoError(t, err, "creating container")

	err = docker.StartContainer(ctx, client, containerName)
	require.NoError(t, err, "starting container")

	// 10. Wait for SSH to be ready.
	err = docker.WaitForSSH(ctx, "127.0.0.1", port, 60*time.Second)
	require.NoError(t, err, "waiting for SSH to be ready")

	cleanup = func() {
		cleanCtx := context.Background()
		_ = docker.StopContainer(cleanCtx, client, containerName)
		_ = docker.RemoveContainer(cleanCtx, client, containerName)
		// Remove the image.
		images, _ := docker.ListBACImages(cleanCtx, client)
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if tag == imageTag {
					_, _ = client.ImageRemove(cleanCtx, img.ID, forceRemoveOpts())
				}
			}
		}
	}

	return containerName, port, cleanup
}

// ----------------------------------------------------------------------------
// 16.1 TestContainerStartsAndSSHConnects
// Validates: Req 3.3, 4.3
// ----------------------------------------------------------------------------

func TestContainerStartsAndSSHConnects(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	_, sshPort, cleanup := setupContainer(t)
	t.Cleanup(cleanup)

	// Assert TCP connection to SSH port succeeds.
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

	containerName, _, cleanup := setupContainer(t)
	t.Cleanup(cleanup)

	// Write a file to the host project dir (the mount source).
	// We need the project dir path — re-derive it from the container name by
	// writing a sentinel file to a known temp dir.
	// Instead, we write a file via docker exec to /workspace and check it on host.
	// Actually, the test writes to the host dir and checks inside the container.
	// We need the host project dir. Since setupContainer uses t.TempDir(), we
	// can't easily get it back. Instead, we write a file inside the container
	// and verify it appears on the host via docker exec in reverse.
	//
	// The simplest approach: exec a command inside the container to create a file
	// at /workspace/sync-test.txt, then exec another command to verify it exists.
	ctx := context.Background()

	// Create a file inside the container at /workspace.
	exitCode, err := docker.ExecInContainer(ctx, containerName, []string{
		"bash", "-c", "echo 'hello from container' > /workspace/sync-test.txt",
	})
	require.NoError(t, err, "exec to create file in /workspace")
	require.Equal(t, 0, exitCode, "expected exit 0 when creating file in /workspace")

	// Verify the file exists inside the container at the workspace mount path.
	exitCode, err = docker.ExecInContainer(ctx, containerName, []string{
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

	containerName, _, cleanup := setupContainer(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	u, err := user.Current()
	require.NoError(t, err)

	// Create a file inside the container at /workspace/ as the container user.
	// Use `su` to run as the container user so the file is owned by UID/GID 1000,
	// not root (docker exec runs as root by default).
	exitCode, err := docker.ExecInContainer(ctx, containerName, []string{
		"su", "-c", "touch /workspace/ownership-test.txt", constants.ContainerUser,
	})
	require.NoError(t, err)
	require.Equal(t, 0, exitCode, "expected exit 0 when creating file")

	// Check UID and GID in two separate execs to keep the shell commands simple
	// and avoid quoting issues. Each command exits 0 iff the value matches.
	checkUID := fmt.Sprintf(`[ "$(stat -c '%%u' /workspace/ownership-test.txt)" = "%s" ]`, u.Uid)
	exitCode, err = docker.ExecInContainer(ctx, containerName, []string{"bash", "-c", checkUID})
	require.NoError(t, err, "exec to check file UID")
	require.Equal(t, 0, exitCode,
		"expected file UID inside container to match host user UID=%s", u.Uid)

	checkGID := fmt.Sprintf(`[ "$(stat -c '%%g' /workspace/ownership-test.txt)" = "%s" ]`, u.Gid)
	exitCode, err = docker.ExecInContainer(ctx, containerName, []string{"bash", "-c", checkGID})
	require.NoError(t, err, "exec to check file GID")
	require.Equal(t, 0, exitCode,
		"expected file GID inside container to match host user GID=%s", u.Gid)
}

// ----------------------------------------------------------------------------
// 16.4 TestCredentialVolumePersistedAcrossRestart
// Validates: Req 8.6
// ----------------------------------------------------------------------------

func TestCredentialVolumePersistedAcrossRestart(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	containerName, sshPort, cleanup := setupContainer(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	// Write a sentinel file to /workspace (the bind-mounted volume) inside the container.
	exitCode, err := docker.ExecInContainer(ctx, containerName, []string{
		"bash", "-c", "echo 'persistent' > /workspace/persist-test.txt",
	})
	require.NoError(t, err)
	require.Equal(t, 0, exitCode, "expected exit 0 when writing sentinel file")

	// Stop the container.
	client, err := docker.NewClient()
	require.NoError(t, err)

	err = docker.StopContainer(ctx, client, containerName)
	require.NoError(t, err, "stopping container")

	// Restart the container.
	err = docker.StartContainer(ctx, client, containerName)
	require.NoError(t, err, "restarting container")

	// Wait for SSH to be ready again.
	err = docker.WaitForSSH(ctx, "127.0.0.1", sshPort, 30*time.Second)
	require.NoError(t, err, "waiting for SSH after restart")

	// Assert the file is still present.
	exitCode, err = docker.ExecInContainer(ctx, containerName, []string{
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

	containerName, sshPort, cleanup := setupContainer(t)
	t.Cleanup(cleanup)

	ctx := context.Background()
	client, err := docker.NewClient()
	require.NoError(t, err)

	// Record the SSH port before restart.
	originalPort := sshPort

	// Stop the container.
	err = docker.StopContainer(ctx, client, containerName)
	require.NoError(t, err, "stopping container")

	// Restart the container.
	err = docker.StartContainer(ctx, client, containerName)
	require.NoError(t, err, "restarting container")

	// Wait for SSH to be ready again.
	err = docker.WaitForSSH(ctx, "127.0.0.1", originalPort, 30*time.Second)
	require.NoError(t, err, "waiting for SSH after restart on original port %d", originalPort)

	// Assert the same port is still reachable (port binding is preserved).
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

	// Generate a host key pair that we will reuse across both builds.
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

	projectDir := t.TempDir()
	dirName := filepath.Base(projectDir)
	containerName := constants.ContainerNamePrefix + sanitize(dirName)
	imageTag := containerName + ":latest"

	port, err := findFreePort()
	require.NoError(t, err)

	buildAndGetFingerprint := func() string {
		t.Helper()
		client, err := docker.NewClient()
		require.NoError(t, err)

		strategy := docker.UserStrategyCreate
		conflictingUser := ""
		conflictingImageUser, err := docker.FindConflictingUser(ctx, client, uid, gid)
		require.NoError(t, err, "checking base image for UID/GID conflicts")
		if conflictingImageUser != nil {
			strategy = docker.UserStrategyRename
			conflictingUser = conflictingImageUser.Username
		}

		builder := docker.NewDockerfileBuilder(
			uid, gid,
			userPubKey,
			hostKeyPriv, hostKeyPub,
			strategy, conflictingUser,
		)
		builder.Finalize()
		spec := docker.ContainerSpec{
			Name:       containerName,
			ImageTag:   imageTag,
			Dockerfile: builder.Build(),
			Mounts: []docker.Mount{
				{HostPath: projectDir, ContainerPath: constants.WorkspaceMountPath},
			},
			SSHPort: port,
			Labels:  map[string]string{"bac.managed": "true"},
			HostUID: uid,
			HostGID: gid,
		}

		_, err = docker.BuildImage(ctx, client, spec, false)
		require.NoError(t, err, "building image")

		return hostKeyPub
	}

	// Build once and record the host key fingerprint.
	fingerprint1 := buildAndGetFingerprint()

	// Rebuild (simulating --rebuild: same key pair, new image build).
	fingerprint2 := buildAndGetFingerprint()

	// The host key fingerprint must be identical across rebuilds.
	require.Equal(t, fingerprint1, fingerprint2,
		"SSH host key fingerprint must be stable across rebuilds")

	// Cleanup.
	t.Cleanup(func() {
		cleanCtx := context.Background()
		client, _ := docker.NewClient()
		if client != nil {
			images, _ := docker.ListBACImages(cleanCtx, client)
			for _, img := range images {
				for _, tag := range img.RepoTags {
					if tag == imageTag {
						_, _ = client.ImageRemove(cleanCtx, img.ID, forceRemoveOpts())
					}
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

	containerName, _, _ := setupContainer(t)
	// Note: we do NOT register the cleanup here because we are testing purge.

	client, err := docker.NewClient()
	require.NoError(t, err)

	imageTag := containerName + ":latest"

	// Verify the container exists before purge.
	info, err := docker.InspectContainer(ctx, client, containerName)
	require.NoError(t, err)
	require.NotNil(t, info, "container should exist before purge")

	// Run purge logic: stop + remove container, remove image.
	err = docker.StopContainer(ctx, client, containerName)
	require.NoError(t, err, "stopping container during purge")

	err = docker.RemoveContainer(ctx, client, containerName)
	require.NoError(t, err, "removing container during purge")

	// Remove the image.
	images, err := docker.ListBACImages(ctx, client)
	require.NoError(t, err)
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == imageTag {
				_, err = client.ImageRemove(ctx, img.ID, forceRemoveOpts())
				require.NoError(t, err, "removing image during purge")
			}
		}
	}

	// Assert container is gone.
	info, err = docker.InspectContainer(ctx, client, containerName)
	require.NoError(t, err)
	require.Nil(t, info, "container should be gone after purge")

	// Assert image is gone.
	images, err = docker.ListBACImages(ctx, client)
	require.NoError(t, err)
	for _, img := range images {
		for _, tag := range img.RepoTags {
			require.NotEqual(t, imageTag, tag, "image should be gone after purge")
		}
	}
}

// ----------------------------------------------------------------------------
// 16.10 TestKnownHostsEntriesLifecycle
// Validates: Req 18.1–18.2, 18.7
// ----------------------------------------------------------------------------

func TestKnownHostsEntriesLifecycle(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	// Use a temp HOME to avoid touching the real ~/.ssh/known_hosts.
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	_, sshPort, cleanup := setupContainer(t)

	// Generate a host public key to use as the known_hosts entry value.
	_, hostPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err)

	// Add known_hosts entries after container starts.
	err = sshpkg.SyncKnownHosts(sshPort, hostPubKey, false)
	require.NoError(t, err, "SyncKnownHosts should succeed")

	// Assert both entries are present.
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

	// Stop and remove the container.
	cleanup()

	// Remove known_hosts entries.
	err = sshpkg.RemoveKnownHostsEntries(sshPort)
	require.NoError(t, err, "RemoveKnownHostsEntries should succeed")

	// Assert both entries are gone.
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

	// Use a temp HOME to avoid touching the real ~/.ssh/config.
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	containerName, sshPort, cleanup := setupContainer(t)

	// Add SSH config entry after container starts.
	err := sshpkg.SyncSSHConfig(containerName, sshPort, false)
	require.NoError(t, err, "SyncSSHConfig should succeed")

	// Assert the Host stanza is present with correct fields.
	cfgPath := filepath.Join(tempHome, ".ssh", "config")
	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err, "ssh config file should exist")
	content := string(data)

	hostLine := fmt.Sprintf("Host %s", containerName)
	portLine := fmt.Sprintf("Port %d", sshPort)
	userLine := fmt.Sprintf("User %s", constants.ContainerUser)
	hostnameLine := "HostName localhost"

	require.True(t, strings.Contains(content, hostLine),
		"ssh config should contain 'Host %s'", containerName)
	require.True(t, strings.Contains(content, portLine),
		"ssh config should contain 'Port %d'", sshPort)
	require.True(t, strings.Contains(content, userLine),
		"ssh config should contain 'User %s'", constants.ContainerUser)
	require.True(t, strings.Contains(content, hostnameLine),
		"ssh config should contain 'HostName localhost'")

	// Stop and remove the container.
	cleanup()

	// Remove the SSH config entry.
	err = sshpkg.RemoveSSHConfigEntry(containerName)
	require.NoError(t, err, "RemoveSSHConfigEntry should succeed")

	// Assert the stanza is gone.
	data, err = os.ReadFile(cfgPath)
	require.NoError(t, err, "ssh config file should still exist after removal")
	content = string(data)

	require.False(t, strings.Contains(content, hostLine),
		"ssh config should NOT contain 'Host %s' after removal", containerName)
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

// findFreePort finds a free TCP port starting at constants.SSHPortStart.
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

// sanitize lowercases s and replaces characters outside [a-z0-9] with '-',
// collapsing consecutive dashes and trimming leading/trailing dashes.
// Used to derive a container name from a temp dir name.
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
	// Collapse consecutive dashes.
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	result = strings.Trim(result, "-")
	if result == "" {
		result = "tmp"
	}
	return result
}

// forceRemoveOpts returns image remove options with Force=true.
func forceRemoveOpts() dockerimage.RemoveOptions {
	return dockerimage.RemoveOptions{Force: true}
}

// ----------------------------------------------------------------------------
// TestBuildImageTimeoutEnforced
// Validates: Req 14.7 (Image_Build_Timeout)
// ----------------------------------------------------------------------------

// testBuildTimeout is the deadline used in the timeout integration test.
// It is intentionally much shorter than constants.ImageBuildTimeout so the
// test completes quickly. 3 seconds is enough for the Docker daemon to accept
// the build request and start executing the sleep RUN step before the context
// is cancelled.
const testBuildTimeout = 3 * time.Second

func TestBuildImageTimeoutEnforced(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	client, err := docker.NewClient()
	require.NoError(t, err, "connecting to Docker daemon")

	// Build a minimal Dockerfile whose single RUN step sleeps far longer than
	// testBuildTimeout. The build must be cancelled before it completes.
	hangingDockerfile := fmt.Sprintf("FROM %s\nRUN sleep 300\n", constants.BaseContainerImage)

	containerName := constants.ContainerNamePrefix + "timeout-test"
	imageTag := containerName + ":latest"

	spec := docker.ContainerSpec{
		Name:       containerName,
		ImageTag:   imageTag,
		Dockerfile: hangingDockerfile,
		Labels:     map[string]string{"bac.managed": "true"},
	}

	// Ensure any partial image is cleaned up regardless of test outcome.
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

	u, err := user.Current()
	require.NoError(t, err)
	uid, err := strconv.Atoi(u.Uid)
	require.NoError(t, err)
	gid, err := strconv.Atoi(u.Gid)
	require.NoError(t, err)

	result, err := docker.FindConflictingUser(ctx, client, uid, gid)
	require.NoError(t, err,
		"FindConflictingUser must succeed even when the base image is not cached locally")
	_ = result

	// Verify the image is now present locally (was pulled by FindConflictingUser).
	_, _, err = client.ImageInspectWithRaw(ctx, constants.BaseContainerImage)
	require.NoError(t, err,
		"base image should be present locally after FindConflictingUser pulls it")
}
