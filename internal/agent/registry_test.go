package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/agent"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// stubAgent is a minimal Agent implementation used only in tests.
type stubAgent struct{ id string }

func (s *stubAgent) ID() string                                              { return s.id }
func (s *stubAgent) Install(_ *docker.DockerfileBuilder)                     {}
func (s *stubAgent) CredentialStorePath() string                             { return "" }
func (s *stubAgent) ContainerMountPath(homeDir string) string                { return "" }
func (s *stubAgent) HasCredentials(_ string) (bool, error)                   { return false, nil }
func (s *stubAgent) HealthCheck(_ context.Context, _ *docker.Client, _ string) error { return nil }
func (s *stubAgent) SummaryInfo(_ context.Context, _ *docker.Client, _ string) ([]agent.KeyValue, error) {
	return nil, nil
}

// newStub is a convenience constructor.
func newStub(id string) agent.Agent { return &stubAgent{id: id} }

// TestLookup_UnknownID verifies that Lookup returns a non-nil error for an
// unregistered ID and that the error message contains the unknown ID.
// Validates: Req 7.2
func TestLookup_UnknownID(t *testing.T) {
	const unknownID = "definitely-not-registered-xyz-123"

	_, err := agent.Lookup(unknownID)

	require.Error(t, err, "Lookup of unknown ID must return an error")
	require.True(t,
		strings.Contains(err.Error(), unknownID),
		"error message %q should contain the unknown ID %q", err.Error(), unknownID,
	)
}

// TestLookup_UnknownID_ListsAvailableAgents verifies that the error message
// from Lookup includes the list of available agent IDs so the caller can
// present a helpful message to the user.
// Validates: Req 7.2
func TestLookup_UnknownID_ListsAvailableAgents(t *testing.T) {
	// Register a test agent so there is at least one known ID to list.
	const knownID = "test-agent-for-error-message-xyz"
	agent.Register(newStub(knownID))

	_, err := agent.Lookup("definitely-not-registered-abc-999")

	require.Error(t, err)
	require.True(t,
		strings.Contains(err.Error(), knownID),
		"error message %q should list the known agent ID %q", err.Error(), knownID,
	)
}

// TestRegister_PanicOnDuplicate verifies that registering the same ID twice
// causes a panic, preventing silent misconfiguration.
// Validates: Req 7.3
func TestRegister_PanicOnDuplicate(t *testing.T) {
	const dupID = "test-duplicate-agent-xyz"
	agent.Register(newStub(dupID))

	require.Panics(t, func() {
		agent.Register(newStub(dupID))
	}, "registering a duplicate agent ID must panic")
}

// TestKnownIDs_Sorted verifies that KnownIDs returns all registered IDs in
// alphabetical order regardless of registration order.
// Validates: Req 7.3
func TestKnownIDs_Sorted(t *testing.T) {
	// Register agents in reverse alphabetical order to confirm sorting.
	ids := []string{
		"test-zzz-agent-xyz",
		"test-mmm-agent-xyz",
		"test-aaa-agent-xyz",
	}
	for _, id := range ids {
		agent.Register(newStub(id))
	}

	known := agent.KnownIDs()

	// Verify all three test IDs are present and appear in sorted order.
	var found []string
	for _, id := range known {
		for _, testID := range ids {
			if id == testID {
				found = append(found, id)
				break
			}
		}
	}

	require.Len(t, found, len(ids), "all registered test IDs should appear in KnownIDs()")

	// The found slice must be in ascending order.
	for i := 1; i < len(found); i++ {
		require.True(t,
			found[i-1] < found[i],
			"KnownIDs() must be sorted: %q should come before %q", found[i-1], found[i],
		)
	}
}

// Feature: bootstrap-ai-coding, Property 26: Unknown agent IDs always produce errors
func TestPropertyUnknownAgentIDAlwaysErrors(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		id := rapid.String().Draw(t, "id")

		// Skip if this string happens to be a registered ID.
		for _, known := range agent.KnownIDs() {
			if id == known {
				return
			}
		}

		_, err := agent.Lookup(id)
		require.Error(t, err, "Lookup of unregistered ID %q must return a non-nil error", id)
	})
}

// TestAll_ReturnsRegisteredAgents verifies that All() returns every agent that
// has been registered, including ones registered in this test run.
func TestAll_ReturnsRegisteredAgents(t *testing.T) {
	const id = "test-all-agent-xyz"
	agent.Register(newStub(id))

	all := agent.All()
	require.NotEmpty(t, all, "All() must return at least one agent after registration")

	found := false
	for _, a := range all {
		if a.ID() == id {
			found = true
			break
		}
	}
	require.True(t, found, "All() must include the agent registered with ID %q", id)
}

// TestAll_CountMatchesKnownIDs verifies that len(All()) == len(KnownIDs()).
func TestAll_CountMatchesKnownIDs(t *testing.T) {
	require.Equal(t, len(agent.KnownIDs()), len(agent.All()),
		"All() and KnownIDs() must report the same number of agents")
}
