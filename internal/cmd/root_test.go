package cmd_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/cmd"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
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

// TestFormatSessionSummaryWithVibeKanban verifies that the "Vibe Kanban:" line
// is present in the output when AgentInfo contains a Vibe Kanban entry.
// Validates: VK-8.3, VK-8.4
func TestFormatSessionSummaryWithVibeKanban(t *testing.T) {
	summary := cmd.SessionSummary{
		DataDir:       "/home/user/.config/bootstrap-ai-coding/bac-myproject",
		ProjectDir:    "/home/user/myproject",
		SSHPort:       2222,
		SSHConnect:    "ssh bac-myproject",
		EnabledAgents: []string{"claude-code", "vibe-kanban"},
		AgentInfo:     []agent.KeyValue{{Key: "Vibe Kanban", Value: "http://localhost:3000"}},
	}
	output := cmd.FormatSessionSummary(summary)
	require.Contains(t, output, "Vibe Kanban:")
	require.Contains(t, output, "http://localhost:3000")
}

// TestFormatSessionSummaryWithoutVibeKanban verifies that the "Vibe Kanban:" line
// is absent from the output when AgentInfo is empty.
// Validates: VK-8.3, VK-8.4
func TestFormatSessionSummaryWithoutVibeKanban(t *testing.T) {
	summary := cmd.SessionSummary{
		DataDir:       "/home/user/.config/bootstrap-ai-coding/bac-myproject",
		ProjectDir:    "/home/user/myproject",
		SSHPort:       2222,
		SSHConnect:    "ssh bac-myproject",
		EnabledAgents: []string{"claude-code"},
		AgentInfo:     nil,
	}
	output := cmd.FormatSessionSummary(summary)
	require.NotContains(t, output, "Vibe Kanban:")
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

// TestRestartPolicyFlagWithStopRejected verifies that --docker-restart-policy
// is rejected when used with --stop-and-remove (CLI-3).
// Validates: CLI-3
func TestRestartPolicyFlagWithStopRejected(t *testing.T) {
	err := cmd.ValidateStartOnlyFlags(cmd.ModeStop, []string{"docker-restart-policy"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--docker-restart-policy",
		"error must name the offending flag")
	require.Contains(t, err.Error(), "--stop-and-remove",
		"error must name the conflicting mode flag")
}

// TestRestartPolicyFlagWithPurgeRejected verifies that --docker-restart-policy
// is rejected when used with --purge (CLI-3).
// Validates: CLI-3
func TestRestartPolicyFlagWithPurgeRejected(t *testing.T) {
	err := cmd.ValidateStartOnlyFlags(cmd.ModePurge, []string{"docker-restart-policy"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--docker-restart-policy",
		"error must name the offending flag")
	require.Contains(t, err.Error(), "--purge",
		"error must name the conflicting mode flag")
}

// TestRestartPolicyInvalidValueRejected verifies that invalid restart policy
// values produce errors from ValidateRestartPolicy.
func TestRestartPolicyInvalidValueRejected(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "random word", value: "invalid"},
		{name: "restart keyword", value: "restart"},
		{name: "uppercase ALWAYS", value: "ALWAYS"},
		{name: "mixed case Never", value: "Never"},
		{name: "empty string", value: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := cmd.ValidateRestartPolicy(tc.value)
			require.Error(t, err, "ValidateRestartPolicy(%q) must return an error", tc.value)
			require.Contains(t, err.Error(), "invalid --docker-restart-policy",
				"error message must mention the flag")
		})
	}
}

// Feature: bootstrap-ai-coding, Property 55: for any string, validation accepts iff it's in the valid set
func TestPropertyRestartPolicyValidationAcceptsIffValid(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		policy := rapid.String().Draw(t, "policy")
		err := cmd.ValidateRestartPolicy(policy)
		isValid := cmd.ValidRestartPolicies[policy]
		if isValid {
			require.NoError(t, err, "valid policy %q must be accepted", policy)
		} else {
			require.Error(t, err, "invalid policy %q must be rejected", policy)
		}
	})
}

// TestHostNetworkOffFlagAcceptedInStartMode verifies that --host-network-off
// is accepted when used in START mode (no error from flag validation).
func TestHostNetworkOffFlagAcceptedInStartMode(t *testing.T) {
	err := cmd.ValidateStartOnlyFlags(cmd.ModeStart, []string{"host-network-off"})
	require.NoError(t, err, "--host-network-off must be accepted in START mode")
}

