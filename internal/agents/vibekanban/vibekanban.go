// Package vibekanban implements the Vibe Kanban agent module — a web-based
// project management tool that runs as a background service inside the container.
// It self-registers with the agent registry via init() and satisfies the
// agent.Agent interface. The core application has no direct dependency on
// this package — it is wired in exclusively via a blank import in main.go.
package vibekanban

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

type vibeKanbanAgent struct{}

func init() {
	agent.Register(&vibeKanbanAgent{})
}

// ID returns the stable Agent_ID "vibe-kanban".
// Satisfies: VK-1.1
func (a *vibeKanbanAgent) ID() string {
	return constants.VibeKanbanAgentName
}

// Install appends Dockerfile RUN steps that install Node.js (if not already
// installed), the vibe-kanban npm package, and the auto-start mechanism
// (supervisor script with crash recovery + ENTRYPOINT wrapper).
// Satisfies: VK-2.1, VK-2.2, VK-2.4, VK-3.1, VK-3.2, VK-3.5, VK-3.6
func (a *vibeKanbanAgent) Install(b *docker.DockerfileBuilder) {
	username := b.Username()

	// Node.js (conditional — skip if another agent already installed it)
	if !b.IsNodeInstalled() {
		b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl ca-certificates && rm -rf /var/lib/apt/lists/*")
		b.Run("curl -fsSL https://deb.nodesource.com/setup_22.x | bash - && DEBIAN_FRONTEND=noninteractive apt-get install -y nodejs && rm -rf /var/lib/apt/lists/*")
		b.MarkNodeInstalled()
	}

	// Runtime dependencies: iproute2 (ss for port discovery), procps (pgrep for health checks)
	b.Run("apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends iproute2 procps && rm -rf /var/lib/apt/lists/*")

	// Install vibe-kanban globally and pre-download the platform binary.
	// The timeout is short (15s) — enough for the download to complete and the
	// server to start (confirming the binary works), then timeout kills it.
	b.Run("npm install -g --no-fund --no-audit vibe-kanban")
	b.Run(fmt.Sprintf("su -c 'BROWSER=none timeout 15 vibe-kanban || true' %s", username))

	// Supervisor script with crash recovery (base64-encoded to avoid quoting issues)
	supervisorScript := buildSupervisorScript(username)
	supervisorB64 := base64.StdEncoding.EncodeToString([]byte(supervisorScript))
	b.Run(fmt.Sprintf("echo %s | base64 -d > /usr/local/bin/vibe-kanban-supervisor.sh && chmod +x /usr/local/bin/vibe-kanban-supervisor.sh",
		supervisorB64))

	// Entrypoint wrapper (base64-encoded to avoid quoting issues)
	entrypoint := buildEntrypointScript()
	entrypointB64 := base64.StdEncoding.EncodeToString([]byte(entrypoint))
	b.Run(fmt.Sprintf("echo %s | base64 -d > /usr/local/bin/bac-entrypoint.sh && chmod +x /usr/local/bin/bac-entrypoint.sh",
		entrypointB64))

	// ENTRYPOINT starts the supervisor before sshd
	b.Entrypoint("/usr/local/bin/bac-entrypoint.sh")
}

// buildSupervisorScript returns the supervisor shell script content.
// It substitutes the container username. The port is auto-assigned by
// vibe-kanban at startup (per VK-3.3) to avoid conflicts when multiple
// containers share the host network namespace.
// The script captures vibe-kanban's stdout to extract the auto-assigned port
// and writes it to a well-known file for SummaryInfo to read.
func buildSupervisorScript(username string) string {
	return fmt.Sprintf(`#!/bin/bash
MAX_RESTARTS=5
WINDOW_SECONDS=60
DELAY_SECONDS=5
PORT_FILE="/tmp/vibe-kanban.port"
LOG_FILE="/tmp/vibe-kanban.log"
RESTART_TIMES=()
while true; do
    NOW=$(date +%%%%s)
    PRUNED=()
    for ts in "${RESTART_TIMES[@]}"; do
        if (( NOW - ts < WINDOW_SECONDS )); then
            PRUNED+=("$ts")
        fi
    done
    RESTART_TIMES=("${PRUNED[@]}")
    if (( ${#RESTART_TIMES[@]} >= MAX_RESTARTS )); then
        echo "vibe-kanban-supervisor: exceeded $MAX_RESTARTS restarts in ${WINDOW_SECONDS}s, giving up" >&2
        exit 1
    fi
    RESTART_TIMES+=("$(date +%%%%s)")
    rm -f "$PORT_FILE"
    su -c "exec env BROWSER=none HOST=0.0.0.0 vibe-kanban" "%s" > "$LOG_FILE" 2>&1 &
    VK_PID=$!
    # Wait up to 30s for the port to appear in the log output
    for i in $(seq 1 30); do
        sleep 1
        if [ -f "$LOG_FILE" ]; then
            PORT=$(grep -oP 'Main server on :\K[0-9]+' "$LOG_FILE" 2>/dev/null | head -1)
            if [ -n "$PORT" ]; then
                echo "$PORT" > "$PORT_FILE"
                break
            fi
        fi
    done
    wait $VK_PID 2>/dev/null || true
    sleep "$DELAY_SECONDS"
done`, username)
}

// buildEntrypointScript returns the entrypoint wrapper script content.
func buildEntrypointScript() string {
	return `#!/bin/bash
set -e
/usr/local/bin/vibe-kanban-supervisor.sh &
exec "$@"`
}

// CredentialStorePath returns empty — no credentials to persist.
// Satisfies: VK-4.1
func (a *vibeKanbanAgent) CredentialStorePath() string {
	return ""
}

// ContainerMountPath returns empty — no bind-mount needed.
// Satisfies: VK-4.2
func (a *vibeKanbanAgent) ContainerMountPath(homeDir string) string {
	return ""
}

// HasCredentials always returns true — nothing to check.
// Satisfies: VK-4.3
func (a *vibeKanbanAgent) HasCredentials(storePath string) (bool, error) {
	return true, nil
}

// HealthCheck verifies that:
// 1. The vibe-kanban binary is present (vibe-kanban --version exits 0)
// 2. The vibe-kanban process is running (pgrep with retries)
// Satisfies: VK-5.1, VK-5.2
func (a *vibeKanbanAgent) HealthCheck(ctx context.Context, c *docker.Client, containerID string) error {
	// Check 1: Binary presence
	exitCode, err := docker.ExecInContainer(ctx, c, containerID, []string{"vibe-kanban", "--version"})
	if err != nil {
		return fmt.Errorf("vibe-kanban health check failed (binary): %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("vibe-kanban health check failed: 'vibe-kanban --version' exited with code %d", exitCode)
	}

	// Check 2: Process running (with retries)
	const maxRetries = 5
	const retryInterval = 2 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		exitCode, err = docker.ExecInContainer(ctx, c, containerID, []string{"pgrep", "-f", "vibe-kanban"})
		if err != nil {
			return fmt.Errorf("vibe-kanban health check failed (process check): %w", err)
		}
		if exitCode == 0 {
			return nil // Process is running
		}
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryInterval):
			}
		}
	}

	return fmt.Errorf("vibe-kanban health check failed: process not running after %d attempts", maxRetries)
}

// vibeKanbanPortFile is the well-known path where the supervisor writes
// the auto-assigned port after vibe-kanban starts.
const vibeKanbanPortFile = "/tmp/vibe-kanban.port"

// SummaryInfo discovers the port Vibe Kanban is listening on by reading
// the port file written by the supervisor script after startup.
// The port is auto-assigned by vibe-kanban at startup (VK-3.3, VK-9.1).
// Returns a single KeyValue with Key "Vibe Kanban" and Value "http://localhost:<port>".
// Retries for up to 30 seconds with 2-second intervals.
// Satisfies: SI-5.1, SI-5.2, SI-5.3, SI-5.4
func (a *vibeKanbanAgent) SummaryInfo(ctx context.Context, c *docker.Client, containerID string) ([]agent.KeyValue, error) {
	deadline := time.Now().Add(30 * time.Second)

	for time.Now().Before(deadline) {
		exitCode, output, err := docker.ExecInContainerWithOutput(ctx, c, containerID,
			[]string{"cat", vibeKanbanPortFile})
		if err != nil {
			return nil, err
		}
		if exitCode == 0 {
			portStr := strings.TrimSpace(output)
			port, err := strconv.Atoi(portStr)
			if err == nil && port > 0 && port <= 65535 {
				return []agent.KeyValue{
					{Key: "Vibe Kanban", Value: fmt.Sprintf("http://localhost:%d", port)},
				}, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("timed out after 30s waiting for vibe-kanban port file (%s)", vibeKanbanPortFile)
}
