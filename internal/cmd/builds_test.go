package cmd_test

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/image"
	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/cmd"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
)

// mockDetermineBuildsAPI is a test double implementing cmd.DetermineBuildsAPI.
// It returns different results based on the image tag requested.
type mockDetermineBuildsAPI struct {
	// images maps image tags to their inspect responses.
	// If a tag is absent from the map, ImageInspectWithRaw returns an error.
	images map[string]image.InspectResponse
}

func (m *mockDetermineBuildsAPI) ImageInspectWithRaw(_ context.Context, imageID string) (image.InspectResponse, []byte, error) {
	resp, ok := m.images[imageID]
	if !ok {
		return image.InspectResponse{}, nil, errors.New("No such image: " + imageID)
	}
	return resp, nil, nil
}

// helper to build an InspectResponse with the given labels on Config.
func inspectWithLabels(labels map[string]string) image.InspectResponse {
	return image.InspectResponse{
		Config: &dockerspec.DockerOCIImageConfig{
			ImageConfig: ocispec.ImageConfig{
				Labels: labels,
			},
		},
	}
}

// TestDetermineBuildsRebuildTrue verifies that when rebuild=true, both layers
// always need building regardless of image state.
// Validates: TL-3
func TestDetermineBuildsRebuildTrue(t *testing.T) {
	mock := &mockDetermineBuildsAPI{
		images: map[string]image.InspectResponse{},
	}

	needBase, needInstance, err := cmd.DetermineBuildsWithAPI(
		context.Background(), mock,
		[]string{"claude-code"}, "bac-myproject", true,
	)

	require.NoError(t, err)
	require.True(t, needBase, "rebuild=true must require base build")
	require.True(t, needInstance, "rebuild=true must require instance build")
}

// TestDetermineBuildsBaseAbsent verifies that when the base image does not exist,
// both layers need building.
// Validates: TL-3
func TestDetermineBuildsBaseAbsent(t *testing.T) {
	mock := &mockDetermineBuildsAPI{
		images: map[string]image.InspectResponse{},
	}

	needBase, needInstance, err := cmd.DetermineBuildsWithAPI(
		context.Background(), mock,
		[]string{"claude-code"}, "bac-myproject", false,
	)

	require.NoError(t, err)
	require.True(t, needBase, "absent base image must require base build")
	require.True(t, needInstance, "absent base image must require instance build")
}

// TestDetermineBuildsBasePresentNoLabel verifies that when the base image exists
// but has no bac.manifest label, both layers need building.
// Validates: TL-3
func TestDetermineBuildsBasePresentNoLabel(t *testing.T) {
	mock := &mockDetermineBuildsAPI{
		images: map[string]image.InspectResponse{
			constants.BaseImageTag: inspectWithLabels(map[string]string{
				"bac.managed": "true",
				// no bac.manifest label
			}),
		},
	}

	needBase, needInstance, err := cmd.DetermineBuildsWithAPI(
		context.Background(), mock,
		[]string{"claude-code"}, "bac-myproject", false,
	)

	require.NoError(t, err)
	require.True(t, needBase, "base without manifest label must require base build")
	require.True(t, needInstance, "base without manifest label must require instance build")
}

// TestDetermineBuildsBasePresentInvalidJSON verifies that when the base image
// has a bac.manifest label with invalid JSON, both layers need building.
// Validates: TL-3
func TestDetermineBuildsBasePresentInvalidJSON(t *testing.T) {
	mock := &mockDetermineBuildsAPI{
		images: map[string]image.InspectResponse{
			constants.BaseImageTag: inspectWithLabels(map[string]string{
				"bac.managed":  "true",
				"bac.manifest": "not-valid-json{{{",
			}),
		},
	}

	needBase, needInstance, err := cmd.DetermineBuildsWithAPI(
		context.Background(), mock,
		[]string{"claude-code"}, "bac-myproject", false,
	)

	require.NoError(t, err)
	require.True(t, needBase, "invalid manifest JSON must require base build")
	require.True(t, needInstance, "invalid manifest JSON must require instance build")
}

// TestDetermineBuildsManifestMismatch verifies that when the base image manifest
// does not match the current enabledIDs, ErrManifestMismatch is returned.
// Validates: TL-4
func TestDetermineBuildsManifestMismatch(t *testing.T) {
	mock := &mockDetermineBuildsAPI{
		images: map[string]image.InspectResponse{
			constants.BaseImageTag: inspectWithLabels(map[string]string{
				"bac.managed":  "true",
				"bac.manifest": `["claude-code"]`,
			}),
		},
	}

	_, _, err := cmd.DetermineBuildsWithAPI(
		context.Background(), mock,
		[]string{"claude-code", "augment-code"}, "bac-myproject", false,
	)

	require.ErrorIs(t, err, cmd.ErrManifestMismatch,
		"mismatched manifest must return ErrManifestMismatch")
}

// TestDetermineBuildsManifestMatchInstanceAbsent verifies that when the base
// image manifest matches but the instance image is absent, only the instance
// layer needs building.
// Validates: TL-8
func TestDetermineBuildsManifestMatchInstanceAbsent(t *testing.T) {
	mock := &mockDetermineBuildsAPI{
		images: map[string]image.InspectResponse{
			constants.BaseImageTag: inspectWithLabels(map[string]string{
				"bac.managed":  "true",
				"bac.manifest": `["claude-code","augment-code"]`,
			}),
			// instance image "bac-myproject:latest" is NOT in the map
		},
	}

	needBase, needInstance, err := cmd.DetermineBuildsWithAPI(
		context.Background(), mock,
		[]string{"claude-code", "augment-code"}, "bac-myproject", false,
	)

	require.NoError(t, err)
	require.False(t, needBase, "matching manifest must not require base build")
	require.True(t, needInstance, "absent instance image must require instance build")
}

// TestDetermineBuildsManifestMatchInstancePresent verifies that when both the
// base image manifest matches and the instance image exists, no builds are needed.
// Validates: TL-8
func TestDetermineBuildsManifestMatchInstancePresent(t *testing.T) {
	mock := &mockDetermineBuildsAPI{
		images: map[string]image.InspectResponse{
			constants.BaseImageTag: inspectWithLabels(map[string]string{
				"bac.managed":  "true",
				"bac.manifest": `["claude-code","augment-code"]`,
			}),
			"bac-myproject:latest": inspectWithLabels(map[string]string{
				"bac.managed":   "true",
				"bac.container": "bac-myproject",
			}),
		},
	}

	needBase, needInstance, err := cmd.DetermineBuildsWithAPI(
		context.Background(), mock,
		[]string{"claude-code", "augment-code"}, "bac-myproject", false,
	)

	require.NoError(t, err)
	require.False(t, needBase, "both cached must not require base build")
	require.False(t, needInstance, "both cached must not require instance build")
}
