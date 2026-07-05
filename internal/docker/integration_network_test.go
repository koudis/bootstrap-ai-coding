//go:build integration

package docker_test

import (
	"context"
	"fmt"
	"net"
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

	// Use a dynamically allocated port to avoid conflicts
	sshPort, err := findFreePort()
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

	// Use a dynamically allocated port to avoid conflicts
	sshPort, err := findFreePort()
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

	// Use a dynamically allocated port to avoid conflicts
	sshPort, err := findFreePort()
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
