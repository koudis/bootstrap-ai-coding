package cmd_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/cmd"

	// Blank imports trigger init() registration for all agent modules.
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/augment"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/buildresources"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/claude"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/codex"
	_ "github.com/koudis/bootstrap-ai-coding/internal/agents/opencode"
)

// Feature: default-agents-expansion, Property 1: Valid agent subset round-trip
func TestPropertyValidAgentSubsetRoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		knownIDs := agent.KnownIDs()
		require.NotEmpty(t, knownIDs, "KnownIDs must not be empty (agents registered via blank imports)")

		// Generate a random non-empty subset of known agent IDs.
		// Shuffle all known IDs, then take a random-sized prefix.
		shuffled := rapid.Permutation(knownIDs).Draw(t, "shuffled")
		subsetSize := rapid.IntRange(1, len(shuffled)).Draw(t, "subsetSize")
		subset := shuffled[:subsetSize]

		// Join subset with commas, parse with ParseAgentsFlag.
		input := strings.Join(subset, ",")
		parsed := cmd.ParseAgentsFlag(input)

		// Assert parsed result matches the input subset in order.
		require.Equal(t, subset, parsed,
			"ParseAgentsFlag(%q) must return the same IDs in order", input)

		// Call agent.Lookup on each parsed ID and assert success.
		for _, id := range parsed {
			a, err := agent.Lookup(id)
			require.NoError(t, err, "agent.Lookup(%q) must succeed for a known agent ID", id)
			require.Equal(t, id, a.ID(),
				"agent.Lookup(%q).ID() must match the input ID", id)
		}
	})
}
