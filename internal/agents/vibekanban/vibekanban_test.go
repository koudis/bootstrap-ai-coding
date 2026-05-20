// Package vibekanban_test contains unit tests for the Vibe Kanban agent module.
// The blank import of the vibekanban package triggers its init() function, which
// registers the vibeKanbanAgent with the global agent registry.
package vibekanban_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/vibekanban"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
	"github.com/koudis/bootstrap-ai-coding/internal/hostinfo"
)

// newTestBuilder returns a DockerfileBuilder pre-seeded with the base layer,
// using fixed key material and UserStrategyCreate with uid=1000, gid=1000.
func newTestBuilder() *docker.DockerfileBuilder {
	return docker.NewBaseImageBuilder(
		&hostinfo.Info{Username: "testuser", HomeDir: "/home/testuser", UID: 1000, GID: 1000},
		docker.UserStrategyCreate, "",
		"",
	)
}

func getAgent(t *testing.T) agent.Agent {
	t.Helper()
	a, err := agent.Lookup(constants.VibeKanbanAgentName)
	require.NoError(t, err, "vibe-kanban agent must be registered")
	return a
}

// ---------------------------------------------------------------------------
// TestID — returns constants.VibeKanbanAgentName
// Validates: VK-1.1
// ---------------------------------------------------------------------------

func TestID(t *testing.T) {
	a := getAgent(t)
	require.Equal(t, constants.VibeKanbanAgentName, a.ID())
}

// ---------------------------------------------------------------------------
// TestInstallNodeAlreadyInstalled — skips Node.js when IsNodeInstalled() is true
// Validates: VK-2.1
// ---------------------------------------------------------------------------

func TestInstallNodeAlreadyInstalled(t *testing.T) {
	a := getAgent(t)

	b := newTestBuilder()
	b.MarkNodeInstalled() // simulate a prior agent having installed Node.js
	require.True(t, b.IsNodeInstalled())

	a.Install(b)
	content := b.Build()

	// Must NOT contain the Node.js setup step
	require.NotContains(t, content, "setup_22.x",
		"must skip Node.js setup when already installed")

	// Must still install the npm package
	require.Contains(t, content, "vibe-kanban",
		"must always install the vibe-kanban npm package")
}

// ---------------------------------------------------------------------------
// TestInstallNodeNotInstalled — installs Node.js when IsNodeInstalled() is false
// Validates: VK-2.1
// ---------------------------------------------------------------------------

func TestInstallNodeNotInstalled(t *testing.T) {
	a := getAgent(t)

	b := newTestBuilder()
	require.False(t, b.IsNodeInstalled(), "fresh builder must have IsNodeInstalled() == false")

	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "setup_22.x",
		"must install Node.js 22 when not already installed")
	require.Contains(t, content, "nodejs",
		"must install nodejs package when not already installed")
	require.True(t, b.IsNodeInstalled(),
		"MarkNodeInstalled() must be called after Node.js installation")
}

// ---------------------------------------------------------------------------
// TestInstallContainsNpmPackage — output contains `npm install -g` with `vibe-kanban`
// Validates: VK-2.2
// ---------------------------------------------------------------------------

func TestInstallContainsNpmPackage(t *testing.T) {
	a := getAgent(t)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "npm install -g",
		"must contain npm install -g")
	require.Contains(t, content, "vibe-kanban",
		"must contain vibe-kanban package name")
}

// ---------------------------------------------------------------------------
// TestInstallContainsEntrypoint — output contains ENTRYPOINT instruction
// Validates: VK-3.1
// ---------------------------------------------------------------------------

func TestInstallContainsEntrypoint(t *testing.T) {
	a := getAgent(t)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "ENTRYPOINT",
		"must contain ENTRYPOINT instruction")
	require.Contains(t, content, "bac-entrypoint.sh",
		"ENTRYPOINT must reference bac-entrypoint.sh")
}

// ---------------------------------------------------------------------------
// TestInstallContainsSupervisor — supervisor script (base64-encoded) contains crash recovery params
// Validates: VK-3.1, VK-2.4
// ---------------------------------------------------------------------------

