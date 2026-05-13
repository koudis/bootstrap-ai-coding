//go:build integration

package vibekanban_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	dockerimage "github.com/docker/docker/api/types/image"
	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/vibekanban"
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

	projectDir, err := os.MkdirTemp("", "bac-vibekanban-integration-*")
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

	vkAgent, err := agent.Lookup(constants.VibeKanbanAgentName)
	if err != nil {
		return fmt.Errorf("looking up vibe-kanban agent: %w", err)
	}
	vkAgent.Install(builder)

	port, err := findFreePortVK()
	if err != nil {
		return fmt.Errorf("finding free port: %w", err)
	}

	sharedContainerName = constants.ContainerNamePrefix + sanitizeVK(dirName)
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

	_, err = docker.BuildImage(ctx, sharedClient, baseSpec, true)
	if err != nil {
		return fmt.Errorf("building base image with vibe-kanban: %w", err)
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
		HostInfo: info,
	}

	_, err = docker.BuildImage(ctx, sharedClient, spec, true)
	if err != nil {
		return fmt.Errorf("building container image with vibe-kanban: %w", err)
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

	// Give vibe-kanban time to start up via the supervisor
	time.Sleep(5 * time.Second)

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
// TestVibeKanbanInstallsAndRuns
// Validates: VK-2.3, VK-3.1, VK-5.1, VK-5.2
// Full image build, binary present (which vibe-kanban exits 0), process running
// ----------------------------------------------------------------------------

func TestVibeKanbanInstallsAndRuns(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	// Verify binary is present
	exitCode, err := docker.ExecInContainer(ctx, sharedClient, sharedContainerName, []string{"which", "vibe-kanban"})
	require.NoError(t, err, "exec which vibe-kanban")
	require.Equal(t, 0, exitCode, "expected 'which vibe-kanban' to exit 0 (binary present)")

	// Verify process is running
	exitCode, err = docker.ExecInContainer(ctx, sharedClient, sharedContainerName, []string{"pgrep", "-f", "vibe-kanban"})
	require.NoError(t, err, "exec pgrep -f vibe-kanban")
	require.Equal(t, 0, exitCode, "expected vibe-kanban process to be running")
}

// ----------------------------------------------------------------------------
// TestVibeKanbanHealthCheck
// Validates: VK-5.1, VK-5.2
// HealthCheck passes on a live container
// ----------------------------------------------------------------------------

func TestVibeKanbanHealthCheck(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	vkAgent, err := agent.Lookup(constants.VibeKanbanAgentName)
	require.NoError(t, err, "looking up vibe-kanban agent")

	err = vkAgent.HealthCheck(ctx, sharedClient, sharedContainerName)
	require.NoError(t, err, "vibe-kanban HealthCheck should return no error")
}

// ----------------------------------------------------------------------------
// TestVibeKanbanPortDiscovery
// Validates: VK-8.1, VK-8.2
// Port is discoverable via ss after startup
// ----------------------------------------------------------------------------

func TestVibeKanbanPortDiscovery(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	port := discoverVibeKanbanPort(t, ctx)
	require.Greater(t, port, 0, "expected to discover a valid port for vibe-kanban")
	require.LessOrEqual(t, port, 65535, "port must be a valid TCP port")
}

// ----------------------------------------------------------------------------
// TestVibeKanbanCrashRecovery
// Validates: VK-3.3, VK-3.5
// Process restarts after being killed (kill + wait + verify running)
// ----------------------------------------------------------------------------

func TestVibeKanbanCrashRecovery(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	// Discover the current vibe-kanban port
	port := discoverVibeKanbanPort(t, ctx)
	require.Greater(t, port, 0, "must discover vibe-kanban port before killing")

	// Kill the vibe-kanban server process by finding what's listening on its port.
	// This avoids accidentally killing the supervisor script.
	_, err := docker.ExecInContainer(ctx, sharedClient, sharedContainerName,
		[]string{"bash", "-c", fmt.Sprintf(
			"PID=$(ss -tlnp sport = :%d | grep -oP 'pid=\\K[0-9]+' | head -1); [ -n \"$PID\" ] && kill $PID; exit 0",
			port)})
	require.NoError(t, err, "exec kill vibe-kanban via port lookup")

	// Wait for the supervisor to restart it (DELAY_SECONDS=5 + startup + port discovery)
	time.Sleep(30 * time.Second)

	// Verify the server is running again by reading the port file
	newPort := discoverVibeKanbanPort(t, ctx)
	require.Greater(t, newPort, 0, "expected vibe-kanban process to be running after crash recovery")
}

// ----------------------------------------------------------------------------
// TestVibeKanbanAccessibleFromHost
// Validates: VK-8.1, VK-8.2
// HTTP GET to localhost:port returns 2xx (host network mode)
// ----------------------------------------------------------------------------

func TestVibeKanbanAccessibleFromHost(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	ctx := context.Background()

	port := discoverVibeKanbanPort(t, ctx)
	require.Greater(t, port, 0, "must discover vibe-kanban port before testing HTTP access")

	url := fmt.Sprintf("http://localhost:%d", port)

	// Retry HTTP GET for up to 15 seconds (server may need time after restart)
	var resp *http.Response
	var httpErr error
	deadline := time.Now().Add(15 * time.Second)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, httpErr = client.Get(url)
		if httpErr == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			return // Success
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
	}

	if httpErr != nil {
		t.Fatalf("HTTP GET %s failed: %v", url, httpErr)
	}
	if resp != nil {
		t.Fatalf("HTTP GET %s returned status %d, expected 2xx", url, resp.StatusCode)
	}
	t.Fatalf("HTTP GET %s timed out without a successful response", url)
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

// discoverVibeKanbanPort reads the port file written by the supervisor script.
// Retries for up to 60 seconds since the supervisor needs time to start
// vibe-kanban and discover its port.
func discoverVibeKanbanPort(t *testing.T, ctx context.Context) int {
	t.Helper()

	const portFile = "/tmp/vibe-kanban.port"
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		exitCode, output, err := docker.ExecInContainerWithOutput(ctx, sharedClient, sharedContainerName,
			[]string{"cat", portFile})
		if err != nil {
			t.Logf("cat port file error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}
		if exitCode == 0 {
			portStr := strings.TrimSpace(output)
			port, err := strconv.Atoi(portStr)
			if err == nil && port > 0 {
				return port
			}
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatal("timed out waiting for vibe-kanban port file (60s)")
	return 0
}

func findFreePortVK() (int, error) {
	for port := constants.SSHPortStart; port < 65535; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found starting at %d", constants.SSHPortStart)
}

func sanitizeVK(s string) string {
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
