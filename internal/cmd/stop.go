package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"

	"github.com/koudis/bootstrap-ai-coding/internal/datadir"
	"github.com/koudis/bootstrap-ai-coding/internal/naming"
	sshpkg "github.com/koudis/bootstrap-ai-coding/internal/ssh"
)

// StopDockerAPI is the subset of Docker operations used by the stop-and-remove flow.
// It exists to enable unit testing without a live Docker daemon.
type StopDockerAPI interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ImageRemove(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error)
}

// RunStopWith implements the stop-and-remove flow against the StopDockerAPI interface.
// This is the testable core; runStop delegates to it after constructing the real client.
func RunStopWith(api StopDockerAPI, projectPath string) error {
	ctx := context.Background()

	// List existing bac-managed container names (filter by bac.managed label).
	containers, err := api.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: bacManagedFilter(),
	})
	if err != nil {
		return fmt.Errorf("listing existing containers: %w", err)
	}
	var existingNames []string
	for _, ctr := range containers {
		for _, n := range ctr.Names {
			existingNames = append(existingNames, strings.TrimPrefix(n, "/"))
		}
	}

	containerName, err := naming.ContainerName(projectPath, existingNames)
	if err != nil {
		return fmt.Errorf("deriving container name: %w", err)
	}

	info, inspectErr := api.ContainerInspect(ctx, containerName)
	if inspectErr != nil {
		if strings.Contains(inspectErr.Error(), "No such container") {
			fmt.Printf("No container found for project %s\n", projectPath)
			return nil
		}
		return inspectErr
	}
	_ = info // container exists

	if err := api.ContainerStop(ctx, containerName, container.StopOptions{}); err != nil {
		if !strings.Contains(err.Error(), "not running") {
			return err
		}
	}
	if err := api.ContainerRemove(ctx, containerName, container.RemoveOptions{Force: true}); err != nil {
		return err
	}
	fmt.Printf("Container %s stopped and removed.\n", containerName)

	// Remove known_hosts entries for this project's SSH port (Req 18.7).
	dd, err := datadir.New(containerName)
	if err == nil {
		if port, err := dd.ReadPort(); err == nil && port != 0 {
			if khErr := sshpkg.RemoveKnownHostsEntries(port); khErr != nil {
				fmt.Fprintf(os.Stderr, "warning: removing known_hosts entries: %v\n", khErr)
			}
		}
	}

	// Remove SSH config entry for this container (Req 19.7).
	if cfgErr := sshpkg.RemoveSSHConfigEntry(containerName); cfgErr != nil {
		fmt.Fprintf(os.Stderr, "warning: removing SSH config entry: %v\n", cfgErr)
	}

	return nil
}

// bacManagedFilter returns a Docker filter for bac-managed resources.
func bacManagedFilter() filters.Args {
	f := filters.NewArgs()
	f.Add("label", "bac.managed=true")
	return f
}
