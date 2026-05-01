package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/go-connections/nat"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
)

// Mount represents a single Docker bind mount.
type Mount struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// ContainerSpec is the fully resolved specification for a container.
type ContainerSpec struct {
	Name       string            // Deterministic container name (bac-<12hex>)
	ImageTag   string            // Docker image tag (derived from container name)
	Dockerfile string            // Complete Dockerfile content (assembled by DockerfileBuilder)
	Mounts     []Mount           // All bind mounts: /workspace + per-agent credential stores
	SSHPort    int               // Host-side TCP port mapped to container port 22
	Labels     map[string]string // Docker labels for identification
	HostUID    int               // Host user UID (passed as build arg for dev user)
	HostGID    int               // Host user GID (passed as build arg for dev user)
}

func buildContextFromDockerfile(dockerfile string) (io.Reader, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := []byte(dockerfile)
	if err := tw.WriteHeader(&tar.Header{
		Name: "Dockerfile",
		Mode: 0o644,
		Size: int64(len(content)),
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(content); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}

// BuildImage builds a Docker image from the spec's Dockerfile and tags it with spec.ImageTag.
func BuildImage(ctx context.Context, c *Client, spec ContainerSpec) (string, error) {
	buildCtx, err := buildContextFromDockerfile(spec.Dockerfile)
	if err != nil {
		return "", fmt.Errorf("creating build context: %w", err)
	}
	resp, err := c.ImageBuild(ctx, buildCtx, build.ImageBuildOptions{
		Tags:       []string{spec.ImageTag},
		Dockerfile: "Dockerfile",
		Remove:     true,
		Labels:     spec.Labels,
	})
	if err != nil {
		return "", fmt.Errorf("starting image build: %w", err)
	}
	defer resp.Body.Close()

	var out strings.Builder
	dec := json.NewDecoder(resp.Body)
	for {
		var msg struct {
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}
		if err := dec.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return out.String(), fmt.Errorf("reading build output: %w", err)
		}
		if msg.Error != "" {
			return out.String(), fmt.Errorf("build error: %s", msg.Error)
		}
		out.WriteString(msg.Stream)
	}
	return out.String(), nil
}

// CreateContainer creates a container from the given spec.
func CreateContainer(ctx context.Context, c *Client, spec ContainerSpec) (string, error) {
	sshPort := nat.Port(fmt.Sprintf("%d/tcp", constants.ContainerSSHPort))
	portBindings := nat.PortMap{
		sshPort: []nat.PortBinding{
			{HostIP: constants.KnownHostsPatterns[1], HostPort: fmt.Sprintf("%d", spec.SSHPort)},
		},
	}
	exposedPorts := nat.PortSet{sshPort: struct{}{}}

	var mounts []mount.Mount
	for _, m := range spec.Mounts {
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   m.HostPath,
			Target:   m.ContainerPath,
			ReadOnly: m.ReadOnly,
		})
	}

	resp, err := c.ContainerCreate(
		ctx,
		&container.Config{
			Image:        spec.ImageTag,
			Labels:       spec.Labels,
			ExposedPorts: exposedPorts,
		},
		&container.HostConfig{
			PortBindings: portBindings,
			Mounts:       mounts,
		},
		nil,
		nil,
		spec.Name,
	)
	if err != nil {
		return "", fmt.Errorf("creating container %s: %w", spec.Name, err)
	}
	return resp.ID, nil
}

// StartContainer starts the container with the given name or ID.
func StartContainer(ctx context.Context, c *Client, nameOrID string) error {
	if err := c.ContainerStart(ctx, nameOrID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting container %s: %w", nameOrID, err)
	}
	return nil
}

// StopContainer stops the container with the given name or ID.
func StopContainer(ctx context.Context, c *Client, nameOrID string) error {
	if err := c.ContainerStop(ctx, nameOrID, container.StopOptions{}); err != nil {
		return fmt.Errorf("stopping container %s: %w", nameOrID, err)
	}
	return nil
}

// RemoveContainer removes the container with the given name or ID.
func RemoveContainer(ctx context.Context, c *Client, nameOrID string) error {
	if err := c.ContainerRemove(ctx, nameOrID, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("removing container %s: %w", nameOrID, err)
	}
	return nil
}

// InspectContainer returns detailed information about the named container.
// Returns (nil, nil) if the container does not exist.
func InspectContainer(ctx context.Context, c *Client, nameOrID string) (*container.InspectResponse, error) {
	info, err := c.ContainerInspect(ctx, nameOrID)
	if err != nil {
		if strings.Contains(err.Error(), "No such container") {
			return nil, nil
		}
		return nil, fmt.Errorf("inspecting container %s: %w", nameOrID, err)
	}
	return &info, nil
}

// WaitForSSH polls host:port with a TCP dial until the connection succeeds or the timeout elapses.
func WaitForSSH(ctx context.Context, host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for SSH on %s after %s", addr, timeout)
}

func bacLabelFilter() filters.Args {
	f := filters.NewArgs()
	f.Add("label", "bac.managed=true")
	return f
}

// ListBACContainers returns all containers managed by this tool (running or stopped).
func ListBACContainers(ctx context.Context, c *Client) ([]container.Summary, error) {
	containers, err := c.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: bacLabelFilter(),
	})
	if err != nil {
		return nil, fmt.Errorf("listing bac containers: %w", err)
	}
	return containers, nil
}

// ListBACContainerNames returns the names of all bac-managed containers (running or stopped).
// The returned names are stripped of the leading "/" that Docker prepends to container names.
// This is the list passed to naming.ContainerName as existingNames.
func ListBACContainerNames(ctx context.Context, c *Client) ([]string, error) {
	containers, err := ListBACContainers(ctx, c)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(containers))
	for _, ctr := range containers {
		for _, n := range ctr.Names {
			// Docker prepends "/" to container names; strip it.
			names = append(names, strings.TrimPrefix(n, "/"))
		}
	}
	return names, nil
}

// ListBACImages returns all images managed by this tool.
func ListBACImages(ctx context.Context, c *Client) ([]image.Summary, error) {
	images, err := c.ImageList(ctx, image.ListOptions{
		Filters: bacLabelFilter(),
	})
	if err != nil {
		return nil, fmt.Errorf("listing bac images: %w", err)
	}
	if len(images) == 0 {
		all, err := c.ImageList(ctx, image.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("listing all images: %w", err)
		}
		for _, img := range all {
			for _, tag := range img.RepoTags {
				if strings.HasPrefix(tag, constants.ContainerNamePrefix) {
					images = append(images, img)
					break
				}
			}
		}
	}
	return images, nil
}

// ExecInContainer runs a command inside a running container and returns the exit code.
func ExecInContainer(ctx context.Context, containerID string, cmd []string) (int, error) {
	c, err := NewClient()
	if err != nil {
		return -1, fmt.Errorf("connecting to Docker for exec: %w", err)
	}

	execID, err := c.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return -1, fmt.Errorf("creating exec in container %s: %w", containerID, err)
	}

	resp, err := c.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return -1, fmt.Errorf("attaching to exec in container %s: %w", containerID, err)
	}
	resp.Close()

	for {
		inspect, err := c.ContainerExecInspect(ctx, execID.ID)
		if err != nil {
			return -1, fmt.Errorf("inspecting exec in container %s: %w", containerID, err)
		}
		if !inspect.Running {
			return inspect.ExitCode, nil
		}
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
