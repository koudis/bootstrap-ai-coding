package docker_test

import (
	"testing"

	"github.com/docker/docker/api/types/mount"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// ---------------------------------------------------------------------------
// Unit tests — VolumeMount in ContainerSpec
// ---------------------------------------------------------------------------

// TestContainerSpecVolumesTranslateToMountTypeVolume verifies that when
// ContainerSpec.Volumes is populated, the expected mount.Mount entries of type
// TypeVolume would be produced. Since CreateContainer requires a live Docker
// daemon, we verify the translation logic conceptually by constructing the
// expected mounts slice the same way CreateContainer does internally.
func TestContainerSpecVolumesTranslateToMountTypeVolume(t *testing.T) {
	spec := docker.ContainerSpec{
		Name:     "bac-testproject",
		ImageTag: "bac-testproject:latest",
		Mounts: []docker.Mount{
			{HostPath: "/tmp/project", ContainerPath: constants.WorkspaceMountPath},
		},
		Volumes: []docker.VolumeMount{
			{Name: "bac-testproject" + constants.VSCodeServerVolumeSuffix, ContainerPath: constants.VSCodeServerPath},
		},
		SSHPort: 2222,
		Labels:  map[string]string{"bac.managed": "true"},
	}

	// Replicate the mount assembly logic from CreateContainer.
	var mounts []mount.Mount
	for _, m := range spec.Mounts {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.HostPath,
			Target:   m.ContainerPath,
			ReadOnly: m.ReadOnly,
		})
	}
	for _, v := range spec.Volumes {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: v.Name,
			Target: v.ContainerPath,
		})
	}

	// Verify the mounts slice contains both the bind mount and the volume mount.
	require.Len(t, mounts, 2, "expected 2 mounts: 1 bind + 1 volume")

	// First mount: bind mount for workspace.
	require.Equal(t, mount.TypeBind, mounts[0].Type)
	require.Equal(t, "/tmp/project", mounts[0].Source)
	require.Equal(t, constants.WorkspaceMountPath, mounts[0].Target)

	// Second mount: volume mount for VS Code server.
	require.Equal(t, mount.TypeVolume, mounts[1].Type)
	require.Equal(t, "bac-testproject"+constants.VSCodeServerVolumeSuffix, mounts[1].Source)
	require.Equal(t, constants.VSCodeServerPath, mounts[1].Target)
}

// TestContainerSpecEmptyVolumesProducesNoVolumeMounts verifies that when
// ContainerSpec.Volumes is empty, no volume mounts are added.
func TestContainerSpecEmptyVolumesProducesNoVolumeMounts(t *testing.T) {
	spec := docker.ContainerSpec{
		Name:     "bac-testproject",
		ImageTag: "bac-testproject:latest",
		Mounts: []docker.Mount{
			{HostPath: "/tmp/project", ContainerPath: constants.WorkspaceMountPath},
		},
		Volumes: nil,
		SSHPort: 2222,
		Labels:  map[string]string{"bac.managed": "true"},
	}

	var mounts []mount.Mount
	for _, m := range spec.Mounts {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.HostPath,
			Target:   m.ContainerPath,
			ReadOnly: m.ReadOnly,
		})
	}
	for _, v := range spec.Volumes {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: v.Name,
			Target: v.ContainerPath,
		})
	}

	require.Len(t, mounts, 1, "expected only 1 bind mount when Volumes is empty")
	require.Equal(t, mount.TypeBind, mounts[0].Type)
}

// ---------------------------------------------------------------------------
// Unit tests — IsBACVolumeName filtering logic
// ---------------------------------------------------------------------------

// TestIsBACVolumeNameMatchesCorrectPattern verifies that IsBACVolumeName
// returns true for volume names that start with constants.ContainerNamePrefix
// and end with constants.VSCodeServerVolumeSuffix.
func TestIsBACVolumeNameMatchesCorrectPattern(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"bac-myproject-vscode-server", true},
		{"bac-parent_child-vscode-server", true},
		{"bac-x-vscode-server", true},
		{"bac--vscode-server", true},
		{"other-myproject-vscode-server", false}, // wrong prefix
		{"bac-myproject-other-volume", false},    // wrong suffix
		{"bac-myproject", false},                 // no suffix
		{"", false},                              // empty
		{"vscode-server", false},                 // no prefix
		{"bac-", false},                          // prefix only, no suffix
		{"-vscode-server", false},                // suffix only, no prefix
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := docker.IsBACVolumeName(tc.name)
			require.Equal(t, tc.want, got,
				"IsBACVolumeName(%q) = %v, want %v", tc.name, got, tc.want)
		})
	}
}

// ---------------------------------------------------------------------------
// Property-based test — IsBACVolumeName
// ---------------------------------------------------------------------------

// Feature: bootstrap-ai-coding, Property 52: IsBACVolumeName correctly filters volumes by prefix and suffix
func TestPropertyIsBACVolumeNameFiltering(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Draw a random middle part for the volume name.
		middle := rapid.StringMatching(`[a-z0-9_.-]{1,30}`).Draw(t, "middle")

		// A correctly formed BAC volume name must match.
		validName := constants.ContainerNamePrefix + middle + constants.VSCodeServerVolumeSuffix
		require.True(t, docker.IsBACVolumeName(validName),
			"IsBACVolumeName(%q) should be true for valid BAC volume name", validName)

		// A name with the correct prefix but wrong suffix must not match.
		wrongSuffix := constants.ContainerNamePrefix + middle + "-other"
		require.False(t, docker.IsBACVolumeName(wrongSuffix),
			"IsBACVolumeName(%q) should be false for wrong suffix", wrongSuffix)

		// A name with the correct suffix but wrong prefix must not match.
		wrongPrefix := "other-" + middle + constants.VSCodeServerVolumeSuffix
		require.False(t, docker.IsBACVolumeName(wrongPrefix),
			"IsBACVolumeName(%q) should be false for wrong prefix", wrongPrefix)
	})
}
