//go:build integration

package kiro_test

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

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/kiro"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"
	sshpkg "github.com/koudis/bootstrap-ai-coding/internal/ssh"
	"github.com/koudis/bootstrap-ai-coding/internal/testutil"
)

// Package-level shared container state — built once in TestMain, reused by all tests.
var (
	sharedContainerName string
	sharedSSHPort       int
	sharedClient        *docker.Client
	sharedImageTag      string
	sharedUsername      string
)

// TestMain gates the integration suite behind an explicit consent prompt,
// builds the container image once, starts the container, and tears it down
// after all tests complete.
func TestMain(m *testing.M) {
	if _, err := exec.LookPath("docker"); err != nil {
		os.Exit(m.Run())
	}

	testutil.RequireIntegrationConsent()

	if err := testutil.EnsureBaseImageAbsent(); err != nil {
		fmt.Fprintf(os.Stderr, "EnsureBaseImageAbsent: %v\n", err)
		os.Exit(1)
	}

	if err := setupSharedContainer(); err != nil {
		fmt.Fprintf(os.Stderr, "setupSharedContainer: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	teardownSharedContainer()
	os.Exit(code)
}

func setupSharedContainer() error {
	ctx := context.Background()

	projectDir, err := os.MkdirTemp("", "bac-kiro-integration-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	dirName := filepath.Base(projectDir)

	hostKeyPriv, hostKeyPub, err := sshpkg.GenerateHostKeyPair()
	if err != nil {
		return fmt.Errorf("generating host key pair: %w", err)
	}

	_, userPubKey, err := sshpkg.GenerateHostKeyPair()
	if err != nil {
		return fmt.Errorf("generating user key pair: %w", err)
	}

	info, err := hostinfo.Current()
	if err != nil {
		return fmt.Errorf("getting host info: %w", err)
	}

	sharedClient, err = docker.NewClient()
	if err != nil {
		return fmt.Errorf("connecting to Docker daemon: %w", err)
	}

	strategy := docker.UserStrategyCreate
	conflictingUser := ""
	conflictingImageUser, err := docker.FindConflictingUser(ctx, sharedClient, info.UID, info.GID)
	if err != nil {
		return fmt.Errorf("checking base image for UID/GID conflicts: %w", err)
	}
	if conflictingImageUser != nil {
		strategy = docker.UserStrategyRename
		conflictingUser = conflictingImageUser.Username
	}

	builder := docker.NewBaseImageBuilder(
		info,
		strategy, conflictingUser,
		"",
	)

	kiroAgent, err := agent.Lookup(constants.KiroAgentName)
	if err != nil {
		return fmt.Errorf("looking up kiro agent: %w", err)
	}
	kiroAgent.Install(builder)

	port, err := findFreePortKiro()
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}

	sharedContainerName = constants.ContainerNamePrefix + sanitizeKiro(dirName)
	sharedImageTag = sharedContainerName + ":latest"
	sharedSSHPort = port
	sharedUsername = info.Username

	// Build base image
	baseSpec := docker.ContainerSpec{
		Name:       sharedContainerName,
		ImageTag:   constants.BaseImageTag,
		Dockerfile: builder.Build(),
		Labels: map[string]string{
			"bac.managed": "true",
		},
		HostInfo: info,
	}

	_, err = docker.BuildImage(ctx, sharedClient, baseSpec, false)
	if err != nil {
		return fmt.Errorf("building base image with kiro: %w", err)
	}

	// Build instance image
	instanceBuilder := docker.NewInstanceImageBuilder(
		info,
		userPubKey,
		hostKeyPriv, hostKeyPub,
		port, false,
	)
	instanceBuilder.Finalize()

	// Create a temp directory for the ~/.kiro credential mount.
	credKiroDir, err := os.MkdirTemp("", "kiro-config-*")
	if err != nil {
		return fmt.Errorf("creating temp kiro config dir: %w", err)
	}

	mounts := []docker.Mount{
		{
			HostPath:      projectDir,
			ContainerPath: constants.WorkspaceMountPath,
			ReadOnly:      false,
		},
		{
			HostPath:      credKiroDir,
			ContainerPath: filepath.Join(info.HomeDir, ".kiro"),
			ReadOnly:      false,
		},
	}

	spec := docker.ContainerSpec{
		Name:       sharedContainerName,
		ImageTag:   sharedImageTag,
		Dockerfile: instanceBuilder.Build(),
		Mounts:     mounts,
		SSHPort:    port,
		Labels: map[string]string{
			"bac.managed": "true",
		},
		HostInfo: info,
	}

	_, err = docker.BuildImage(ctx, sharedClient, spec, false)
	if err != nil {
		return fmt.Errorf("building container image with kiro: %w", err)
	}

	_, err = docker.CreateContainer(ctx, sharedClient, spec)
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	err = docker.StartContainer(ctx, sharedClient, sharedContainerName)
	if err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	// Kiro installation takes longer (Node.js + CLI download) — allow up to 2
	// minutes for SSH.
	err = docker.WaitForSSH(ctx, "127.0.0.1", port, 120*time.Second)
	if err != nil {
		return fmt.Errorf("waiting for SSH to be ready: %w", err)
	}

	return nil
}

func teardownSharedContainer() {
	ctx := context.Background()
	if sharedClient == nil {
		return
	}
	_ = docker.StopContainer(ctx, sharedClient, sharedContainerName)
	_ = docker.RemoveContainer(ctx, sharedClient, sharedContainerName)
	images, _ := docker.ListBACImages(ctx, sharedClient)
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag == sharedImageTag {
				_, _ = sharedClient.ImageRemove(ctx, img.ID, dockerimage.RemoveOptions{Force: true})
			}
		}
	}
}

// ----------------------------------------------------------------------------
// TestKiroAvailableInContainer
// Validates: KIRO-2.3
// ----------------------------------------------------------------------------

func TestKiroAvailableInContainer(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	exitCode, err := docker.ExecInContainer(ctx, sharedClient, sharedContainerName, []string{"kiro-cli", "--version"})
	require.NoError(t, err, "exec kiro-cli --version")
	require.Equal(t, 0, exitCode, "expected 'kiro-cli --version' to exit 0")
}

// ----------------------------------------------------------------------------
// TestKiroHealthCheck
// Validates: KIRO-7
// ----------------------------------------------------------------------------

func TestKiroHealthCheck(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	kiroAgent, err := agent.Lookup(constants.KiroAgentName)
	require.NoError(t, err, "looking up kiro agent")

	err = kiroAgent.HealthCheck(ctx, sharedClient, sharedContainerName, sharedUsername)
	require.NoError(t, err, "kiro HealthCheck should return no error")
}

// ----------------------------------------------------------------------------
// TestKiroCredentialMountExists
// Validates: KIRO-5
// ----------------------------------------------------------------------------

func TestKiroCredentialMountExists(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	info, err := hostinfo.Current()
	require.NoError(t, err, "getting host info")

	kiroPath := filepath.Join(info.HomeDir, ".kiro")
	exitCode, err := docker.ExecInContainer(ctx, sharedClient, sharedContainerName, []string{"ls", "-d", kiroPath})
	require.NoError(t, err, "exec ls -d kiro credential path")
	require.Equal(t, 0, exitCode, "expected kiro credential mount path %s to exist", kiroPath)
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

func findFreePortKiro() (int, error) {
	for port := constants.SSHPortStart; port < 65535; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found starting at %d", constants.SSHPortStart)
}

func sanitizeKiro(s string) string {
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
