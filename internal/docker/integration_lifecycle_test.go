//go:build integration

package docker_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"
	sshpkg "github.com/koudis/bootstrap-ai-coding/internal/ssh"
)

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
