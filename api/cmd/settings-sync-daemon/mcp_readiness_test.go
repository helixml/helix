package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoteContextServerOrigins(t *testing.T) {
	t.Run("dedupes origins and ignores stdio servers", func(t *testing.T) {
		settings := map[string]interface{}{
			"context_servers": map[string]interface{}{
				// stdio: runs in-container, cannot fail the way this check catches
				"chrome-devtools": map[string]interface{}{
					"command": "/usr/bin/chrome-devtools-mcp",
					"args":    []interface{}{"--viewport", "1280x800"},
				},
				"helix-tasks": map[string]interface{}{
					"url": "http://helix-api.internal:18080/api/v1/mcp/helix-tasks?rev=842d852b",
				},
				"kodit": map[string]interface{}{
					"url": "http://helix-api.internal:18080/api/v1/mcp/kodit?session_id=ses_1",
				},
				"partner": map[string]interface{}{
					"url": "https://mcp.example.com/sse",
				},
			},
		}
		got := remoteContextServerOrigins(settings)
		want := []string{"http://helix-api.internal:18080", "https://mcp.example.com"}
		if len(got) != len(want) {
			t.Fatalf("origins = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("origins = %v, want %v", got, want)
			}
		}
	})

	t.Run("no context servers yields none", func(t *testing.T) {
		if got := remoteContextServerOrigins(map[string]interface{}{}); len(got) != 0 {
			t.Fatalf("origins = %v, want none", got)
		}
	})

	t.Run("unparseable url is skipped, not fatal", func(t *testing.T) {
		settings := map[string]interface{}{
			"context_servers": map[string]interface{}{
				"broken": map[string]interface{}{"url": "://nope"},
				"good":   map[string]interface{}{"url": "http://api:8080/api/v1/mcp/session"},
			},
		}
		got := remoteContextServerOrigins(settings)
		if len(got) != 1 || got[0] != "http://api:8080" {
			t.Fatalf("origins = %v, want [http://api:8080]", got)
		}
	})
}

// newReadinessDaemon builds the minimal daemon the readiness check needs, with
// markers redirected into a tempdir so tests never touch /tmp state shared with
// a real container.
func newReadinessDaemon(t *testing.T, settings map[string]interface{}) (*SettingsDaemon, string, string) {
	t.Helper()
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	unreachable := filepath.Join(dir, "unreachable")

	origReady, origUnreachable := mcpEndpointsReadyMarker, mcpEndpointsUnreachableMarker
	mcpEndpointsReadyMarker, mcpEndpointsUnreachableMarker = ready, unreachable
	t.Cleanup(func() {
		mcpEndpointsReadyMarker, mcpEndpointsUnreachableMarker = origReady, origUnreachable
	})

	return &SettingsDaemon{
		helixSettings: settings,
		httpClient:    &http.Client{Timeout: 5 * time.Second},
	}, ready, unreachable
}

