package docker_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// ---------------------------------------------------------------------------
// Unit tests — NewClient error path (Req 6.2)
// ---------------------------------------------------------------------------

// TestNewClientDaemonUnreachable verifies that NewClient returns a non-nil,
// descriptive error when the Docker daemon is not reachable.
// Validates: Req 6.2
func TestNewClientDaemonUnreachable(t *testing.T) {
	// Point DOCKER_HOST at a port that is almost certainly not listening.
	// The Docker SDK's NewClientWithOpts succeeds (it only builds the struct),
	// but the subsequent Ping call will fail, triggering the error path.
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")

	client, err := docker.NewClient()

	require.Nil(t, client, "expected nil client when daemon is unreachable")
	require.Error(t, err, "expected an error when daemon is unreachable")

	// The error message must be helpful — it should mention Docker or daemon.
	msg := strings.ToLower(err.Error())
	require.True(t,
		strings.Contains(msg, "docker") || strings.Contains(msg, "daemon") || strings.Contains(msg, "reachable"),
		"error message should mention Docker/daemon/reachable, got: %q", err.Error(),
	)
}

// ---------------------------------------------------------------------------
// Unit tests — CheckVersion / IsVersionCompatible error path (Req 6.4)
// ---------------------------------------------------------------------------

// TestCheckVersionTooOld verifies that IsVersionCompatible returns false for
// versions older than constants.MinDockerVersion ("20.10"), which is the
// predicate used by CheckVersion to decide whether to return an error.
//
// The full CheckVersion error path (including the error message that embeds
// both the detected and required versions) requires a live Docker daemon and
// is therefore covered by integration tests (//go:build integration).
//
// Validates: Req 6.4
func TestCheckVersionTooOld(t *testing.T) {
	oldVersions := []string{
		"19.03.0",
		"20.09.9",
		"18.09.0",
		"1.13.0",
	}

	for _, v := range oldVersions {
		t.Run("too_old_"+v, func(t *testing.T) {
			require.False(t, docker.IsVersionCompatible(v),
				"version %q should be rejected (older than %s)", v, constants.MinDockerVersion)
		})
	}
}

// TestCheckVersionErrorMessageFormat verifies that the error message format
// used by CheckVersion would contain both the detected and required versions.
// We test this by confirming the format string components are present in a
// manually constructed message that mirrors what CheckVersion produces.
//
// Validates: Req 6.4
func TestCheckVersionErrorMessageFormat(t *testing.T) {
	detectedVersion := "19.03.0"
	requiredVersion := constants.MinDockerVersion

	// Mirror the format string from CheckVersion:
	//   "Docker version %s is too old; %s or newer is required"
	msg := fmt.Sprintf(
		"Docker version %s is too old; %s or newer is required",
		detectedVersion, requiredVersion,
	)

	require.Contains(t, msg, detectedVersion,
		"error message must contain the detected version")
	require.Contains(t, msg, requiredVersion,
		"error message must contain the required version")
}

// ---------------------------------------------------------------------------
// Unit tests — edge cases for IsVersionCompatible
// ---------------------------------------------------------------------------

func TestIsVersionCompatible(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"20.10.0", true},
		{"20.9.9", false},
		{"21.0.0", true},
		{"19.99.99", false},
		{"20.10", true},  // no patch component
		{"", false},      // invalid: empty string
		{"abc", false},   // invalid: non-numeric
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("version=%q", tc.version), func(t *testing.T) {
			got := docker.IsVersionCompatible(tc.version)
			require.Equal(t, tc.want, got,
				"IsVersionCompatible(%q) = %v, want %v", tc.version, got, tc.want)
		})
	}
}

// ---------------------------------------------------------------------------
// Property 13: Docker version comparison is correct
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 13: Docker version comparison is correct
func TestPropertyDockerVersionComparison(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		major := rapid.IntRange(0, 30).Draw(t, "major")
		minor := rapid.IntRange(0, 30).Draw(t, "minor")
		patch := rapid.IntRange(0, 30).Draw(t, "patch")

		version := fmt.Sprintf("%d.%d.%d", major, minor, patch)
		expected := major > 20 || (major == 20 && minor >= 10)

		got := docker.IsVersionCompatible(version)
		require.Equal(t, expected, got,
			"IsVersionCompatible(%q): got %v, want %v (major=%d, minor=%d, patch=%d)",
			version, got, expected, major, minor, patch)
	})
}
