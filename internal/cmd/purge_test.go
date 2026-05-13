package cmd_test

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/cmd"
)

// mockPurgeDockerAPI is a test double that records which images were removed.
// It implements cmd.PurgeDockerAPI.
type mockPurgeDockerAPI struct {
	containers []container.Summary
	images     []image.Summary

	removedImageIDs []string
	pruned          bool
}

func (m *mockPurgeDockerAPI) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return m.containers, nil
}

func (m *mockPurgeDockerAPI) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return nil
}

func (m *mockPurgeDockerAPI) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	return nil
}

func (m *mockPurgeDockerAPI) ImageList(_ context.Context, _ image.ListOptions) ([]image.Summary, error) {
	return m.images, nil
}

func (m *mockPurgeDockerAPI) ImageRemove(_ context.Context, imageID string, _ image.RemoveOptions) ([]image.DeleteResponse, error) {
	m.removedImageIDs = append(m.removedImageIDs, imageID)
	return nil, nil
}

func (m *mockPurgeDockerAPI) ImagesPrune(_ context.Context, _ filters.Args) (image.PruneReport, error) {
	m.pruned = true
	return image.PruneReport{}, nil
}

// TestPurgeRemovesBothBaseAndInstanceImages verifies that the purge flow removes
// both the base image (bac-base:latest) and instance images (e.g. bac-myproject:latest)
// when both are present with bac.managed=true labels.
// Validates: TL-7
func TestPurgeRemovesBothBaseAndInstanceImages(t *testing.T) {
	mock := &mockPurgeDockerAPI{
		containers: []container.Summary{},
		images: []image.Summary{
			{
				ID:       "sha256:base111",
				RepoTags: []string{"bac-base:latest"},
				Labels:   map[string]string{"bac.managed": "true"},
			},
			{
				ID:       "sha256:instance222",
				RepoTags: []string{"bac-myproject:latest"},
				Labels:   map[string]string{"bac.managed": "true", "bac.container": "bac-myproject"},
			},
		},
	}

	err := cmd.RunPurgeWith(mock)
	require.NoError(t, err)

	require.Len(t, mock.removedImageIDs, 2,
		"purge must remove exactly 2 images (base + instance)")
	require.Contains(t, mock.removedImageIDs, "sha256:base111",
		"purge must remove the base image (bac-base:latest)")
	require.Contains(t, mock.removedImageIDs, "sha256:instance222",
		"purge must remove the instance image (bac-myproject:latest)")
}

// TestPurgeRemovesMultipleInstanceImages verifies that purge removes the base
// image and all instance images when multiple projects exist.
// Validates: TL-7
func TestPurgeRemovesMultipleInstanceImages(t *testing.T) {
	mock := &mockPurgeDockerAPI{
		containers: []container.Summary{},
		images: []image.Summary{
			{
				ID:       "sha256:base111",
				RepoTags: []string{"bac-base:latest"},
				Labels:   map[string]string{"bac.managed": "true"},
			},
			{
				ID:       "sha256:instance-a",
				RepoTags: []string{"bac-projecta:latest"},
				Labels:   map[string]string{"bac.managed": "true", "bac.container": "bac-projecta"},
			},
			{
				ID:       "sha256:instance-b",
				RepoTags: []string{"bac-projectb:latest"},
				Labels:   map[string]string{"bac.managed": "true", "bac.container": "bac-projectb"},
			},
		},
	}

	err := cmd.RunPurgeWith(mock)
	require.NoError(t, err)

	require.Len(t, mock.removedImageIDs, 3,
		"purge must remove all 3 images (1 base + 2 instances)")
	require.Contains(t, mock.removedImageIDs, "sha256:base111")
	require.Contains(t, mock.removedImageIDs, "sha256:instance-a")
	require.Contains(t, mock.removedImageIDs, "sha256:instance-b")
}

// TestPurgeRemovesInstanceImagesBeforeBaseImage verifies that purge removes
// instance images (children) before the base image (parent) to avoid Docker's
// "image has dependent child images" error.
// Validates: TL-7.4
func TestPurgeRemovesInstanceImagesBeforeBaseImage(t *testing.T) {
	mock := &mockPurgeDockerAPI{
		containers: []container.Summary{},
		// Deliberately list base image FIRST to verify reordering.
		images: []image.Summary{
			{
				ID:       "sha256:base111",
				RepoTags: []string{"bac-base:latest"},
				Labels:   map[string]string{"bac.managed": "true"},
			},
			{
				ID:       "sha256:instance-a",
				RepoTags: []string{"bac-projecta:latest"},
				Labels:   map[string]string{"bac.managed": "true", "bac.container": "bac-projecta"},
			},
			{
				ID:       "sha256:instance-b",
				RepoTags: []string{"bac-projectb:latest"},
				Labels:   map[string]string{"bac.managed": "true", "bac.container": "bac-projectb"},
			},
		},
	}

	err := cmd.RunPurgeWith(mock)
	require.NoError(t, err)

	require.Len(t, mock.removedImageIDs, 3)

	// Find the index of the base image in the removal order.
	baseIdx := -1
	for i, id := range mock.removedImageIDs {
		if id == "sha256:base111" {
			baseIdx = i
			break
		}
	}
	require.NotEqual(t, -1, baseIdx, "base image must be removed")

	// All instance images must appear before the base image.
	for i, id := range mock.removedImageIDs {
		if id == "sha256:instance-a" || id == "sha256:instance-b" {
			require.Less(t, i, baseIdx,
				"instance image %s (index %d) must be removed before base image (index %d)", id, i, baseIdx)
		}
	}
}

// TestPurgeAlsoStopsAndRemovesContainers verifies that purge stops and removes
// containers before removing images.
// Validates: TL-7
func TestPurgeAlsoStopsAndRemovesContainers(t *testing.T) {
	mock := &mockPurgeDockerAPI{
		containers: []container.Summary{
			{
				ID:    "container-1",
				Names: []string{"/bac-myproject"},
			},
		},
		images: []image.Summary{
			{
				ID:       "sha256:base111",
				RepoTags: []string{"bac-base:latest"},
				Labels:   map[string]string{"bac.managed": "true"},
			},
		},
	}

	err := cmd.RunPurgeWith(mock)
	require.NoError(t, err)

	// Image should still be removed even when containers exist.
	require.Len(t, mock.removedImageIDs, 1)
	require.Contains(t, mock.removedImageIDs, "sha256:base111")
}