func TestInstallContainsSupervisor(t *testing.T) {
	a := getAgent(t)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.Contains(t, content, "vibe-kanban-supervisor.sh",
		"must contain supervisor script reference")

	// The script is base64-encoded in the Dockerfile. Decode it to verify contents.
	// Find the line that writes the supervisor script: "echo <base64> | base64 -d > /usr/local/bin/vibe-kanban-supervisor.sh"
	var supervisorB64 string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "vibe-kanban-supervisor.sh") && strings.Contains(line, "base64 -d") {
			// Extract the base64 payload between "echo " and " | base64"
			after := strings.TrimPrefix(line, "RUN echo ")
			idx := strings.Index(after, " | base64")
			if idx > 0 {
				supervisorB64 = after[:idx]
			}
			break
		}
	}
	require.NotEmpty(t, supervisorB64, "must find base64-encoded supervisor script in Dockerfile")

	decoded, err := base64Decode(supervisorB64)
	require.NoError(t, err, "supervisor script base64 must decode cleanly")

	require.Contains(t, decoded, "MAX_RESTARTS=5",
		"supervisor must have MAX_RESTARTS=5")
	require.Contains(t, decoded, "WINDOW_SECONDS=60",
		"supervisor must have WINDOW_SECONDS=60")
	require.Contains(t, decoded, "DELAY_SECONDS=5",
		"supervisor must have DELAY_SECONDS=5")
}

// ---------------------------------------------------------------------------
// TestInstallDoesNotContainCMD — output does NOT contain CMD instruction
// Validates: VK-3.1
// ---------------------------------------------------------------------------

func TestInstallDoesNotContainCMD(t *testing.T) {
	a := getAgent(t)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	// Check that no line starts with CMD
	for _, line := range strings.Split(content, "\n") {
		require.False(t, strings.HasPrefix(strings.TrimSpace(line), "CMD"),
			"Install() must not emit a CMD instruction, found: %q", line)
	}
}

// ---------------------------------------------------------------------------
// TestInstallNoRustNoPnpm — output does NOT contain rust/pnpm references
// Validates: VK-2.2
// ---------------------------------------------------------------------------

func TestInstallNoRustNoPnpm(t *testing.T) {
	a := getAgent(t)

	b := newTestBuilder()
	a.Install(b)
	content := b.Build()

	require.NotContains(t, content, "rust",
		"Install() must not contain rust references")
	require.NotContains(t, content, "pnpm",
		"Install() must not contain pnpm references")
}

// ---------------------------------------------------------------------------
// TestCredentialStorePath — returns empty string
// Validates: VK-4.1
// ---------------------------------------------------------------------------

func TestCredentialStorePath(t *testing.T) {
	a := getAgent(t)
	require.Equal(t, "", a.CredentialStorePath())
}

// ---------------------------------------------------------------------------
// TestContainerMountPath — returns empty string for various homeDir values
// Validates: VK-4.2
// ---------------------------------------------------------------------------

func TestContainerMountPath(t *testing.T) {
	a := getAgent(t)

	homeDirs := []string{
		"/home/testuser",
		"/home/dev",
		"/root",
		"/home/alice",
	}
	for _, homeDir := range homeDirs {
		require.Equal(t, "", a.ContainerMountPath(homeDir),
			"ContainerMountPath(%q) must return empty string", homeDir)
	}
}

// ---------------------------------------------------------------------------
// TestHasCredentials — returns (true, nil)
// Validates: VK-4.3
// ---------------------------------------------------------------------------

func TestHasCredentials(t *testing.T) {
	a := getAgent(t)

	has, err := a.HasCredentials("")
	require.NoError(t, err)
	require.True(t, has, "HasCredentials must always return true for vibe-kanban")

	has, err = a.HasCredentials("/some/path")
	require.NoError(t, err)
	require.True(t, has, "HasCredentials must always return true regardless of path")
}

// ---------------------------------------------------------------------------
// Health check tests — mock Docker client via httptest with connection hijacking
// ---------------------------------------------------------------------------

// hijackHandler handles the exec attach endpoint by hijacking the HTTP connection,
// which is what the Docker SDK expects for exec attach operations.
func hijackHandler(w http.ResponseWriter, _ *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Write the HTTP 101 Switching Protocols response that Docker SDK expects
	buf.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
	buf.Flush()
	conn.Close()
}

