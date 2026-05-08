package cmd

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/docker/docker/api/types/image"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	dockerpkg "github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// DetermineBuildsAPI is the subset of Docker operations used by determineBuilds.
// It exists to enable unit testing without a live Docker daemon.
type DetermineBuildsAPI interface {
	ImageInspectWithRaw(ctx context.Context, imageID string) (image.InspectResponse, []byte, error)
}

// ErrManifestMismatch is returned by determineBuilds when the base image exists
// but its bac.manifest label does not match the current enabledIDs. The caller
// should print a message instructing the user to run with --rebuild and exit 0.
var ErrManifestMismatch = errors.New("agent configuration changed")

// DetermineBuildsWithAPI is the testable core of determineBuilds. It accepts
// the DetermineBuildsAPI interface so tests can supply a mock without a live
// Docker daemon. The production code path (determineBuilds) delegates to this.
func DetermineBuildsWithAPI(ctx context.Context, api DetermineBuildsAPI, enabledIDs []string, containerName string, rebuild bool) (needBase, needInstance bool, err error) {
	if rebuild {
		return true, true, nil
	}

	// Check base image
	baseInfo, _, inspectErr := api.ImageInspectWithRaw(ctx, constants.BaseImageTag)
	if inspectErr != nil {
		// Base image doesn't exist — must build both
		return true, true, nil
	}

	manifestJSON, ok := baseInfo.Config.Labels["bac.manifest"]
	if !ok {
		// No manifest label — rebuild base
		return true, true, nil
	}

	var manifestIDs []string
	if err := json.Unmarshal([]byte(manifestJSON), &manifestIDs); err != nil {
		// Invalid JSON in manifest label — rebuild base
		return true, true, nil
	}

	if !StringSlicesEqual(manifestIDs, enabledIDs) {
		// Manifest mismatch — caller prints message and exits
		return false, false, ErrManifestMismatch
	}

	// Base is good. Check instance image.
	instanceTag := containerName + ":latest"
	_, _, inspectErr = api.ImageInspectWithRaw(ctx, instanceTag)
	if inspectErr != nil {
		// Instance image missing — build it only
		return false, true, nil
	}

	// Both cached
	return false, false, nil
}

// determineBuilds decides which layers (base and/or instance) need to be built.
//
// Logic:
//  1. If rebuild is true, both layers always need building.
//  2. Inspect bac-base:latest — if absent, build both.
//  3. Check the bac.manifest label on the base image — if absent or invalid JSON, build both.
//  4. If the manifest content doesn't match enabledIDs, return ErrManifestMismatch.
//  5. Inspect <containerName>:latest — if absent, build instance only.
//  6. Otherwise, both are cached — skip both builds.
func determineBuilds(ctx context.Context, c *dockerpkg.Client, enabledIDs []string, containerName string, rebuild bool) (needBase, needInstance bool, err error) {
	return DetermineBuildsWithAPI(ctx, c, enabledIDs, containerName, rebuild)
}
