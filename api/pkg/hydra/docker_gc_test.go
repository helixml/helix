package hydra

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

// serveFakeDocker serves handler on a fresh unix socket and returns its path.
func serveFakeDocker(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	socketFile, err := os.CreateTemp("/tmp", "helix-hydra-gc-*.sock")
	require.NoError(t, err)
	socketPath := socketFile.Name()
	require.NoError(t, socketFile.Close())
	require.NoError(t, os.Remove(socketPath))
	t.Cleanup(func() { _ = os.Remove(socketPath) })

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/_ping") {
			w.Header().Set("API-Version", "1.43")
			_, _ = w.Write([]byte("OK"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		handler(w, r)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return socketPath
}

func dockerNotFound(w http.ResponseWriter, id string) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"message":"No such container: ` + id + `"}`))
}

// containerID returns the id in /v<version>/containers/<id>[/json].
func containerID(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/json"), "/")
	return parts[len(parts)-1]
}

// A desktop stopped without a destroy — the disk-pressure emergency brake —
// stays tracked. It used to count as live forever, so the reaper never removed
// its container, zvol or workspace (observed: six desktops left Exited for ten
// days after their sessions ended). Only a container Docker confirms running,
// or one Docker cannot be asked about, may keep its session live.
func TestRunningTrackedSessions(t *testing.T) {
	states := map[string]string{"cid_running": "true", "cid_stopped": "false"}
	socket := serveFakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		id := containerID(r.URL.Path)
		running, ok := states[id]
		if !ok {
			dockerNotFound(w, id)
			return
		}
		_, _ = w.Write([]byte(`{"Id":"` + id + `","State":{"Running":` + running + `}}`))
	})
	unreachable := filepath.Join(t.TempDir(), "no-docker.sock")
	removed := &DevContainer{SessionID: "ses_removed", ContainerID: "cid_removed", DockerSocket: socket}
	dm := &DevContainerManager{containers: map[string]*DevContainer{
		"ses_running":     {SessionID: "ses_running", ContainerID: "cid_running", DockerSocket: socket},
		"ses_stopped":     {SessionID: "ses_stopped", ContainerID: "cid_stopped", DockerSocket: socket},
		"ses_removed":     removed,
		"ses_unreachable": {SessionID: "ses_unreachable", ContainerID: "cid_any", DockerSocket: unreachable},
	}}

	running, gone := dm.runningTrackedSessions(context.Background())

	require.ElementsMatch(t, []string{"ses_running", "ses_unreachable"}, running)
	require.Equal(t, []*DevContainer{removed}, gone)
	// Listing is read-only, so a dry-run reconcile leaves the map alone.
	require.Len(t, dm.containers, 4)

	dm.forgetContainers(gone)
	// A removed container is forgotten so hydra stops listing it; a stopped
	// one stays tracked so a warm restart can still find it.
	require.NotContains(t, dm.containers, "ses_removed")
	require.Contains(t, dm.containers, "ses_stopped")
}

// The reaper inspects a container, then removes it. If the session restarts
// the container in between, a forced removal would destroy a desktop someone
// is using. Removal must let Docker refuse a container that is running again.
func TestReapOrphanContainersNeverRemovesARestartedContainer(t *testing.T) {
	for _, tc := range []struct {
		name               string
		restartedMeanwhile bool
		wantReaped         []string
	}{
		{name: "still stopped", restartedMeanwhile: false, wantReaped: []string{"ubuntu-external-01dead"}},
		{name: "restarted between inspect and remove", restartedMeanwhile: true, wantReaped: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var removeForced bool
			socket := serveFakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
					_, _ = w.Write([]byte(`[{"Id":"cid_dead","Names":["/ubuntu-external-01dead"],"State":"exited","Labels":{"helix.session_id":"ses_01dead"}}]`))
				case r.Method == http.MethodGet:
					_, _ = w.Write([]byte(`{"Id":"cid_dead","Name":"/ubuntu-external-01dead","State":{"Running":false,"FinishedAt":"2026-01-01T00:00:00Z"}}`))
				case r.Method == http.MethodDelete:
					removeForced = r.URL.Query().Get("force") == "1"
					if tc.restartedMeanwhile && !removeForced {
						w.WriteHeader(http.StatusConflict)
						_, _ = w.Write([]byte(`{"message":"cannot remove container: container is running"}`))
						return
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					dockerNotFound(w, containerID(r.URL.Path))
				}
			})
			dockerClient, err := client.NewClientWithOpts(client.WithHost("unix://"+socket), client.WithAPIVersionNegotiation())
			require.NoError(t, err)
			defer dockerClient.Close()

			reaped := reapOrphanContainers(context.Background(), dockerClient, map[string]bool{}, time.Now(), false)

			require.False(t, removeForced, "orphan removal must not be forced")
			require.Equal(t, tc.wantReaped, reaped)
		})
	}
}

// A zero or negative interval reaches time.NewTicker in the disk-pressure
// monitor goroutine, which panics and takes hydra down.
func TestEnvDurationRejectsNonPositive(t *testing.T) {
	for _, v := range []string{"0", "0s", "-5s"} {
		t.Setenv("HELIX_TEST_DURATION", v)
		require.Equal(t, 30*time.Second, envDuration("HELIX_TEST_DURATION", 30*time.Second), v)
	}
	t.Setenv("HELIX_TEST_DURATION", "45s")
	require.Equal(t, 45*time.Second, envDuration("HELIX_TEST_DURATION", 30*time.Second))
}

// Every float env is a free-space percentage. NaN compares false against any
// threshold, silently disabling admission refusal and the emergency brake.
func TestEnvFloatRejectsNonPercentages(t *testing.T) {
	for _, v := range []string{"NaN", "Inf", "-1", "101"} {
		t.Setenv("HELIX_TEST_PCT", v)
		require.Equal(t, 5.0, envFloat("HELIX_TEST_PCT", 5.0), v)
	}
	t.Setenv("HELIX_TEST_PCT", "2.5")
	require.Equal(t, 2.5, envFloat("HELIX_TEST_PCT", 5.0))
}

// The reaper destroys desktops, so the rules that keep one are pinned at the
// Docker API: a live session's, a sandbox-API (sbx_) one, a running one, and
// one that stopped within the grace period must all survive. Only a dead
// session's container past grace — here one that never started, so its age
// comes from creation — is removed.
func TestReapOrphanContainersKeepsEverythingButDeadPastGrace(t *testing.T) {
	now := time.Now()
	old := now.Add(-60 * 24 * time.Hour)
	recent := now.Add(-time.Hour)
	type fake struct {
		session, state string
		finished       time.Time
		created        time.Time
	}
	containers := map[string]fake{
		"cid_live":       {session: "ses_01live", state: "exited", finished: old, created: old},
		"cid_sbx":        {session: "sbx_01api", state: "exited", finished: old, created: old},
		"cid_running":    {session: "ses_01run", state: "running", finished: old, created: old},
		"cid_recent":     {session: "ses_01recent", state: "exited", finished: recent, created: old},
		"cid_neverstart": {session: "ses_01never", state: "created", created: old},
	}
	var removed []string
	socket := serveFakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			list := make([]string, 0, len(containers))
			for id, c := range containers {
				list = append(list, `{"Id":"`+id+`","Names":["/`+id+`"],"State":"`+c.state+`","Created":`+
					strconv.FormatInt(c.created.Unix(), 10)+`,"Labels":{"helix.session_id":"`+c.session+`"}}`)
			}
			_, _ = w.Write([]byte("[" + strings.Join(list, ",") + "]"))
		case r.Method == http.MethodGet:
			id := containerID(r.URL.Path)
			c := containers[id]
			finished := "0001-01-01T00:00:00Z"
			if !c.finished.IsZero() {
				finished = c.finished.UTC().Format(time.RFC3339Nano)
			}
			_, _ = w.Write([]byte(`{"Id":"` + id + `","Name":"/` + id + `","State":{"Running":` +
				strconv.FormatBool(c.state == "running") + `,"FinishedAt":"` + finished + `"}}`))
		case r.Method == http.MethodDelete:
			removed = append(removed, containerID(r.URL.Path))
			w.WriteHeader(http.StatusNoContent)
		default:
			dockerNotFound(w, containerID(r.URL.Path))
		}
	})
	dockerClient, err := client.NewClientWithOpts(client.WithHost("unix://"+socket), client.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer dockerClient.Close()

	reaped := reapOrphanContainers(context.Background(), dockerClient, map[string]bool{"ses_01live": true}, now.Add(-30*24*time.Hour), false)

	require.Equal(t, []string{"cid_neverstart"}, reaped)
	require.Equal(t, []string{"cid_neverstart"}, removed)
}