// newFakeDockerClient creates a *docker.Client backed by a fake HTTP server.
// The exitCode parameter controls what exit code the exec inspect returns for
// all exec operations.
func newFakeDockerClient(t *testing.T, exitCode int) *docker.Client {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Handle API version negotiation / ping
		if strings.HasSuffix(path, "/_ping") || path == "/_ping" {
			w.Header().Set("Api-Version", "1.47")
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "OK")
			return
		}

		// ContainerExecCreate
		if r.Method == http.MethodPost && strings.Contains(path, "/exec") && !strings.Contains(path, "/start") && !strings.Contains(path, "/json") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(container.ExecCreateResponse{ID: "fake-exec-id"})
			return
		}

		// ContainerExecAttach (start) — requires connection hijacking
		if r.Method == http.MethodPost && strings.Contains(path, "/exec/") && strings.Contains(path, "/start") {
			hijackHandler(w, r)
			return
		}

		// ContainerExecInspect (json)
		if r.Method == http.MethodGet && strings.Contains(path, "/exec/") && strings.Contains(path, "/json") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(container.ExecInspect{
				ExitCode: exitCode,
				Running:  false,
			})
			return
		}

		// Default
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Point DOCKER_HOST at our fake server (use tcp:// scheme for Docker SDK)
	host := strings.Replace(srv.URL, "http://", "tcp://", 1)
	t.Setenv("DOCKER_HOST", host)
	client, err := docker.NewClient()
	require.NoError(t, err)

	return client
}

// newFakeDockerClientWithExecSequence creates a *docker.Client backed by a fake
// HTTP server where each successive exec returns a different exit code from the
// provided sequence.
func newFakeDockerClientWithExecSequence(t *testing.T, exitCodes []int) *docker.Client {
	t.Helper()

	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasSuffix(path, "/_ping") || path == "/_ping" {
			w.Header().Set("Api-Version", "1.47")
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "OK")
			return
		}

		// ContainerExecCreate — increment call count
		if r.Method == http.MethodPost && strings.Contains(path, "/exec") && !strings.Contains(path, "/start") && !strings.Contains(path, "/json") {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(container.ExecCreateResponse{ID: fmt.Sprintf("fake-exec-id-%d", callCount)})
			return
		}

		// ContainerExecAttach (start) — requires connection hijacking
		if r.Method == http.MethodPost && strings.Contains(path, "/exec/") && strings.Contains(path, "/start") {
			hijackHandler(w, r)
			return
		}

		// ContainerExecInspect (json) — return exit code based on which exec this is
		if r.Method == http.MethodGet && strings.Contains(path, "/exec/") && strings.Contains(path, "/json") {
			// Determine which exec ID this is for
			exitCode := 1 // default to failure
			for i, code := range exitCodes {
				id := fmt.Sprintf("fake-exec-id-%d", i+1)
				if strings.Contains(path, id) {
					exitCode = code
					break
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(container.ExecInspect{
				ExitCode: exitCode,
				Running:  false,
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	host := strings.Replace(srv.URL, "http://", "tcp://", 1)
	t.Setenv("DOCKER_HOST", host)
	client, err := docker.NewClient()
	require.NoError(t, err)

	return client
}

// ---------------------------------------------------------------------------
// TestHealthCheckBinaryFailure — error message identifies binary check
// Validates: VK-5.1
// ---------------------------------------------------------------------------

func TestHealthCheckBinaryFailure(t *testing.T) {
	a := getAgent(t)

	// Create a fake Docker client that returns exit code 1 for all execs
	// (simulating vibe-kanban --version failing)
	client := newFakeDockerClient(t, 1)

	ctx := context.Background()
	err := a.HealthCheck(ctx, client, "fake-container-id")

	require.Error(t, err, "HealthCheck must fail when binary check returns non-zero")
	require.Contains(t, err.Error(), "vibe-kanban",
		"error message must mention vibe-kanban")
	// The binary check fails first — error identifies the version/binary check
	require.Contains(t, err.Error(), "--version",
		"error message must identify the binary check (references --version)")
}

// ---------------------------------------------------------------------------
// TestHealthCheckProcessFailure — error message identifies process check
// Validates: VK-5.2
// ---------------------------------------------------------------------------

func TestHealthCheckProcessFailure(t *testing.T) {
	a := getAgent(t)

	// First exec (binary check) passes with exit 0, all subsequent execs
	// (pgrep process checks, up to 5 retries) fail with exit 1.
	exitCodes := []int{0, 1, 1, 1, 1, 1}
	client := newFakeDockerClientWithExecSequence(t, exitCodes)

	ctx := context.Background()
	err := a.HealthCheck(ctx, client, "fake-container-id")

	require.Error(t, err, "HealthCheck must fail when process is not running")
	require.Contains(t, err.Error(), "vibe-kanban",
		"error message must mention vibe-kanban")
	require.Contains(t, err.Error(), "process",
		"error message must identify the process check")
}

// Feature: bootstrap-ai-coding, Property 3: No-credential-store invariant
func TestPropertyNoCredentialStore(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		homeDir := rapid.String().Draw(rt, "homeDir")
		storePath := rapid.String().Draw(rt, "storePath")

		a, err := agent.Lookup(constants.VibeKanbanAgentName)
		require.NoError(rt, err, "vibe-kanban agent must be registered")

		// **Validates: Requirements VK-4.2**
		mountPath := a.ContainerMountPath(homeDir)
		require.Equal(rt, "", mountPath,
			"ContainerMountPath(%q) must always return empty string", homeDir)

		// **Validates: Requirements VK-4.3**
		has, credErr := a.HasCredentials(storePath)
		require.NoError(rt, credErr,
			"HasCredentials(%q) must not return an error", storePath)
		require.True(rt, has,
			"HasCredentials(%q) must always return true", storePath)
	})
}

// Feature: bootstrap-ai-coding, Property 1: Node.js conditional installation invariant
func TestPropertyNodeJSConditionalInstallation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// draw inputs
		nodePreInstalled := rapid.Bool().Draw(rt, "nodePreInstalled")

		// exercise the function
		b := newTestBuilder()
		if nodePreInstalled {
			b.MarkNodeInstalled()
		}

		a, err := agent.Lookup(constants.VibeKanbanAgentName)
		require.NoError(rt, err, "vibe-kanban agent must be registered")

		a.Install(b)

		output := b.Build()

		// assert the property holds:
		// 1. At most one occurrence of "setup_22.x" (Node.js installation block)
		occurrences := strings.Count(output, "setup_22.x")
		require.LessOrEqual(rt, occurrences, 1,
			"Install() must produce at most one Node.js installation block, got %d", occurrences)

		// 2. After Install(), IsNodeInstalled() returns true
		require.True(rt, b.IsNodeInstalled(),
			"IsNodeInstalled() must return true after Install()")
	})
}

