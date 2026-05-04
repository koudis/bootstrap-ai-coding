//go:build integration

// Package testutil provides shared helpers for integration tests.
package testutil

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/api/types/image"
	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// RequireIntegrationConsent checks the BAC_INTEGRATION_CONSENT environment
// variable. If it is not set to "yes", a warning is printed to stderr and the
// process exits with code 1.
//
// Call this from TestMain in every integration test package, after verifying
// that Docker is available but before running any tests.
//
// To grant consent (e.g. in CI), export the variable before running tests:
//
//	BAC_INTEGRATION_CONSENT=yes go test -tags integration -timeout 30m ./...
func RequireIntegrationConsent() {
	if os.Getenv("BAC_INTEGRATION_CONSENT") == "yes" {
		return
	}

	fmt.Fprint(os.Stderr,
		"\n"+
			"WARNING: Integration tests interact with the local Docker daemon.\n"+
			"They may pull, build, delete, and update Docker images and containers.\n"+
			"\n"+
			"To run these tests, set the environment variable:\n"+
			"  BAC_INTEGRATION_CONSENT=yes go test -tags integration ./...\n"+
			"\n")
	fmt.Fprintln(os.Stderr, "Aborted — no consent given.")
	os.Exit(1)
}

// EnsureBaseImageAbsent removes constants.BaseContainerImage from the local
// Docker store if it is present, so integration test suites always start from
// a clean slate. If the image is not present, it returns nil (nothing to do).
// An error is returned only if the removal itself fails.
//
// Call this from TestMain in every integration test package, after
// RequireIntegrationConsent() but before m.Run().
func EnsureBaseImageAbsent() error {
	client, err := docker.NewClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	_, _, err = client.ImageInspectWithRaw(ctx, constants.BaseContainerImage)
	if err != nil {
		// Image not present — nothing to do.
		return nil
	}

	fmt.Fprintf(os.Stderr, "Removing cached base image %s to ensure clean test environment\n", constants.BaseContainerImage)

	_, err = client.ImageRemove(ctx, constants.BaseContainerImage, image.RemoveOptions{Force: true})
	return err
}
