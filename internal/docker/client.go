package docker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	dockerclient "github.com/docker/docker/client"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/build"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
)

// Client wraps the Docker SDK client and exposes the methods used by the tool.
type Client struct {
	inner *dockerclient.Client
}

// NewClient creates a Docker SDK client, pings the daemon, and returns a
// ready-to-use Client. Returns a descriptive error if the daemon is unreachable.
func NewClient() (*Client, error) {
	inner, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating Docker client: %w", err)
	}
	ctx := context.Background()
	if _, err := inner.Ping(ctx); err != nil {
		return nil, fmt.Errorf("Docker daemon is not reachable — make sure Docker is running: %w", err)
	}
	return &Client{inner: inner}, nil
}

// CheckVersion returns an error if the connected daemon's version is older than
// constants.MinDockerVersion. The error message includes both the detected and
// required versions.
func CheckVersion(c *Client) error {
	ctx := context.Background()
	v, err := c.inner.ServerVersion(ctx)
	if err != nil {
		return fmt.Errorf("querying Docker server version: %w", err)
	}
	if !IsVersionCompatible(v.Version) {
		return fmt.Errorf(
			"Docker version %s is too old; %s or newer is required",
			v.Version, constants.MinDockerVersion,
		)
	}
	return nil
}

// IsVersionCompatible reports whether the given Docker version string meets the
// minimum requirement (constants.MinDockerVersion = "20.10").
// It parses "major.minor[.patch[-extra]]" and returns true iff
// major > 20 || (major == 20 && minor >= 10).
func IsVersionCompatible(version string) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minorStr := strings.SplitN(parts[1], "-", 2)[0]
	minor, err := strconv.Atoi(minorStr)
	if err != nil {
		return false
	}
	return major > 20 || (major == 20 && minor >= 10)
}

// --- Delegating methods -------------------------------------------------------

func (c *Client) Ping(ctx context.Context) (types.Ping, error) {
	return c.inner.Ping(ctx)
}

func (c *Client) ServerVersion(ctx context.Context) (types.Version, error) {
	return c.inner.ServerVersion(ctx)
}

func (c *Client) ImageInspectWithRaw(ctx context.Context, imageID string) (image.InspectResponse, []byte, error) {
	return c.inner.ImageInspectWithRaw(ctx, imageID)
}

func (c *Client) ImageBuild(ctx context.Context, buildContext io.Reader, options build.ImageBuildOptions) (build.ImageBuildResponse, error) {
	return c.inner.ImageBuild(ctx, buildContext, options)
}

func (c *Client) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	return c.inner.ContainerInspect(ctx, containerID)
}

func (c *Client) ContainerCreate(
	ctx context.Context,
	config *container.Config,
	hostConfig *container.HostConfig,
	networkingConfig *network.NetworkingConfig,
	platform *ocispec.Platform,
	containerName string,
) (container.CreateResponse, error) {
	return c.inner.ContainerCreate(ctx, config, hostConfig, networkingConfig, platform, containerName)
}

func (c *Client) ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error {
	return c.inner.ContainerStart(ctx, containerID, options)
}

func (c *Client) ContainerStop(ctx context.Context, containerID string, options container.StopOptions) error {
	return c.inner.ContainerStop(ctx, containerID, options)
}

func (c *Client) ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error {
	return c.inner.ContainerRemove(ctx, containerID, options)
}

func (c *Client) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	return c.inner.ContainerList(ctx, options)
}

func (c *Client) ImageList(ctx context.Context, options image.ListOptions) ([]image.Summary, error) {
	return c.inner.ImageList(ctx, options)
}

func (c *Client) ImageRemove(ctx context.Context, imageID string, options image.RemoveOptions) ([]image.DeleteResponse, error) {
	return c.inner.ImageRemove(ctx, imageID, options)
}