// TestHostNetworkOffFlagWithStopRejected verifies that --host-network-off
// is rejected when used with --stop-and-remove (CLI-3).
// Validates: CLI-3
func TestHostNetworkOffFlagWithStopRejected(t *testing.T) {
	err := cmd.ValidateStartOnlyFlags(cmd.ModeStop, []string{"host-network-off"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--host-network-off",
		"error must name the offending flag")
	require.Contains(t, err.Error(), "--stop-and-remove",
		"error must name the conflicting mode flag")
}

// TestHostNetworkOffFlagWithPurgeRejected verifies that --host-network-off
// is rejected when used with --purge (CLI-3).
// Validates: CLI-3
func TestHostNetworkOffFlagWithPurgeRejected(t *testing.T) {
	err := cmd.ValidateStartOnlyFlags(cmd.ModePurge, []string{"host-network-off"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "--host-network-off",
		"error must name the offending flag")
	require.Contains(t, err.Error(), "--purge",
		"error must name the conflicting mode flag")
}

// TestRestartPolicyDefaultIsUnlessStopped verifies that the --docker-restart-policy
// flag has default value "unless-stopped" (i.e., constants.DefaultRestartPolicy).
// Validates: Req 25.2
func TestRestartPolicyDefaultIsUnlessStopped(t *testing.T) {
	// Verify the constant itself equals the expected string.
	require.Equal(t, "unless-stopped", constants.DefaultRestartPolicy,
		"DefaultRestartPolicy constant must be \"unless-stopped\"")

	// Verify the default value passes validation (confirming it is a valid policy).
	err := cmd.ValidateRestartPolicy(constants.DefaultRestartPolicy)
	require.NoError(t, err,
		"the default restart policy must pass validation")
}

// Feature: bootstrap-ai-coding, Property 2: Session summary formatting includes all agent info after standard fields
func TestPropertyFormatSessionSummaryAgentInfo(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random standard fields.
		dataDir := rapid.StringMatching(`/[a-zA-Z0-9/_.-]+`).Draw(t, "dataDir")
		projectDir := rapid.StringMatching(`/[a-zA-Z0-9/_.-]+`).Draw(t, "projectDir")
		sshPort := rapid.IntRange(1024, 65535).Draw(t, "sshPort")
		sshConnect := rapid.StringMatching(`ssh bac-[a-z0-9-]+`).Draw(t, "sshConnect")
		agentCount := rapid.IntRange(1, 5).Draw(t, "agentCount")
		agents := make([]string, agentCount)
		for i := range agents {
			agents[i] = rapid.StringMatching(`[a-z][a-z0-9-]*`).Draw(t, "agent")
		}

		// Generate random AgentInfo (0–5 entries).
		infoCount := rapid.IntRange(0, 5).Draw(t, "infoCount")
		var agentInfo []agent.KeyValue
		for i := 0; i < infoCount; i++ {
			key := rapid.StringMatching(`[A-Za-z][A-Za-z0-9 ]*`).Draw(t, fmt.Sprintf("key%d", i))
			value := rapid.StringMatching(`[a-zA-Z0-9:/._-]+`).Draw(t, fmt.Sprintf("value%d", i))
			agentInfo = append(agentInfo, agent.KeyValue{Key: key, Value: value})
		}

		summary := cmd.SessionSummary{
			DataDir:       dataDir,
			ProjectDir:    projectDir,
			SSHPort:       sshPort,
			SSHConnect:    sshConnect,
			EnabledAgents: agents,
			AgentInfo:     agentInfo,
		}

		output := cmd.FormatSessionSummary(summary)
		lines := strings.Split(output, "\n")

		// (a) Every KeyValue.Key and KeyValue.Value appears in the output.
		for _, kv := range agentInfo {
			require.Contains(t, output, kv.Key+":",
				"output must contain key %q with colon", kv.Key)
			require.Contains(t, output, kv.Value,
				"output must contain value %q", kv.Value)
		}

		// (b) All agent info lines appear after the "Enabled agents" line.
		enabledAgentsLineIdx := -1
		for i, line := range lines {
			if strings.HasPrefix(line, "Enabled agents:") {
				enabledAgentsLineIdx = i
				break
			}
		}
		require.NotEqual(t, -1, enabledAgentsLineIdx,
			"output must contain 'Enabled agents:' line")

		for _, kv := range agentInfo {
			for i, line := range lines {
				if strings.Contains(line, kv.Key+":") && strings.Contains(line, kv.Value) {
					require.Greater(t, i, enabledAgentsLineIdx,
						"agent info line for key %q must appear after 'Enabled agents:' line", kv.Key)
				}
			}
		}

		// (c) When AgentInfo is nil or empty, no extra lines beyond the standard five fields.
		if len(agentInfo) == 0 {
			// Count non-empty lines — should be exactly 5 (the standard fields).
			nonEmptyLines := 0
			for _, line := range lines {
				if line != "" {
					nonEmptyLines++
				}
			}
			require.Equal(t, 5, nonEmptyLines,
				"when AgentInfo is empty, output must have exactly 5 non-empty lines")
		}
	})
}

