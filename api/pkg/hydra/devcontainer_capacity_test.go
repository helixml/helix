package hydra

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReserveContainerSlotConcurrentLimit(t *testing.T) {
	dm := &DevContainerManager{}
	start := make(chan struct{})
	type result struct {
		release func()
		err     error
	}
	results := make(chan result, 2)

	for _, sessionID := range []string{"ses_1", "ses_2"} {
		go func() {
			<-start
			release, err := dm.reserveContainerSlot(context.Background(), sessionID, 1, func() (map[string]struct{}, error) {
				return map[string]struct{}{}, nil
			})
			results <- result{release: release, err: err}
		}()
	}
	close(start)

	first := <-results
	second := <-results
	successes := 0
	for _, got := range []result{first, second} {
		if got.err == nil {
			successes++
			got.release()
		} else {
			assert.ErrorContains(t, got.err, "runner is at capacity")
		}
	}
	assert.Equal(t, 1, successes)
}

func TestReserveContainerSlotReleaseAllowsNextStart(t *testing.T) {
	dm := &DevContainerManager{}
	running := func() (map[string]struct{}, error) { return map[string]struct{}{}, nil }

	release, err := dm.reserveContainerSlot(context.Background(), "ses_failed", 1, running)
	require.NoError(t, err)
	release()

	release, err = dm.reserveContainerSlot(context.Background(), "ses_retry", 1, running)
	require.NoError(t, err)
	release()
}

func TestReserveContainerSlotWaitsForSameSession(t *testing.T) {
	dm := &DevContainerManager{}
	var mu sync.Mutex
	running := false
	listRunning := func() (map[string]struct{}, error) {
		mu.Lock()
		defer mu.Unlock()
		if running {
			return map[string]struct{}{"ses_1": {}}, nil
		}
		return map[string]struct{}{}, nil
	}

	releaseFirst, err := dm.reserveContainerSlot(context.Background(), "ses_1", 1, listRunning)
	require.NoError(t, err)

	result := make(chan error, 1)
	go func() {
		release, err := dm.reserveContainerSlot(context.Background(), "ses_1", 1, listRunning)
		if err == nil {
			release()
		}
		result <- err
	}()

	mu.Lock()
	running = true
	mu.Unlock()
	releaseFirst()
	require.NoError(t, <-result)
}

func TestReserveContainerSlotRejectsRunningListFailure(t *testing.T) {
	dm := &DevContainerManager{}
	_, err := dm.reserveContainerSlot(context.Background(), "ses_1", 1, func() (map[string]struct{}, error) {
		return nil, errors.New("dockerd unavailable")
	})
	require.ErrorContains(t, err, "dockerd unavailable")
}

func TestRunningCapacitySessionsExcludesGoldenBuilds(t *testing.T) {
	server := newDockerListServer(t, `[
		{"Id":"normal","Names":["/ubuntu-external-normal"],"Labels":{"helix.session_id":"ses_normal"}},
		{"Id":"golden","Names":["/ubuntu-external-golden"],"Labels":{"helix.session_id":"ses_golden","helix.golden_build":"true"}}
	]`)
	dockerClient, err := client.NewClientWithOpts(
		client.WithHost(strings.Replace(server.URL, "http://", "tcp://", 1)),
		client.WithHTTPClient(server.Client()),
		client.WithVersion("1.43"),
	)
	require.NoError(t, err)
	defer dockerClient.Close()

	sessions, err := runningCapacitySessions(context.Background(), dockerClient)
	require.NoError(t, err)
	assert.Equal(t, map[string]struct{}{"ses_normal": {}}, sessions)
}

func TestTrackedExistingContainerPreservesGoldenBuild(t *testing.T) {
	tracked := trackedExistingContainer(
		&CreateDevContainerRequest{SessionID: "ses_golden", ContainerName: "golden"},
		dockertypes.ContainerJSON{
			ContainerJSONBase: &dockertypes.ContainerJSONBase{ID: "container-1"},
			Config: &container.Config{Labels: map[string]string{
				containerGoldenBuildLabel: "true",
				containerProjectIDLabel:   "prj_1",
			}},
		},
		"10.0.0.1",
	)

	assert.True(t, tracked.IsGoldenBuild)
	assert.Equal(t, "prj_1", tracked.ProjectID)
}

