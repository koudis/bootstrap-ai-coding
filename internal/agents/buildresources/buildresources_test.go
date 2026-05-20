package buildresources_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"

	// Blank import triggers init() registration
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/buildresources"
)

func testInfo() *hostinfo.Info {
	return &hostinfo.Info{
		Username: "testuser",
		HomeDir:  "/home/testuser",
		UID:      1000,
		GID:      1000,
	}
}

func getAgent(t *testing.T) agent.Agent {
	t.Helper()
	a, err := agent.Lookup(constants.BuildResourcesAgentName)
	require.NoError(t, err, "build-resources agent must be registered")
	return a
}

func TestID(t *testing.T) {
	a := getAgent(t)
	require.Equal(t, "build-resources", a.ID())
}

func TestCredentialStorePath(t *testing.T) {
	a := getAgent(t)
	require.Equal(t, "", a.CredentialStorePath())
}

func TestContainerMountPath(t *testing.T) {
	a := getAgent(t)
	require.Equal(t, "", a.ContainerMountPath("/home/testuser"))
}

func TestHasCredentials(t *testing.T) {
	a := getAgent(t)
	has, err := a.HasCredentials("")
	require.NoError(t, err)
	require.True(t, has, "HasCredentials must always return true for build-resources")
}

func TestInstallAppendsExpectedPackages(t *testing.T) {
	a := getAgent(t)
	info := testInfo()
	b := docker.NewBaseImageBuilder(
		info,
		docker.UserStrategyCreate, "",
		"",
	)

	a.Install(b)
	content := b.Build()

	// Verify apt packages are present
	expectedPackages := []string{
		"python3", "python3-pip", "python3-venv", "python3-dev",
		"python3-setuptools", "python3-wheel",
		"build-essential", "cmake", "pkg-config",
		"default-jdk",
		"libssl-dev", "libffi-dev",
		"curl", "ca-certificates", "unzip", "wget",
	}
	for _, pkg := range expectedPackages {
		require.Contains(t, content, pkg,
			"Install() must include package %q", pkg)
	}

	// Verify Go tarball download
	require.Contains(t, content, "go.dev/dl/go",
		"Install() must download Go from go.dev")
	require.Contains(t, content, "/usr/local",
		"Install() must extract Go to /usr/local")

	// Verify Go PATH setup
	require.Contains(t, content, "/etc/profile.d/golang.sh",
		"Install() must create golang.sh profile script")
	require.Contains(t, content, "/usr/local/go/bin",
		"Install() must add /usr/local/go/bin to PATH")

	// Verify uv installation
	require.Contains(t, content, "astral.sh/uv/install.sh",
		"Install() must install uv via official installer")
	require.Contains(t, content, "UV_INSTALL_DIR=/usr/local/bin",
		"Install() must install uv to /usr/local/bin")
}

// TestSummaryInfoReturnsNil verifies that the Build Resources agent's SummaryInfo
// method returns (nil, nil) since it has no additional session summary info.
// Validates: SI-6.3
func TestSummaryInfoReturnsNil(t *testing.T) {
	a := getAgent(t)

	info, err := a.SummaryInfo(context.Background(), nil, "")
	require.NoError(t, err)
	require.Nil(t, info)
}

func TestInstallUsesSystemWidePaths(t *testing.T) {
	a := getAgent(t)
	info := testInfo()
	b := docker.NewBaseImageBuilder(
		info,
		docker.UserStrategyCreate, "",
		"",
	)

	a.Install(b)
	lines := b.Lines()

	// Verify no USER directives are emitted by Install() — everything runs as root
	var userLinesFromInstall []string
	// The base builder emits lines before Install() is called; count lines after
	// the base builder's output
	baseBuilder := docker.NewBaseImageBuilder(
		info,
		docker.UserStrategyCreate, "",
		"",
	)
	baseLineCount := len(baseBuilder.Lines())

	for _, line := range lines[baseLineCount:] {
		if strings.HasPrefix(line, "USER ") {
			userLinesFromInstall = append(userLinesFromInstall, line)
		}
	}

	require.Empty(t, userLinesFromInstall,
		"Install() should not emit USER directives — all tools installed system-wide as root")
}