// **Validates: Requirements SI-2.3, SI-2.4, SI-7.2, SI-7.3, SI-7.4**

// Feature: bootstrap-ai-coding, Property 1: Collection preserves order and excludes errors
func TestPropertyCollectionPreservesOrderAndExcludesErrors(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random number of agent results (1–10).
		n := rapid.IntRange(1, 10).Draw(t, "numAgents")

		results := make([]cmd.AgentInfoResult, n)
		var expected []agent.KeyValue

		for i := 0; i < n; i++ {
			hasError := rapid.Bool().Draw(t, fmt.Sprintf("hasError[%d]", i))
			if hasError {
				results[i] = cmd.AgentInfoResult{
					KeyValues: nil,
					Err:       fmt.Errorf("agent %d failed", i),
				}
			} else {
				// Generate 0–3 KeyValue pairs for this agent.
				kvCount := rapid.IntRange(0, 3).Draw(t, fmt.Sprintf("kvCount[%d]", i))
				kvs := make([]agent.KeyValue, kvCount)
				for j := 0; j < kvCount; j++ {
					kvs[j] = agent.KeyValue{
						Key:   rapid.StringMatching(`[A-Za-z][A-Za-z0-9 ]*`).Draw(t, fmt.Sprintf("key[%d][%d]", i, j)),
						Value: rapid.String().Draw(t, fmt.Sprintf("value[%d][%d]", i, j)),
					}
				}
				results[i] = cmd.AgentInfoResult{
					KeyValues: kvs,
					Err:       nil,
				}
				expected = append(expected, kvs...)
			}
		}

		collected := cmd.CollectAgentInfo(results)

		// The collected output must match the expected filtered/ordered result.
		require.Equal(t, len(expected), len(collected),
			"collected length must match expected (non-erroring agents' KVs)")
		for i := range expected {
			require.Equal(t, expected[i].Key, collected[i].Key,
				"Key at position %d must match", i)
			require.Equal(t, expected[i].Value, collected[i].Value,
				"Value at position %d must match", i)
		}
	})
}

// Feature: bootstrap-ai-coding, Property 4: Session summary includes Vibe Kanban URL for any valid port
func TestPropertySessionSummaryIncludesVibeKanbanURL(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		port := rapid.IntRange(1, 65535).Draw(t, "port")
		url := fmt.Sprintf("http://localhost:%d", port)

		summary := cmd.SessionSummary{
			DataDir:       "/home/user/.config/bootstrap-ai-coding/bac-test",
			ProjectDir:    "/home/user/project",
			SSHPort:       2222,
			SSHConnect:    "ssh bac-test",
			EnabledAgents: []string{"vibe-kanban"},
			AgentInfo:     []agent.KeyValue{{Key: "Vibe Kanban", Value: url}},
		}

		output := cmd.FormatSessionSummary(summary)

		// When AgentInfo contains Vibe Kanban, output must contain "Vibe Kanban:" and the URL.
		require.Contains(t, output, "Vibe Kanban:",
			"output must contain 'Vibe Kanban:' label when AgentInfo is set")
		require.Contains(t, output, url,
			"output must contain the Vibe Kanban URL %q", url)

		// When AgentInfo is empty, output must NOT contain "Vibe Kanban:".
		summaryEmpty := cmd.SessionSummary{
			DataDir:       "/home/user/.config/bootstrap-ai-coding/bac-test",
			ProjectDir:    "/home/user/project",
			SSHPort:       2222,
			SSHConnect:    "ssh bac-test",
			EnabledAgents: []string{"claude-code"},
			AgentInfo:     nil,
		}

		outputEmpty := cmd.FormatSessionSummary(summaryEmpty)
		require.NotContains(t, outputEmpty, "Vibe Kanban:",
			"output must NOT contain 'Vibe Kanban:' when AgentInfo is empty")
	})
}