func TestCreateDevContainerEnforcesRunnerCapacity(t *testing.T) {
	t.Setenv("HELIX_DISK_PRESSURE_ENABLED", "false")
	resetDiskPressureConfig()
	t.Cleanup(resetDiskPressureConfig)

	socketPath := newUnixDockerListServer(t, `[{
		"Id":"existing","Names":["/ubuntu-external-existing"],"Labels":{"helix.session_id":"ses_existing"}
	}]`)

	dm := &DevContainerManager{
		containers:         make(map[string]*DevContainer),
		creatingContainers: make(map[string]chan struct{}),
		maxSandboxes:       1,
	}
	_, err := dm.CreateDevContainer(context.Background(), &CreateDevContainerRequest{
		SessionID:           "ses_new",
		Image:               "ubuntu:22.04",
		ContainerName:       "ubuntu-external-new",
		ContainerType:       DevContainerTypeHeadless,
		DockerSocket:        socketPath,
		SkipImageValidation: true,
		MaxSandboxes:        20,
	})
	require.ErrorContains(t, err, "runner is at capacity (1/1 dev containers)")
}

func TestCreateDevContainerUnlimitedRunnerOverridesAPICapacity(t *testing.T) {
	t.Setenv("HELIX_DISK_PRESSURE_ENABLED", "false")
	resetDiskPressureConfig()
	t.Cleanup(resetDiskPressureConfig)

	socketPath := newUnixDockerListServer(t, `[{"Id":"existing","Names":["/ubuntu-external-existing"],"Labels":{"helix.session_id":"ses_existing"}}]`)
	dm := &DevContainerManager{
		containers:         make(map[string]*DevContainer),
		creatingContainers: make(map[string]chan struct{}),
		maxSandboxes:       -1,
	}
	_, err := dm.CreateDevContainer(context.Background(), &CreateDevContainerRequest{
		SessionID:           "ses_new",
		Image:               "ubuntu:22.04",
		ContainerName:       "ubuntu-external-new",
		ContainerType:       DevContainerTypeHeadless,
		DockerSocket:        socketPath,
		SkipImageValidation: true,
		MaxSandboxes:        1,
		Network:             "forbidden",
	})
	require.ErrorContains(t, err, "unsupported sandbox network")
	assert.NotContains(t, err.Error(), "runner is at capacity")
}

func TestCreateDevContainerFailureReleasesRunnerCapacity(t *testing.T) {
	t.Setenv("HELIX_DISK_PRESSURE_ENABLED", "false")
	resetDiskPressureConfig()
	t.Cleanup(resetDiskPressureConfig)

	socketPath := newUnixDockerListServer(t, `[]`)
	dm := &DevContainerManager{
		containers:         make(map[string]*DevContainer),
		creatingContainers: make(map[string]chan struct{}),
	}
	start := func(sessionID string) error {
		_, err := dm.CreateDevContainer(context.Background(), &CreateDevContainerRequest{
			SessionID:           sessionID,
			Image:               "ubuntu:22.04",
			ContainerName:       "ubuntu-external-" + sessionID,
			ContainerType:       DevContainerTypeHeadless,
			DockerSocket:        socketPath,
			SkipImageValidation: true,
			MaxSandboxes:        1,
			Network:             "forbidden",
		})
		return err
	}

	require.ErrorContains(t, start("ses_failed"), "unsupported sandbox network")
	err := start("ses_after_failure")
	require.ErrorContains(t, err, "unsupported sandbox network")
	assert.NotContains(t, err.Error(), "runner is at capacity")
}

func newDockerListServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(dockerListHandler(body))
	t.Cleanup(server.Close)
	return server
}

func dockerListHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/_ping") {
			w.Header().Set("API-Version", "1.43")
			_, _ = w.Write([]byte("OK"))
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	})
}

func newUnixDockerListServer(t *testing.T, body string) string {
	t.Helper()
	socketFile, err := os.CreateTemp("/tmp", "helix-hydra-capacity-*.sock")
	require.NoError(t, err)
	socketPath := socketFile.Name()
	require.NoError(t, socketFile.Close())
	require.NoError(t, os.Remove(socketPath))
	t.Cleanup(func() { _ = os.Remove(socketPath) })

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	server := &http.Server{Handler: dockerListHandler(body)}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return socketPath
}
