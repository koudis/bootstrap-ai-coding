//go:build integration

package augment_test

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	sshpkg "github.com/koudis/bootstrap-ai-coding/internal/ssh"
)

// setupContainerWithAugment builds a container image with the Augment Code
// agent installed, starts the container, waits for SSH to be ready, and
// returns the container name, SSH port, and a cleanup function.
func setupContainerWithAugment(t *testing.T) (containerName string, sshPort int, cleanup func()) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	projectDir := t.TempDir()
	dirName := filepath.Base(projectDir)

	hostKeyPriv, hostKeyPub, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating host key pair")

	_, userPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating user key pair")

	u, err := user.Current()
	require.NoError(t, err, "getting current user")
	uid, err := strconv.Atoi(u.Uid)
	require.NoError(t, err, "parsing UID")
	gid, err := strconv.Atoi(u.Gid)
	require.NoError(t, err, "parsing GID")

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

	// Install the Augment Code agent steps into the Dockerfile.
	augmentAgent, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err, "looking up augment agent")
	augmentAgent.Install(builder)

	port, err := findFreePortAugment()
	require.NoError(t, err, "finding free port")

	containerName = constants.ContainerNamePrefix + sanitizeAugment(dirName)
	imageTag := containerName + ":latest"

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

	_, err = docker.BuildImage(ctx, client, spec)
	require.NoError(t, err, "building container image with augment")

	_, err = docker.CreateContainer(ctx, client, spec)
	require.NoError(t, err, "creating container")

	err = docker.StartContainer(ctx, client, containerName)
	require.NoError(t, err, "starting container")

	// Augment installation takes longer — allow up to 2 minutes for SSH.
	err = docker.WaitForSSH(ctx, "127.0.0.1", port, 120*time.Second)
	require.NoError(t, err, "waiting for SSH to be ready")

	cleanup = func() {
		cleanCtx := context.Background()
		_ = docker.StopContainer(cleanCtx, client, containerName)
		_ = docker.RemoveContainer(cleanCtx, client, containerName)
		images, _ := docker.ListBACImages(cleanCtx, client)
		for _, img := range images {
			for _, tag := range img.RepoTags {
				if tag == imageTag {
					_, _ = client.ImageRemove(cleanCtx, img.ID, dockerimage.RemoveOptions{Force: true})
				}
			}
		}
	}

	return containerName, port, cleanup
}

// ----------------------------------------------------------------------------
// 25.1 TestAugmentAvailableInContainer
// Validates: AC-2.3
// ----------------------------------------------------------------------------

func TestAugmentAvailableInContainer(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	containerName, _, cleanup := setupContainerWithAugment(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	// Exec `auggie --version` inside the container and assert exit 0.
	exitCode, err := docker.ExecInContainer(ctx, containerName, []string{"auggie", "--version"})
	require.NoError(t, err, "exec auggie --version")
	require.Equal(t, 0, exitCode, "expected 'auggie --version' to exit 0")
}

// ----------------------------------------------------------------------------
// 25.2 TestAugmentHealthCheck
// Validates: AC-5
// ----------------------------------------------------------------------------

func TestAugmentHealthCheck(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	containerName, _, cleanup := setupContainerWithAugment(t)
	t.Cleanup(cleanup)

	ctx := context.Background()

	augAgent, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err, "looking up augment agent")

	err = augAgent.HealthCheck(ctx, containerName)
	require.NoError(t, err, "augment HealthCheck should return no error")
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

func findFreePortAugment() (int, error) {
	for port := constants.SSHPortStart; port < 65535; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found starting at %d", constants.SSHPortStart)
}

func sanitizeAugment(s string) string {
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
