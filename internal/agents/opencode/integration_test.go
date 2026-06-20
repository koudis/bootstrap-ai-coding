//go:build integration

package opencode_test

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
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/opencode"
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

	projectDir, err := os.MkdirTemp("", "bac-opencode-integration-*")
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

	opencodeAgent, err := agent.Lookup(constants.OpenCodeAgentName)
	if err != nil {
		return fmt.Errorf("looking up opencode agent: %w", err)
	}
	opencodeAgent.Install(builder)

	port, err := findFreePortOpencode()
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}

	sharedContainerName = constants.ContainerNamePrefix + sanitizeOpencode(dirName)
	sharedImageTag = sharedContainerName + ":latest"
	sharedSSHPort = port

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
		return fmt.Errorf("building base image with opencode: %w", err)
	}

	// Build instance image
	instanceBuilder := docker.NewInstanceImageBuilder(
		info,
		userPubKey,
		hostKeyPriv, hostKeyPub,
		port, false,
	)
	instanceBuilder.Finalize()

	// Create temp directories for credential mounts
	credAuthDir, err := os.MkdirTemp("", "opencode-auth-*")
	if err != nil {
		return fmt.Errorf("creating temp auth dir: %w", err)
	}
	credConfigDir, err := os.MkdirTemp("", "opencode-config-*")
	if err != nil {
		return fmt.Errorf("creating temp config dir: %w", err)
	}

	// Prepare mounts: workspace + primary credential store + additional mounts
	mounts := []docker.Mount{
		{
			HostPath:      projectDir,
			ContainerPath: constants.WorkspaceMountPath,
			ReadOnly:      false,
		},
		{
			HostPath:      credAuthDir,
			ContainerPath: filepath.Join(info.HomeDir, ".local", "share", "opencode"),
			ReadOnly:      false,
		},
	}

	// Add additional mounts from the AdditionalMounter interface
	if mounter, ok := opencodeAgent.(agent.AdditionalMounter); ok {
		for _, extra := range mounter.AdditionalMounts(info.HomeDir) {
			mounts = append(mounts, docker.Mount{
				HostPath:      credConfigDir,
				ContainerPath: extra.ContainerPath,
				ReadOnly:      extra.ReadOnly,
			})
		}
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
		return fmt.Errorf("building container image with opencode: %w", err)
	}

	_, err = docker.CreateContainer(ctx, sharedClient, spec)
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	err = docker.StartContainer(ctx, sharedClient, sharedContainerName)
	if err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

	// OpenCode installation takes longer (npm install) — allow up to 2 minutes for SSH.
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
// TestOpenCodeAvailableInContainer
// Validates: Requirements 2.3, 2.5
// ----------------------------------------------------------------------------

func TestOpenCodeAvailableInContainer(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	exitCode, err := docker.ExecInContainer(ctx, sharedClient, sharedContainerName, []string{"opencode", "--version"})
	require.NoError(t, err, "exec opencode --version")
	require.Equal(t, 0, exitCode, "expected 'opencode --version' to exit 0")
}

// ----------------------------------------------------------------------------
// TestOpenCodeHealthCheck
// Validates: Requirements 5.1
// ----------------------------------------------------------------------------

func TestOpenCodeHealthCheck(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	opencodeAgent, err := agent.Lookup(constants.OpenCodeAgentName)
	require.NoError(t, err, "looking up opencode agent")

	err = opencodeAgent.HealthCheck(ctx, sharedClient, sharedContainerName)
	require.NoError(t, err, "opencode HealthCheck should return no error")
}

// ----------------------------------------------------------------------------
// TestOpenCodeCredentialMountsExist
// Validates: Requirements 3.1, 3.2, 3.3, 3.4, 3.5
// ----------------------------------------------------------------------------

func TestOpenCodeCredentialMountsExist(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	info, err := hostinfo.Current()
	require.NoError(t, err, "getting host info")

	// Verify primary credential mount path exists (~/.local/share/opencode)
	primaryPath := filepath.Join(info.HomeDir, ".local", "share", "opencode")
	exitCode, err := docker.ExecInContainer(ctx, sharedClient, sharedContainerName, []string{"ls", "-d", primaryPath})
	require.NoError(t, err, "exec ls -d primary credential path")
	require.Equal(t, 0, exitCode, "expected primary credential mount path %s to exist", primaryPath)

	// Verify additional credential mount path exists (~/.config/opencode)
	configPath := filepath.Join(info.HomeDir, ".config", "opencode")
	exitCode, err = docker.ExecInContainer(ctx, sharedClient, sharedContainerName, []string{"ls", "-d", configPath})
	require.NoError(t, err, "exec ls -d additional credential path")
	require.Equal(t, 0, exitCode, "expected additional credential mount path %s to exist", configPath)
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

func findFreePortOpencode() (int, error) {
	for port := constants.SSHPortStart; port < 65535; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found starting at %d", constants.SSHPortStart)
}

func sanitizeOpencode(s string) string {
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
