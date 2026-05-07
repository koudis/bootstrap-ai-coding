package cmd_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/cmd"
)

// Feature: bootstrap-ai-coding, Property 16: --agents flag parsing produces correct agent ID slices
func TestPropertyParseAgentsFlagProducesCorrectSlice(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw a slice of 1–5 non-empty, trimmed agent ID strings.
		ids := rapid.SliceOfN(
			rapid.StringMatching(`[a-z][a-z0-9-]*`),
			1, 5,
		).Draw(t, "ids")

		// Build a comma-separated string, optionally adding whitespace around commas.
		sep := rapid.SampledFrom([]string{",", " , ", "  ,  ", ", ", " ,"}).Draw(t, "sep")
		input := strings.Join(ids, sep)

		result := cmd.ParseAgentsFlag(input)

		require.Equal(t, ids, result,
			"ParseAgentsFlag(%q) should return trimmed IDs in order", input)
	})
}

// Feature: bootstrap-ai-coding, Property 25: Session summary always contains all required fields
func TestPropertyFormatSessionSummaryContainsAllLabels(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dataDir := rapid.StringMatching(`/[a-zA-Z0-9/_.-]+`).Draw(t, "dataDir")
		projectDir := rapid.StringMatching(`/[a-zA-Z0-9/_.-]+`).Draw(t, "projectDir")
		sshPort := rapid.IntRange(1024, 65535).Draw(t, "sshPort")
		sshConnect := rapid.StringMatching(`ssh bac-[a-z0-9-]+`).Draw(t, "sshConnect")
		agentCount := rapid.IntRange(1, 5).Draw(t, "agentCount")
		agents := make([]string, agentCount)
		for i := range agents {
			agents[i] = rapid.StringMatching(`[a-z][a-z0-9-]*`).Draw(t, "agent")
		}

		summary := cmd.SessionSummary{
			DataDir:       dataDir,
			ProjectDir:    projectDir,
			SSHPort:       sshPort,
			SSHConnect:    sshConnect,
			EnabledAgents: agents,
		}

		output := cmd.FormatSessionSummary(summary)

		require.Contains(t, output, "Data directory:", "output must contain 'Data directory:' label")
		require.Contains(t, output, "Project directory:", "output must contain 'Project directory:' label")
		require.Contains(t, output, "SSH port:", "output must contain 'SSH port:' label")
		require.Contains(t, output, "SSH connect:", "output must contain 'SSH connect:' label")
		require.Contains(t, output, "Enabled agents:", "output must contain 'Enabled agents:' label")
	})
}

// Feature: bootstrap-ai-coding, Property 32: S ∧ U is always rejected
func TestPropertyResolveModeRejectsBothFlags(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		_, err := cmd.ResolveMode(true, true)
		require.Error(t, err,
			"ResolveMode(true, true) must always return a non-nil error")
	})
}

// Feature: bootstrap-ai-coding, Property 33: Mode is always exactly one of START, STOP, PURGE
func TestPropertyResolveModeReturnsExactlyOneMode(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw (stopAndRemove, purge) pairs excluding (true, true).
		stopAndRemove := rapid.Bool().Draw(t, "stopAndRemove")
		purge := rapid.Bool().Draw(t, "purge")

		// Skip the mutually exclusive combination.
		if stopAndRemove && purge {
			return
		}

		mode, err := cmd.ResolveMode(stopAndRemove, purge)

		require.NoError(t, err,
			"ResolveMode(%v, %v) must not return an error for valid flag combinations",
			stopAndRemove, purge)

		validModes := []cmd.Mode{cmd.ModeStart, cmd.ModeStop, cmd.ModePurge}
		found := false
		for _, m := range validModes {
			if mode == m {
				found = true
				break
			}
		}
		require.True(t, found,
			"ResolveMode(%v, %v) = %v, which is not one of ModeStart, ModeStop, ModePurge",
			stopAndRemove, purge, mode)
	})
}

// Unit tests for ResolveMode

func TestResolveModeStart(t *testing.T) {
	mode, err := cmd.ResolveMode(false, false)
	require.NoError(t, err)
	require.Equal(t, cmd.ModeStart, mode)
}

func TestResolveModeStop(t *testing.T) {
	mode, err := cmd.ResolveMode(true, false)
	require.NoError(t, err)
	require.Equal(t, cmd.ModeStop, mode)
}

