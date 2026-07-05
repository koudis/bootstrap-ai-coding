//go:build integration

package docker_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"
)

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

// ----------------------------------------------------------------------------
// TestReadOnlyFileMountIsReadableButNotWritable
// Validates: CC-8 (read-only bind-mount of ~/.claude.json) — core mount plumbing
// ----------------------------------------------------------------------------

func TestReadOnlyFileMountIsReadableButNotWritable(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	buildSharedImage(t)

	ctx := context.Background()

	projectDir := t.TempDir()
	dirName := filepath.Base(projectDir)

	// Create a temporary file to mount read-only into the container.
	hostFile := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(hostFile, []byte(`{"test":"read-only-mount"}`), 0o644)
	require.NoError(t, err, "creating host file for RO mount")

	port, err := findFreePort()
	require.NoError(t, err, "finding free port")

	containerName := constants.ContainerNamePrefix + sanitize(dirName) + "-ro"
	containerFilePath := filepath.Join(sharedHostInfo.HomeDir, ".config-test.json")

	spec := docker.ContainerSpec{
		Name:     containerName,
		ImageTag: sharedImageTag,
		Mounts: []docker.Mount{
			{HostPath: projectDir, ContainerPath: constants.WorkspaceMountPath},
			{HostPath: hostFile, ContainerPath: containerFilePath, ReadOnly: true},
		},
		SSHPort:        port,
		Labels:         map[string]string{"bac.managed": "true"},
		HostInfo:       sharedHostInfo,
		HostNetworkOff: true,
	}

	_, err = docker.CreateContainer(ctx, sharedClient, spec)
	require.NoError(t, err, "creating container with RO file mount")

	t.Cleanup(func() {
		cleanCtx := context.Background()
		_ = docker.StopContainer(cleanCtx, sharedClient, containerName)
		_ = docker.RemoveContainer(cleanCtx, sharedClient, containerName)
	})

	err = docker.StartContainer(ctx, sharedClient, containerName)
	require.NoError(t, err, "starting container with RO file mount")

	err = docker.WaitForSSH(ctx, "127.0.0.1", port, 60*time.Second)
	require.NoError(t, err, "waiting for SSH to be ready")

	// Verify the file is readable inside the container.
	exitCode, err := docker.ExecInContainer(ctx, sharedClient, containerName, []string{
		"cat", containerFilePath,
	})
	require.NoError(t, err, "exec cat on RO-mounted file")
	require.Equal(t, 0, exitCode, "expected RO-mounted file to be readable")

	// Verify writes are rejected (read-only filesystem).
	exitCode, err = docker.ExecInContainer(ctx, sharedClient, containerName, []string{
		"bash", "-c", fmt.Sprintf("echo 'write attempt' > %s", containerFilePath),
	})
	require.NoError(t, err, "exec write attempt on RO-mounted file")
	require.NotEqual(t, 0, exitCode, "expected write to RO-mounted file to fail")
}
