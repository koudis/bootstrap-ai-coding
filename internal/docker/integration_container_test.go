//go:build integration

package docker_test

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"
)

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
