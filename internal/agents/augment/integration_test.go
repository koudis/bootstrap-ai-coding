//go:build integration

package augment_test

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
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
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

	projectDir, err := os.MkdirTemp("", "bac-augment-integration-*")
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

	augmentAgent, err := agent.Lookup(constants.AugmentCodeAgentName)
	if err != nil {
		return fmt.Errorf("looking up augment agent: %w", err)
	}
	augmentAgent.Install(builder)

	port, err := findFreePortAugment()
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}

	sharedContainerName = constants.ContainerNamePrefix + sanitizeAugment(dirName)
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
		HostUID: info.UID,
		HostGID: info.GID,
	}

	_, err = docker.BuildImage(ctx, sharedClient, baseSpec, true)
	if err != nil {
		return fmt.Errorf("building base image with augment: %w", err)
	}

	// Build instance image
	instanceBuilder := docker.NewInstanceImageBuilder(
		info,
		userPubKey,
		hostKeyPriv, hostKeyPub,
		port, false,
	)
	instanceBuilder.Finalize()

	spec := docker.ContainerSpec{
		Name:       sharedContainerName,
		ImageTag:   sharedImageTag,
		Dockerfile: instanceBuilder.Build(),
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
		HostUID: info.UID,
		HostGID: info.GID,
	}

	_, err = docker.BuildImage(ctx, sharedClient, spec, true)
	if err != nil {
		return fmt.Errorf("building container image with augment: %w", err)
	}

	_, err = docker.CreateContainer(ctx, sharedClient, spec)
	if err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	err = docker.StartContainer(ctx, sharedClient, sharedContainerName)
	if err != nil {
		return fmt.Errorf("starting container: %w", err)
	}

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
// 25.1 TestAugmentAvailableInContainer
// Validates: AC-2.3
// ----------------------------------------------------------------------------

func TestAugmentAvailableInContainer(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	exitCode, err := docker.ExecInContainer(ctx, sharedClient, sharedContainerName, []string{"auggie", "--version"})
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

	ctx := context.Background()

	augAgent, err := agent.Lookup(constants.AugmentCodeAgentName)
	require.NoError(t, err, "looking up augment agent")

	err = augAgent.HealthCheck(ctx, sharedClient, sharedContainerName)
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
