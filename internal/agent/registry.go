package agent

import (
	"fmt"
	"sort"
)

var registry = map[string]Agent{}

// Register adds an agent to the global registry.
// Panics on duplicate ID.
func Register(a Agent) {
	if _, exists := registry[a.ID()]; exists {
		panic(fmt.Sprintf("agent: duplicate registration for ID %q", a.ID()))
	}
	registry[a.ID()] = a
}

// Lookup returns the agent with the given ID, or a descriptive error if not found.
func Lookup(id string) (Agent, error) {
	a, ok := registry[id]
	if !ok {
		return nil, fmt.Errorf("agent %q is not registered; available agents: %v", id, KnownIDs())
	}
	return a, nil
}

// All returns all registered agents in an unspecified order.
func All() []Agent {
	agents := make([]Agent, 0, len(registry))
	for _, a := range registry {
		agents = append(agents, a)
	}
	return agents
}

// KnownIDs returns the IDs of all registered agents, sorted alphabetically.
func KnownIDs() []string {
	ids := make([]string, 0, len(registry))
	for id := range registry {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