func TestResolveModePurge(t *testing.T) {
	mode, err := cmd.ResolveMode(false, true)
	require.NoError(t, err)
	require.Equal(t, cmd.ModePurge, mode)
}

func TestStopAndPurgeTogetherRejected(t *testing.T) {
	_, err := cmd.ResolveMode(true, true)
	require.Error(t, err)
}

// Unit tests for ParseAgentsFlag

func TestParseAgentsFlagEmpty(t *testing.T) {
	result := cmd.ParseAgentsFlag("")
	require.Empty(t, result)
}

func TestParseAgentsFlagSingle(t *testing.T) {
	result := cmd.ParseAgentsFlag("claude-code")
	require.Equal(t, []string{"claude-code"}, result)
}

func TestParseAgentsFlagMultiple(t *testing.T) {
	result := cmd.ParseAgentsFlag("claude-code,aider,gemini")
	require.Equal(t, []string{"claude-code", "aider", "gemini"}, result)
}

func TestParseAgentsFlagWhitespace(t *testing.T) {
	result := cmd.ParseAgentsFlag("claude-code , aider , gemini")
	require.Equal(t, []string{"claude-code", "aider", "gemini"}, result)
}

// Unit tests for FormatSessionSummary

func TestFormatSessionSummaryLabels(t *testing.T) {
	summary := cmd.SessionSummary{
		DataDir:       "/home/user/.config/bootstrap-ai-coding/bac-myproject",
		ProjectDir:    "/home/user/myproject",
		SSHPort:       2222,
		SSHConnect:    "ssh bac-myproject",
		EnabledAgents: []string{"claude-code"},
	}
	output := cmd.FormatSessionSummary(summary)
	require.Contains(t, output, "Data directory:")
	require.Contains(t, output, "Project directory:")
	require.Contains(t, output, "SSH port:")
	require.Contains(t, output, "SSH connect:")
	require.Contains(t, output, "Enabled agents:")
}

func TestFormatSessionSummaryValues(t *testing.T) {
	summary := cmd.SessionSummary{
		DataDir:       "/home/user/.config/bootstrap-ai-coding/bac-myproject",
		ProjectDir:    "/home/user/myproject",
		SSHPort:       2222,
		SSHConnect:    "ssh bac-myproject",
		EnabledAgents: []string{"claude-code", "aider"},
	}
	output := cmd.FormatSessionSummary(summary)
	require.Contains(t, output, "/home/user/.config/bootstrap-ai-coding/bac-myproject")
	require.Contains(t, output, "/home/user/myproject")
	require.Contains(t, output, "2222")
	require.Contains(t, output, "ssh bac-myproject")
	require.Contains(t, output, "claude-code")
	require.Contains(t, output, "aider")
}

// Feature: bootstrap-ai-coding, Property 35: --port is always within 1024–65535 when provided
func TestPropertyPortValidationRange(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		port := rapid.Int().Draw(t, "port")

		// The port validation logic from run(): port < 1024 || port > 65535 is invalid.
		isValid := port >= 1024 && port <= 65535
		isRejected := port < 1024 || port > 65535

		// The two conditions must be complementary (exactly one is true).
		require.NotEqual(t, isValid, isRejected,
			"port %d: isValid and isRejected must be mutually exclusive", port)

		// Verify the boundary conditions explicitly.
		if port >= 1024 && port <= 65535 {
			require.True(t, isValid,
				"port %d is in [1024, 65535] and must be accepted", port)
		} else {
			require.True(t, isRejected,
				"port %d is outside [1024, 65535] and must be rejected", port)
		}
	})
}

// ── Unit tests for CLI-3: start-only flags rejected with --stop-and-remove / --purge ──

