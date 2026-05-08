package cmd

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

// PurgeDockerAPI is the subset of Docker operations used by the purge flow.
// It exists to enable unit testing without a live Docker daemon.
type PurgeDockerAPI interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error)
	ImageRemove(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error)
}

// RunPurgeWith implements the container and image removal portion of the purge
// flow against the PurgeDockerAPI interface. This is the testable core that
// verifies both base and instance images are removed.
//
// It does NOT handle data directory purging, known_hosts cleanup, SSH config
// cleanup, or user confirmation — those are handled by the full runPurge function.
func RunPurgeWith(api PurgeDockerAPI) error {
	ctx := context.Background()

	// List bac-managed containers.
	containers, err := api.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: purgeFilter(),
	})
	if err != nil {
		return fmt.Errorf("listing bac containers: %w", err)
	}

	// List bac-managed images.
	images, err := api.ImageList(ctx, image.ListOptions{
		Filters: purgeFilter(),
	})
	if err != nil {
		return fmt.Errorf("listing bac images: %w", err)
	}

	// Stop and remove all containers.
	for _, ctr := range containers {
		_ = api.ContainerStop(ctx, ctr.ID, container.StopOptions{})
		if err := api.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{Force: true}); err != nil {
			fmt.Printf("warning: removing container %s: %v\n", ctr.ID, err)
		}
	}

	// Remove all images.
	for _, img := range images {
		if _, err := api.ImageRemove(ctx, img.ID, image.RemoveOptions{Force: true}); err != nil {
			tag := img.ID
			if len(img.RepoTags) > 0 {
				tag = img.RepoTags[0]
			}
			fmt.Printf("warning: removing image %s: %v\n", tag, err)
		}
	}

	return nil
}

func purgeFilter() filters.Args {
	f := filters.NewArgs()
	f.Add("label", "bac.managed=true")
	return f
}