func TestVerifyMCPEndpoints(t *testing.T) {
	t.Run("reachable origin marks ready", func(t *testing.T) {
		// 404 is a perfectly good answer: it proves the transport works, which
		// is the only thing that can break the agent's MCP registration.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		d, ready, unreachable := newReadinessDaemon(t, map[string]interface{}{
			"context_servers": map[string]interface{}{
				"helix-tasks": map[string]interface{}{"url": srv.URL + "/api/v1/mcp/helix-tasks"},
			},
		})
		if err := d.verifyMCPEndpoints(context.Background(), 5*time.Second, 100*time.Millisecond); err != nil {
			t.Fatalf("verifyMCPEndpoints: %v", err)
		}
		if _, err := os.Stat(ready); err != nil {
			t.Fatalf("ready marker not written: %v", err)
		}
		if _, err := os.Stat(unreachable); !os.IsNotExist(err) {
			t.Fatalf("unreachable marker should not exist: %v", err)
		}
	})

	t.Run("unreachable origin fails and records why", func(t *testing.T) {
		// Bind then immediately close so the port is closed but well-formed.
		srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		deadURL := srv.URL
		srv.Close()

		d, ready, unreachable := newReadinessDaemon(t, map[string]interface{}{
			"context_servers": map[string]interface{}{
				"helix-tasks": map[string]interface{}{"url": deadURL + "/api/v1/mcp/helix-tasks"},
			},
		})
		err := d.verifyMCPEndpoints(context.Background(), 500*time.Millisecond, 100*time.Millisecond)
		if err == nil {
			t.Fatal("verifyMCPEndpoints succeeded against a closed port")
		}
		if _, statErr := os.Stat(ready); !os.IsNotExist(statErr) {
			t.Fatalf("ready marker must not be written on failure: %v", statErr)
		}
		body, readErr := os.ReadFile(unreachable)
		if readErr != nil {
			t.Fatalf("unreachable marker not written: %v", readErr)
		}
		if len(body) == 0 {
			t.Fatal("unreachable marker is empty; the operator needs the failing url")
		}
	})

	t.Run("one dead origin fails even when another answers", func(t *testing.T) {
		live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer live.Close()
		dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		deadURL := dead.URL
		dead.Close()

		d, ready, _ := newReadinessDaemon(t, map[string]interface{}{
			"context_servers": map[string]interface{}{
				"kodit":       map[string]interface{}{"url": live.URL + "/api/v1/mcp/kodit"},
				"helix-tasks": map[string]interface{}{"url": deadURL + "/api/v1/mcp/helix-tasks"},
			},
		})
		if err := d.verifyMCPEndpoints(context.Background(), 500*time.Millisecond, 100*time.Millisecond); err == nil {
			t.Fatal("verifyMCPEndpoints succeeded with one unreachable origin")
		}
		if _, err := os.Stat(ready); !os.IsNotExist(err) {
			t.Fatalf("ready marker must not be written: %v", err)
		}
	})

	t.Run("recovers when the origin comes back", func(t *testing.T) {
		// Grab a port, hold it closed so the first probes are refused, then
		// bind a real server on the same address mid-flight. This is the
		// actual transient the gate exists to ride out: the API proxy is not
		// listening yet when the container boots.
		probe, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := probe.Addr().String()
		if err := probe.Close(); err != nil {
			t.Fatal(err)
		}

		d, ready, _ := newReadinessDaemon(t, map[string]interface{}{
			"context_servers": map[string]interface{}{
				"helix-tasks": map[string]interface{}{"url": "http://" + addr + "/api/v1/mcp/helix-tasks"},
			},
		})

		bound := make(chan *httptest.Server, 1)
		go func() {
			time.Sleep(300 * time.Millisecond)
			ln, lnErr := net.Listen("tcp", addr)
			if lnErr != nil {
				bound <- nil
				return
			}
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			srv.Listener.Close()
			srv.Listener = ln
			srv.Start()
			bound <- srv
		}()

		verifyErr := d.verifyMCPEndpoints(context.Background(), 10*time.Second, 100*time.Millisecond)
		srv := <-bound
		if srv == nil {
			t.Skip("could not rebind the probe port; another process took it")
		}
		defer srv.Close()

		if verifyErr != nil {
			t.Fatalf("verifyMCPEndpoints never recovered: %v", verifyErr)
		}
		if _, statErr := os.Stat(ready); statErr != nil {
			t.Fatalf("ready marker not written after recovery: %v", statErr)
		}
	})

	t.Run("unsynced settings never mark ready", func(t *testing.T) {
		// syncFromHelix only assigns helixSettings after a successful fetch, so
		// a failed initial sync leaves it nil. Marking ready here would let Zed
		// start against an agent config that was never written.
		d, ready, unreachable := newReadinessDaemon(t, nil)
		if err := d.verifyMCPEndpoints(context.Background(), time.Second, 100*time.Millisecond); err == nil {
			t.Fatal("verifyMCPEndpoints declared readiness with no synced settings")
		}
		if _, err := os.Stat(ready); !os.IsNotExist(err) {
			t.Fatalf("ready marker must not be written: %v", err)
		}
		if _, err := os.Stat(unreachable); err != nil {
			t.Fatalf("unreachable marker not written: %v", err)
		}
	})

	t.Run("no remote context servers is ready, not an error", func(t *testing.T) {
		d, ready, _ := newReadinessDaemon(t, map[string]interface{}{
			"context_servers": map[string]interface{}{
				"chrome-devtools": map[string]interface{}{"command": "/usr/bin/chrome-devtools-mcp"},
			},
		})
		if err := d.verifyMCPEndpoints(context.Background(), time.Second, 100*time.Millisecond); err != nil {
			t.Fatalf("verifyMCPEndpoints: %v", err)
		}
		if _, err := os.Stat(ready); err != nil {
			t.Fatalf("ready marker not written: %v", err)
		}
	})

	t.Run("stale unreachable marker is cleared on success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		d, _, unreachable := newReadinessDaemon(t, map[string]interface{}{
			"context_servers": map[string]interface{}{
				"helix-tasks": map[string]interface{}{"url": srv.URL + "/api/v1/mcp/helix-tasks"},
			},
		})
		if err := os.WriteFile(unreachable, []byte("stale\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := d.verifyMCPEndpoints(context.Background(), time.Second, 100*time.Millisecond); err != nil {
			t.Fatalf("verifyMCPEndpoints: %v", err)
		}
		if _, err := os.Stat(unreachable); !os.IsNotExist(err) {
			t.Fatal("stale unreachable marker survived a successful probe")
		}
	})
}
