package hostinfo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCurrentReturnsNonNilInfo(t *testing.T) {
	info, err := Current()
	require.NoError(t, err)
	require.NotNil(t, info)
}

func TestCurrentUsernameNonEmpty(t *testing.T) {
	info, err := Current()
	require.NoError(t, err)
	require.NotEmpty(t, info.Username, "Username must not be empty")
}

func TestCurrentHomeDirNonEmpty(t *testing.T) {
	info, err := Current()
	require.NoError(t, err)
	require.NotEmpty(t, info.HomeDir, "HomeDir must not be empty")
}

func TestCurrentUIDNonNegative(t *testing.T) {
	info, err := Current()
	require.NoError(t, err)
	require.GreaterOrEqual(t, info.UID, 0, "UID must be non-negative")
}

func TestCurrentGIDNonNegative(t *testing.T) {
	info, err := Current()
	require.NoError(t, err)
	require.GreaterOrEqual(t, info.GID, 0, "GID must be non-negative")
}
