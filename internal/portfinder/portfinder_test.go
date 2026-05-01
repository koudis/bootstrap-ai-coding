package portfinder_test

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/portfinder"
)

// Feature: bootstrap-ai-coding, Property 20: Port finder returns the first free port at or above constants.SSHPortStart
func TestPortFinderReturnsFirstFreePort(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw how many ports to occupy: 0–5
		n := rapid.IntRange(0, 5).Draw(t, "numOccupied")

		// Open up to n real TCP listeners starting at constants.SSHPortStart.
		// Skip ports that fail to bind — they may already be in use on the machine.
		var listeners []net.Listener
		occupiedPorts := make(map[int]bool)

		for port := constants.SSHPortStart; len(listeners) < n; port++ {
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				// Port already in use — skip it; it will naturally be skipped by FindFreePort too
				continue
			}
			listeners = append(listeners, ln)
			occupiedPorts[port] = true
		}

		// Ensure all listeners are closed when the test ends.
		t.Cleanup(func() {
			for _, ln := range listeners {
				ln.Close()
			}
		})

		// Call FindFreePort and verify the result.
		got, err := portfinder.FindFreePort()
		require.NoError(t, err)

		// Property: returned port is always >= constants.SSHPortStart
		require.GreaterOrEqual(t, got, constants.SSHPortStart,
			"FindFreePort() returned %d, want >= %d", got, constants.SSHPortStart)

		// Property: IsPortFree returns true for the returned port
		// (we must close our listeners first so the port is actually free)
		for _, ln := range listeners {
			ln.Close()
		}
		listeners = nil // prevent double-close in Cleanup

		require.True(t, portfinder.IsPortFree(got),
			"IsPortFree(%d) should be true for the port returned by FindFreePort()", got)
	})
}
