package hydra

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newUnixDockerInspectServer serves ContainerInspect for the given container
// states ("running" or "exited") and 404s every other container.
func newUnixDockerInspectServer(t *testing.T, states map[string]string) string {
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
		// GET /v<version>/containers/<id>/json
		parts := strings.Split(r.URL.Path, "/")
		id := parts[len(parts)-2]
		state, ok := states[id]
		if r.Method != http.MethodGet || parts[len(parts)-1] != "json" || !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"No such container: ` + id + `"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		running := "false"
		if state == "running" {
			running = "true"
		}
		_, _ = w.Write([]byte(`{"Id":"` + id + `","Name":"/` + id + `","State":{"Status":"` + state + `","Running":` + running + `}}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return socketPath
}

// A desktop stopped without a destroy — the disk-pressure emergency brake —
// stays tracked. It used to count as live forever, so the reaper never removed
// its container, zvol or workspace (observed: six desktops left Exited for ten
// days after their sessions ended). Only a container Docker confirms running,
// or one Docker cannot be asked about, may keep its session live.
func TestRunningTrackedSessions(t *testing.T) {
	socket := newUnixDockerInspectServer(t, map[string]string{
		"cid_running": "running",
		"cid_stopped": "exited",
	})
	unreachable := filepath.Join(t.TempDir(), "no-docker.sock")
	dm := &DevContainerManager{containers: map[string]*DevContainer{
		"ses_running":     {SessionID: "ses_running", ContainerID: "cid_running", DockerSocket: socket},
		"ses_stopped":     {SessionID: "ses_stopped", ContainerID: "cid_stopped", DockerSocket: socket},
		"ses_removed":     {SessionID: "ses_removed", ContainerID: "cid_removed", DockerSocket: socket},
		"ses_unreachable": {SessionID: "ses_unreachable", ContainerID: "cid_any", DockerSocket: unreachable},
	}}

	live := dm.runningTrackedSessions(context.Background())

	require.ElementsMatch(t, []string{"ses_running", "ses_unreachable"}, live)
	// A removed container is forgotten so hydra stops listing it; a stopped
	// one stays tracked so a warm restart can still find it.
	require.NotContains(t, dm.containers, "ses_removed")
	require.Contains(t, dm.containers, "ses_stopped")
}
