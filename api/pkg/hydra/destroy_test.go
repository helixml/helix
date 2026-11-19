package hydra

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsResourceID(t *testing.T) {
	for id, want := range map[string]bool{
		"ses_01abc":    true,
		"ses_":         false,
		"spt_01abc":    false,
		"ses_../etc":   false,
		"ses_a\\b":     false,
		"":             false,
		"xses_01abc":   false,
		"ses_01abc/..": false,
	} {
		assert.Equal(t, want, isResourceID(id, "ses_"), id)
	}
}

func TestDestroyDevContainerRejectsUnsafeIDs(t *testing.T) {
	dm := &DevContainerManager{containers: map[string]*DevContainer{}}

	_, err := dm.DestroyDevContainer(context.Background(), "ses_../../etc", "")
	require.ErrorContains(t, err, "invalid session id")

	_, err = dm.DestroyDevContainer(context.Background(), "ses_01abc", "spt_../x")
	require.ErrorContains(t, err, "invalid spec task id")
}

// Archiving a finished spec task destroys a desktop that is usually already
// stopped: its container is exited or gone, and hydra may have restarted and
// lost its in-memory record. Destroy must still remove everything on disk —
// above all the task's workspace — rather than bail out because there is
// nothing left to stop.
func TestDestroyDevContainerRemovesWorkspaceOfAlreadyStoppedDesktop(t *testing.T) {
	const sessionID, specTaskID = "ses_01stopped", "spt_01finished"

	for name, exitedContainer := range map[string]bool{
		"container exited, hydra lost its record": true,
		"container already removed":               false,
	} {
		t.Run(name, func(t *testing.T) {
			resetZFSState()
			zfsAvailableOnce.Do(func() {}) // a file-copy host: no zvols to destroy
			t.Cleanup(resetZFSState)

			root := t.TempDir()
			oldWorkspaces, oldRuntime, oldSessions, oldDisks := workspacesBaseDir, sessionRuntimeBaseDir, sessionsBaseDir, instanceDisksBaseDir
			workspacesBaseDir = filepath.Join(root, "workspaces")
			sessionRuntimeBaseDir = filepath.Join(root, "sessions-runtime")
			sessionsBaseDir = filepath.Join(root, "container-docker-sessions")
			instanceDisksBaseDir = filepath.Join(root, "instance-disks")
			t.Cleanup(func() {
				workspacesBaseDir, sessionRuntimeBaseDir, sessionsBaseDir, instanceDisksBaseDir = oldWorkspaces, oldRuntime, oldSessions, oldDisks
			})

			workspace := filepath.Join(workspacesBaseDir, "spec-tasks", specTaskID)
			owned := []string{
				workspace,
				filepath.Join(workspacesBaseDir, "sessions", sessionID),
				filepath.Join(sessionRuntimeBaseDir, sessionID),
				filepath.Join(sessionsBaseDir, containerDataVolumeName(sessionID, dockerGoldenCache)),
				filepath.Join(instanceDisksBaseDir, sessionID),
			}
			for _, dir := range owned {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "file"), []byte("x"), 0644))
			}
			// Another task's workspace must survive.
			other := filepath.Join(workspacesBaseDir, "spec-tasks", "spt_01other")
			require.NoError(t, os.MkdirAll(other, 0755))

			var removed []string
			socket := serveFakeDocker(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
					if exitedContainer && strings.Contains(r.URL.RawQuery, "label") {
						_, _ = w.Write([]byte(`[{"Id":"exited-1","Names":["/ubuntu-external-01stopped"],"State":"exited","Labels":{"helix.session_id":"` + sessionID + `"}}]`))
						return
					}
					_, _ = w.Write([]byte(`[]`))
				case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/containers/"):
					removed = append(removed, containerID(r.URL.Path))
					w.WriteHeader(http.StatusNoContent)
				case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/volumes/"):
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"message":"no such volume"}`))
				default:
					t.Errorf("unexpected Docker call %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			})

			// Nothing tracked: hydra has no record of the session.
			dm := &DevContainerManager{containers: map[string]*DevContainer{}, defaultDockerSocket: socket}

			_, err := dm.DestroyDevContainer(context.Background(), sessionID, specTaskID)
			require.NoError(t, err)

			for _, dir := range owned {
				_, err := os.Stat(dir)
				assert.True(t, os.IsNotExist(err), "%s must be removed", dir)
			}
			assert.DirExists(t, other)
			if exitedContainer {
				assert.Contains(t, removed, "exited-1", "an exited container left behind must be removed")
			} else {
				assert.Empty(t, removed)
			}
		})
	}
}
