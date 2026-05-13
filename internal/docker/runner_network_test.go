package docker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/require"

	"github.com/koudis/bootstrap-ai-coding/internal/constants"
	"github.com/koudis/bootstrap-ai-coding/internal/docker"
)

// createRequest captures the JSON body sent to the Docker daemon's
// /containers/create endpoint. The Docker SDK sends container.Config and
// container.HostConfig as a combined JSON payload.
type createRequest struct {
	HostConfig container.HostConfig `json:"HostConfig"`
	// ExposedPorts in the container config
	ExposedPorts nat.PortSet `json:"ExposedPorts"`
}

// newFakeDockerClient creates a *docker.Client backed by a fake HTTP server
// that captures the ContainerCreate request body. The returned channel receives
// the decoded createRequest when ContainerCreate is called.
func newFakeDockerClient(t *testing.T) (*docker.Client, chan createRequest) {
	t.Helper()
	ch := make(chan createRequest, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Match the containers/create endpoint (path includes version prefix)
		if r.Method == http.MethodPost && r.URL.Path[len(r.URL.Path)-18:] == "/containers/create" {
			var req createRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode create request: %v", err)
			}
			ch <- req
			// Return a valid CreateResponse
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"Id":"fake-container-id","Warnings":[]}`)
			return
		}
		// Default: return 200 for version negotiation (_ping, version, etc.)
		if r.URL.Path == "/_ping" || r.URL.Path == "/v1.47/_ping" {
			w.Header().Set("Api-Version", "1.47")
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "OK")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Create a Docker SDK client pointing at our fake server.
	inner, err := dockerclient.NewClientWithOpts(
		dockerclient.WithHost(srv.URL),
		dockerclient.WithHTTPClient(srv.Client()),
		dockerclient.WithAPIVersionNegotiation(),
	)
	require.NoError(t, err)

	// Wrap in our Client type using the exported constructor pattern.
	// Since Client wraps the inner client, we use NewClientFromInner (test helper).
	client := docker.NewClientForTest(inner)
	return client, ch
}

// TestCreateContainerHostNetworkMode verifies that CreateContainer sets
// NetworkMode: "host" and no port bindings when HostNetworkOff == false.
func TestCreateContainerHostNetworkMode(t *testing.T) {
	client, ch := newFakeDockerClient(t)

	spec := docker.ContainerSpec{
		Name:           "bac-test-host",
		ImageTag:       "bac-test:latest",
		SSHPort:        2222,
		HostNetworkOff: false,
		Labels:         map[string]string{"bac.managed": "true"},
	}

	id, err := docker.CreateContainer(context.Background(), client, spec)
	require.NoError(t, err)
	require.Equal(t, "fake-container-id", id)

	req := <-ch
	require.Equal(t, container.NetworkMode("host"), req.HostConfig.NetworkMode,
		"HostNetworkOff=false must set NetworkMode to 'host'")
	require.Empty(t, req.HostConfig.PortBindings,
		"HostNetworkOff=false must not set PortBindings")
	require.Empty(t, req.ExposedPorts,
		"HostNetworkOff=false must not set ExposedPorts")
}

// TestCreateContainerBridgeMode verifies that CreateContainer uses port bindings
// and does not set NetworkMode: "host" when HostNetworkOff == true.
func TestCreateContainerBridgeMode(t *testing.T) {
	client, ch := newFakeDockerClient(t)

	spec := docker.ContainerSpec{
		Name:           "bac-test-bridge",
		ImageTag:       "bac-test:latest",
		SSHPort:        3333,
		HostNetworkOff: true,
		Labels:         map[string]string{"bac.managed": "true"},
	}

	id, err := docker.CreateContainer(context.Background(), client, spec)
	require.NoError(t, err)
	require.Equal(t, "fake-container-id", id)

	req := <-ch

	// NetworkMode should NOT be "host"
	require.NotEqual(t, container.NetworkMode("host"), req.HostConfig.NetworkMode,
		"HostNetworkOff=true must not set NetworkMode to 'host'")

	// PortBindings should map ContainerSSHPort/tcp → HostBindIP:SSHPort
	sshPort := nat.Port(fmt.Sprintf("%d/tcp", constants.ContainerSSHPort))
	bindings, ok := req.HostConfig.PortBindings[sshPort]
	require.True(t, ok, "HostNetworkOff=true must set PortBindings for %s", sshPort)
	require.Len(t, bindings, 1)
	require.Equal(t, constants.HostBindIP, bindings[0].HostIP)
	require.Equal(t, fmt.Sprintf("%d", spec.SSHPort), bindings[0].HostPort)

	// ExposedPorts should include the SSH port
	_, exposed := req.ExposedPorts[sshPort]
	require.True(t, exposed, "HostNetworkOff=true must set ExposedPorts for %s", sshPort)
}