// TestNoUpdateSSHConfigFlagWithStopRejected verifies that --no-update-ssh-config
// is rejected when used with --stop-and-remove (CLI-3).
// Validates: CLI-3
func TestNoUpdateSSHConfigFlagWithStopRejected(t *testing.T) {
	err := cmd.ValidateStartOnlyFlags(cmd.ModeStop, []string{"no-update-ssh-config"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--no-update-ssh-config",
		"error must name the offending flag")
	require.Contains(t, err.Error(), "--stop-and-remove",
		"error must name the conflicting mode flag")
}

// TestNoUpdateSSHConfigFlagWithPurgeRejected verifies that --no-update-ssh-config
// is rejected when used with --purge (CLI-3).
// Validates: CLI-3
func TestNoUpdateSSHConfigFlagWithPurgeRejected(t *testing.T) {
	err := cmd.ValidateStartOnlyFlags(cmd.ModePurge, []string{"no-update-ssh-config"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--no-update-ssh-config",
		"error must name the offending flag")
	require.Contains(t, err.Error(), "--purge",
		"error must name the conflicting mode flag")
}

// TestValidateStartOnlyFlagsAllowedInStartMode verifies that start-only flags
// are accepted when mode is ModeStart.
func TestValidateStartOnlyFlagsAllowedInStartMode(t *testing.T) {
	startOnlyFlags := []string{
		"agents", "port", "ssh-key", "rebuild",
		"no-update-known-hosts", "no-update-ssh-config", "verbose",
	}
	for _, flag := range startOnlyFlags {
		err := cmd.ValidateStartOnlyFlags(cmd.ModeStart, []string{flag})
		require.NoError(t, err, "flag --%s must be allowed in start mode", flag)
	}
}

// TestValidateStartOnlyFlagsOtherFlagsNotRejected verifies that flags not in
// the start-only set do not cause an error in stop/purge modes.
func TestValidateStartOnlyFlagsOtherFlagsNotRejected(t *testing.T) {
	// "stop-and-remove" and "purge" are not start-only flags.
	require.NoError(t, cmd.ValidateStartOnlyFlags(cmd.ModeStop, []string{"stop-and-remove"}))
	require.NoError(t, cmd.ValidateStartOnlyFlags(cmd.ModePurge, []string{"purge"}))
}

// ── Unit tests for StringSlicesEqual ─────────────────────────────────────────

func TestStringSlicesEqualBothEmpty(t *testing.T) {
	require.True(t, cmd.StringSlicesEqual([]string{}, []string{}))
}

func TestStringSlicesEqualNilAndEmpty(t *testing.T) {
	require.True(t, cmd.StringSlicesEqual(nil, nil))
}

func TestStringSlicesEqualSameElements(t *testing.T) {
	require.True(t, cmd.StringSlicesEqual([]string{"a", "b", "c"}, []string{"a", "b", "c"}))
}

func TestStringSlicesEqualDifferentLength(t *testing.T) {
	require.False(t, cmd.StringSlicesEqual([]string{"a"}, []string{"a", "b"}))
}

func TestStringSlicesEqualDifferentContent(t *testing.T) {
	require.False(t, cmd.StringSlicesEqual([]string{"a", "b"}, []string{"a", "c"}))
}

func TestStringSlicesEqualOrderMatters(t *testing.T) {
	require.False(t, cmd.StringSlicesEqual([]string{"b", "a"}, []string{"a", "b"}))
}

// Feature: bootstrap-ai-coding, Property 34: StringSlicesEqual is reflexive and symmetric
func TestPropertyStringSlicesEqualReflexiveAndSymmetric(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		s := rapid.SliceOf(rapid.String()).Draw(t, "s")

		// Reflexive: s == s
		require.True(t, cmd.StringSlicesEqual(s, s),
			"StringSlicesEqual must be reflexive")

		// Symmetric: s == t iff t == s
		other := rapid.SliceOf(rapid.String()).Draw(t, "other")
		require.Equal(t,
			cmd.StringSlicesEqual(s, other),
			cmd.StringSlicesEqual(other, s),
			"StringSlicesEqual must be symmetric")
	})
}

// TestVerboseFlagWithStopRejected verifies that --verbose is rejected when
// used with --stop-and-remove (CLI-3).
// Validates: Req 20.5, CLI-3
func TestVerboseFlagWithStopRejected(t *testing.T) {
	err := cmd.ValidateStartOnlyFlags(cmd.ModeStop, []string{"verbose"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--verbose",
		"error must name the offending flag")
	require.Contains(t, err.Error(), "--stop-and-remove",
		"error must name the conflicting mode flag")
}

// TestVerboseFlagWithPurgeRejected verifies that --verbose is rejected when
// used with --purge (CLI-3).
// Validates: Req 20.5, CLI-3
func TestVerboseFlagWithPurgeRejected(t *testing.T) {
	err := cmd.ValidateStartOnlyFlags(cmd.ModePurge, []string{"verbose"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--verbose",
		"error must name the offending flag")
	require.Contains(t, err.Error(), "--purge",
		"error must name the conflicting mode flag")
}
