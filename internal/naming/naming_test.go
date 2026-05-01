package naming_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/naming"
)

// allowedChars matches only characters allowed in a sanitized name component: [a-z0-9.-]
var allowedChars = regexp.MustCompile(`^[a-z0-9.\-]*$`)

// Feature: bootstrap-ai-coding, Property 12: Container naming produces correct, collision-resistant names
// Sub-property: determinism — same path + same existingNames always returns same name
func TestContainerNameDeterminism(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Use paths that don't touch the filesystem (no data dirs exist for them).
		// Two-segment paths like /tmp/<dirname> are safe.
		dirname := rapid.StringMatching(`[a-z][a-z0-9-]*`).Draw(t, "dirname")
		path := "/tmp/" + dirname

		name1, err1 := naming.ContainerName(path, nil)
		name2, err2 := naming.ContainerName(path, nil)

		require.NoError(t, err1)
		require.NoError(t, err2)
		require.Equal(t, name1, name2, "same path must always produce the same name")
	})
}

// Feature: bootstrap-ai-coding, Property 12: Container naming produces correct, collision-resistant names
// Sub-property: prefix — every returned name starts with constants.ContainerNamePrefix
func TestContainerNameHasBacPrefix(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		dirname := rapid.StringMatching(`[a-z][a-z0-9-]*`).Draw(t, "dirname")
		path := "/tmp/" + dirname

		name, err := naming.ContainerName(path, nil)

		require.NoError(t, err)
		require.True(t,
			strings.HasPrefix(name, constants.ContainerNamePrefix),
			"name %q does not start with prefix %q", name, constants.ContainerNamePrefix,
		)
	})
}

// Feature: bootstrap-ai-coding, Property 12: Container naming produces correct, collision-resistant names
// Sub-property: conflict advancement — with level-1 name occupied, returns a different name
func TestContainerNameConflictAdvancesToNextLevel(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Use a two-segment path so level-2 (parentdir_dirname) is well-defined.
		dirname := rapid.StringMatching(`[a-z][a-z0-9-]*`).Draw(t, "dirname")
		path := "/tmp/" + dirname

		// Obtain the level-1 candidate.
		level1, err := naming.ContainerName(path, nil)
		require.NoError(t, err)

		// With level-1 occupied, the function must return something different.
		level2, err := naming.ContainerName(path, []string{level1})
		require.NoError(t, err)

		require.NotEqual(t, level1, level2,
			"should advance past occupied level-1 name %q", level1)
		require.True(t,
			strings.HasPrefix(level2, constants.ContainerNamePrefix),
			"advanced name %q does not start with prefix %q", level2, constants.ContainerNamePrefix,
		)
	})
}

// Feature: bootstrap-ai-coding, Property 12: Container naming produces correct, collision-resistant names
// Sub-property: sanitization — SanitizeNameComponent only produces chars in [a-z0-9.-] (no _, no uppercase, no special chars)
func TestSanitizeNameComponentOutputCharset(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw an arbitrary string (any Unicode).
		input := rapid.String().Draw(t, "input")

		output := naming.SanitizeNameComponent(input)

		// Empty output is allowed (e.g. input was all dashes that got trimmed).
		if output == "" {
			return
		}

		require.True(t,
			allowedChars.MatchString(output),
			"SanitizeNameComponent(%q) = %q contains disallowed characters (only [a-z0-9.-] permitted)",
			input, output,
		)

		// Explicitly assert no underscore (reserved separator).
		require.NotContains(t, output, "_",
			"SanitizeNameComponent(%q) = %q must not contain '_'", input, output)

		// No uppercase letters.
		require.Equal(t, strings.ToLower(output), output,
			"SanitizeNameComponent(%q) = %q must be lowercase", input, output)
	})
}
