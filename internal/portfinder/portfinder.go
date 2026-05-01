// Package portfinder provides SSH port auto-selection starting at
// constants.SSHPortStart, incrementing until a free TCP port is found.
package portfinder

import (
	"fmt"
	"net"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
)

// FindFreePort iterates from constants.SSHPortStart upward and returns the
// first TCP port on 127.0.0.1 that is not already in use.
func FindFreePort() (int, error) {
	for port := constants.SSHPortStart; port < 65535; port++ {
		if IsPortFree(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port found starting at %d", constants.SSHPortStart)
}

// IsPortFree reports whether the given port is available for binding on
// 127.0.0.1. It attempts to open a TCP listener; if successful it closes the
// listener immediately and returns true.
func IsPortFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}
