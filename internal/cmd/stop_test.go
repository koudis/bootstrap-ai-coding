package cmd_test

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/cmd"
)

// mockStopDockerAPI is a test double that records which Docker methods were called.
// It implements cmd.StopDockerAPI.
type mockStopDockerAPI struct {
	containerListResult    []container.Summary
	containerInspectResult container.InspectResponse
	containerInspectErr    error

	imageRemoveCalled bool
}

func (m *mockStopDockerAPI) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	return m.containerListResult, nil
}

func (m *mockStopDockerAPI) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	return m.containerInspectResult, m.containerInspectErr
}

func (m *mockStopDockerAPI) ContainerStop(_ context.Context, _ string, _ container.StopOptions) error {
	return nil
}

func (m *mockStopDockerAPI) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	return nil
}

func (m *mockStopDockerAPI) ImageRemove(_ context.Context, _ string, _ image.RemoveOptions) ([]image.DeleteResponse, error) {
	m.imageRemoveCalled = true
	return nil, nil
}

// TestStopAndRemoveDoesNotRemoveImages verifies that the stop-and-remove flow
// does NOT call ImageRemove. This confirms TL-6: --stop-and-remove preserves
// both Base_Image and Instance_Image.
// Validates: TL-6
func TestStopAndRemoveDoesNotRemoveImages(t *testing.T) {
	mock := &mockStopDockerAPI{
		// Simulate a container that exists for the project.
		containerListResult: []container.Summary{
			{
				Names: []string{"/bac-myproject"},
			},
		},
		containerInspectResult: container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{
				State: &container.State{Running: true},
			},
		},
	}

	err := cmd.RunStopWith(mock, "/home/user/myproject")
	require.NoError(t, err)
	require.False(t, mock.imageRemoveCalled,
		"stop-and-remove must NOT call ImageRemove — images should be preserved (TL-6)")
}

// TestStopAndRemoveNoContainerDoesNotRemoveImages verifies that when no container
// exists, the stop-and-remove flow still does NOT call ImageRemove.
// Validates: TL-6
func TestStopAndRemoveNoContainerDoesNotRemoveImages(t *testing.T) {
	mock := &mockStopDockerAPI{
		containerListResult: []container.Summary{},
		containerInspectErr: errors.New("No such container: bac-myproject"),
	}

	err := cmd.RunStopWith(mock, "/home/user/myproject")
	require.NoError(t, err)
	require.False(t, mock.imageRemoveCalled,
		"stop-and-remove with no container must NOT call ImageRemove (TL-6)")
}