func (c *Client) ContainerExecCreate(ctx context.Context, containerID string, options container.ExecOptions) (container.ExecCreateResponse, error) {
	return c.inner.ContainerExecCreate(ctx, containerID, options)
}

func (c *Client) ContainerExecAttach(ctx context.Context, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error) {
	return c.inner.ContainerExecAttach(ctx, execID, config)
}

func (c *Client) ContainerExecInspect(ctx context.Context, execID string) (container.ExecInspect, error) {
	return c.inner.ContainerExecInspect(ctx, execID)
}

func (c *Client) ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	return c.inner.ContainerWait(ctx, containerID, condition)
}

func (c *Client) ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error) {
	return c.inner.ContainerLogs(ctx, containerID, options)
}

func (c *Client) ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error) {
	return c.inner.ImagePull(ctx, refStr, options)
}

// --- User conflict detection --------------------------------------------------

// ImageUser represents a user entry found in the base image's /etc/passwd.
type ImageUser struct {
	Username string
	UID      int
	GID      int
}

// FindConflictingUser inspects constants.BaseContainerImage and returns the first
// user whose UID or GID matches the given uid/gid. Returns (nil, nil) if no
// conflict exists. Returns (nil, err) on any Docker error.
//
// It ensures the base image is present locally (pulling it if needed), then
// runs a short-lived container with "getent passwd", waits for it to exit,
// reads its stdout, then removes the container.
func FindConflictingUser(ctx context.Context, client *Client, uid, gid int) (*ImageUser, error) {
	// 0. Ensure the base image is present locally; pull it if not.
	// ImageInspectWithRaw is a cheap local-only check — no network call.
	if _, _, err := client.ImageInspectWithRaw(ctx, constants.BaseContainerImage); err != nil {
		fmt.Printf("Pulling base image %s...\n", constants.BaseContainerImage)
		rc, pullErr := client.ImagePull(ctx, constants.BaseContainerImage, image.PullOptions{})
		if pullErr != nil {
			return nil, fmt.Errorf("pulling base image %s: %w", constants.BaseContainerImage, pullErr)
		}
		// Drain the pull stream so the pull completes before we proceed.
		_, _ = io.Copy(io.Discard, rc)
		rc.Close()
	}

	// 1. Create container (no AutoRemove so we can read logs after exit)
	resp, err := client.ContainerCreate(ctx, &container.Config{
		Image: constants.BaseContainerImage,
		Cmd:   []string{"getent", "passwd"},
	}, &container.HostConfig{}, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("creating passwd inspection container: %w", err)
	}
	containerID := resp.ID

	// Ensure the container is removed when we're done, regardless of outcome.
	defer func() {
		_ = client.ContainerRemove(context.Background(), containerID, container.RemoveOptions{Force: true})
	}()

	// 2. Start the container
	if err := client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("starting passwd inspection container: %w", err)
	}

	// 3. Wait for the container to finish
	statusCh, errCh := client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case waitErr := <-errCh:
		if waitErr != nil {
			return nil, fmt.Errorf("waiting for passwd inspection container: %w", waitErr)
		}
	case <-statusCh:
	}

	// 4. Retrieve stdout logs
	out, err := client.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true})
	if err != nil {
		return nil, fmt.Errorf("reading passwd output: %w", err)
	}
	defer out.Close()

	// 5. Demultiplex the Docker log stream (8-byte header per chunk)
	var stdout bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, io.Discard, out); err != nil {
		return nil, fmt.Errorf("demultiplexing passwd output: %w", err)
	}

	// 6. Parse /etc/passwd format: username:x:uid:gid:gecos:home:shell
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ":")
		if len(fields) < 4 {
			continue
		}
		entryUID, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		entryGID, err := strconv.Atoi(fields[3])
		if err != nil {
			continue
		}
		if entryUID == uid || entryGID == gid {
			return &ImageUser{
				Username: fields[0],
				UID:      entryUID,
				GID:      entryGID,
			}, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning passwd output: %w", err)
	}

	return nil, nil
}
