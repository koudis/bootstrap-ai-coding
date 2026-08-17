//go:build integration

package docker_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
	sharedImageTag    string
	sharedClient      *docker.Client
	sharedProjectDir  string
	sharedHostInfo    *hostinfo.Info
	sharedUserPrivKey string // PEM-encoded private key for SSH auth in tests
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

	userPrivKey, userPubKey, err := sshpkg.GenerateHostKeyPair()
	require.NoError(t, err, "generating user key pair")
	sharedUserPrivKey = userPrivKey

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