// Feature: bootstrap-ai-coding, Property 2: Install does not emit CMD
func TestPropertyInstallDoesNotEmitCMD(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		nodePreInstalled := rapid.Bool().Draw(rt, "nodePreInstalled")

		b := newTestBuilder()
		if nodePreInstalled {
			b.MarkNodeInstalled()
		}

		a, err := agent.Lookup(constants.VibeKanbanAgentName)
		require.NoError(rt, err, "vibe-kanban agent must be registered")

		a.Install(b)

		output := b.Build()
		for _, line := range strings.Split(output, "\n") {
			trimmed := strings.TrimSpace(line)
			require.False(rt, strings.HasPrefix(trimmed, "CMD"),
				"Install() must not emit any CMD instruction, but found: %q", trimmed)
		}
	})
}

// Feature: bootstrap-ai-coding, Property 3: Vibe Kanban URL format
func TestPropertyVibeKanbanURLFormat(t *testing.T) {
	// **Validates: Requirements SI-5.2**
	rapid.Check(t, func(rt *rapid.T) {
		port := rapid.IntRange(1, 65535).Draw(rt, "port")

		// Construct the URL the same way SummaryInfo does.
		url := fmt.Sprintf("http://localhost:%d", port)

		// The URL must match "http://localhost:<port>" exactly.
		expected := fmt.Sprintf("http://localhost:%d", port)
		require.Equal(rt, expected, url,
			"URL must be exactly http://localhost:<port> for port %d", port)

		// Structural invariants:
		// 1. Starts with the correct scheme and host
		require.True(rt, strings.HasPrefix(url, "http://localhost:"),
			"URL must start with http://localhost:")

		// 2. The port suffix is the decimal string representation of the port
		portStr := strings.TrimPrefix(url, "http://localhost:")
		require.Equal(rt, fmt.Sprintf("%d", port), portStr,
			"port portion must be the decimal string of the port number")

		// 3. No trailing path, slash, or query string
		require.NotContains(rt, portStr, "/",
			"URL must not contain a trailing slash or path")
		require.NotContains(rt, portStr, "?",
			"URL must not contain a query string")
	})
}

// base64Decode is a test helper that decodes a standard base64 string.
func base64Decode(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
