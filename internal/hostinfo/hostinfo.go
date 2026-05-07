// Package hostinfo resolves the host user's identity at CLI startup.
// The Info struct replaces the former compile-time constants ContainerUser
// and ContainerUserHome, providing runtime-resolved values that match the
// host user's OS account.
package hostinfo

import (
	"fmt"
	"os/user"
	"strconv"
)

// Info holds the runtime-resolved host user identity.
// These values determine the Container_User username and home directory.
type Info struct {
	Username string // host username (e.g. "alice")
	HomeDir  string // host home directory (e.g. "/home/alice")
	UID      int    // host effective UID
	GID      int    // host effective GID
}

// Current returns the host user's identity. Called once at CLI startup.
// Returns an error if the OS user cannot be determined.
func Current() (*Info, error) {
	u, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("resolving host user: %w", err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return nil, fmt.Errorf("parsing host UID %q: %w", u.Uid, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return nil, fmt.Errorf("parsing host GID %q: %w", u.Gid, err)
	}
	return &Info{
		Username: u.Username,
		HomeDir:  u.HomeDir,
		UID:      uid,
		GID:      gid,
	}, nil
}
