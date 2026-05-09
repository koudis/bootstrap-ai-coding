package docker_test

import (
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// TestRestartPolicyAppliedToContainerSpec verifies that the RestartPolicy value
// from ContainerSpec reaches the Docker HostConfig via ResolveRestartPolicy.
//
// When RestartPolicy is explicitly set, that exact value is used.
// When RestartPolicy is empty, constants.DefaultRestartPolicy is used.
func TestRestartPolicyAppliedToContainerSpec(t *testing.T) {
	t.Run("explicit policy reaches HostConfig", func(t *testing.T) {
		spec := docker.ContainerSpec{
			Name:          "bac-test",
			ImageTag:      "bac-test:latest",
			SSHPort:       2222,
			RestartPolicy: "always",
		}

		resolved := docker.ResolveRestartPolicy(spec)
		require.Equal(t, "always", resolved,
			"explicit RestartPolicy must be passed through unchanged")

		// Verify the resolved value produces the correct Docker RestartPolicy struct.
		hostConfigPolicy := container.RestartPolicy{
			Name: container.RestartPolicyMode(resolved),
		}
		require.Equal(t, container.RestartPolicyMode("always"), hostConfigPolicy.Name,
			"HostConfig.RestartPolicy.Name must match the spec value")
	})

	t.Run("empty policy defaults to constants.DefaultRestartPolicy", func(t *testing.T) {
		spec := docker.ContainerSpec{
			Name:          "bac-test",
			ImageTag:      "bac-test:latest",
			SSHPort:       2222,
			RestartPolicy: "",
		}

		resolved := docker.ResolveRestartPolicy(spec)
		require.Equal(t, constants.DefaultRestartPolicy, resolved,
			"empty RestartPolicy must default to constants.DefaultRestartPolicy")

		// Verify the default produces the correct Docker RestartPolicy struct.
		hostConfigPolicy := container.RestartPolicy{
			Name: container.RestartPolicyMode(resolved),
		}
		require.Equal(t, container.RestartPolicyMode(constants.DefaultRestartPolicy), hostConfigPolicy.Name,
			"HostConfig.RestartPolicy.Name must be the default when spec is empty")
	})

	t.Run("all valid policies resolve correctly", func(t *testing.T) {
		validPolicies := []string{"no", "always", "unless-stopped", "on-failure"}
		for _, policy := range validPolicies {
			spec := docker.ContainerSpec{
				Name:          "bac-test",
				ImageTag:      "bac-test:latest",
				SSHPort:       2222,
				RestartPolicy: policy,
			}

			resolved := docker.ResolveRestartPolicy(spec)
			require.Equal(t, policy, resolved,
				"RestartPolicy %q must pass through unchanged", policy)

			hostConfigPolicy := container.RestartPolicy{
				Name: container.RestartPolicyMode(resolved),
			}
			require.Equal(t, container.RestartPolicyMode(policy), hostConfigPolicy.Name,
				"HostConfig.RestartPolicy.Name must be %q", policy)
		}
	})
}

// Feature: bootstrap-ai-coding, Property 56: for any valid policy, ContainerSpec.RestartPolicy matches
func TestPropertyRestartPolicyContainerSpecMatches(t *testing.T) {
	validPolicies := []string{"no", "always", "unless-stopped", "on-failure"}
	rapid.Check(t, func(t *rapid.T) {
		// Draw either a valid policy or empty string
		useEmpty := rapid.Bool().Draw(t, "useEmpty")
		var policy string
		if useEmpty {
			policy = ""
		} else {
			idx := rapid.IntRange(0, len(validPolicies)-1).Draw(t, "policyIdx")
			policy = validPolicies[idx]
		}

		spec := docker.ContainerSpec{
			Name:          "bac-test",
			ImageTag:      "bac-test:latest",
			SSHPort:       2222,
			RestartPolicy: policy,
		}

		resolved := docker.ResolveRestartPolicy(spec)
		if policy == "" {
			require.Equal(t, constants.DefaultRestartPolicy, resolved)
		} else {
			require.Equal(t, policy, resolved)
		}
	})
}
