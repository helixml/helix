package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeSettingsFile renders a settings.json containing contextServers and
// returns its path.
func writeSettingsFile(t *testing.T, contextServers interface{}) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	settings := map[string]interface{}{}
	if contextServers != nil {
		settings["context_servers"] = contextServers
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func newReadinessDaemon() *SettingsDaemon {
	return &SettingsDaemon{httpClient: &http.Client{Timeout: 5 * time.Second}}
}

// deadOrigin returns an http:// origin nothing is listening on.
func deadOrigin(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	return url
}

func TestContextServerEndpoints(t *testing.T) {
	t.Run("dedupes origins and ignores stdio servers", func(t *testing.T) {
		path := writeSettingsFile(t, map[string]interface{}{
			// stdio: runs in-container, cannot fail the way this check catches
			"chrome-devtools": map[string]interface{}{
				"command": "/usr/bin/chrome-devtools-mcp",
				"args":    []interface{}{"--viewport", "1280x800"},
			},
			"helix-tasks": map[string]interface{}{
				"url":     "http://helix-api.internal:18080/api/v1/mcp/helix-tasks?rev=842d852b",
				"headers": map[string]interface{}{"Authorization": "Bearer hl-test"},
			},
			"kodit": map[string]interface{}{
				"url": "http://helix-api.internal:18080/api/v1/mcp/kodit?session_id=ses_1",
			},
			"partner": map[string]interface{}{"url": "https://mcp.example.com/sse"},
		})
		got, err := contextServerEndpoints(path)
		if err != nil {
			t.Fatalf("contextServerEndpoints: %v", err)
		}
		// Sorted by name; stdio chrome-devtools excluded. Each URL is kept
		// verbatim — path and query included — because that is what gets probed.
		want := []contextServerEndpoint{
			{name: "helix-tasks", url: "http://helix-api.internal:18080/api/v1/mcp/helix-tasks?rev=842d852b"},
			{name: "kodit", url: "http://helix-api.internal:18080/api/v1/mcp/kodit?session_id=ses_1"},
			{name: "partner", url: "https://mcp.example.com/sse"},
		}
		if len(got) != len(want) {
			t.Fatalf("endpoints = %+v, want %+v", got, want)
		}
		for i := range want {
			if got[i].name != want[i].name || got[i].url != want[i].url {
				t.Fatalf("endpoints = %+v, want %+v", got, want)
			}
		}
		if got[0].headers["Authorization"] != "Bearer hl-test" {
			t.Fatalf("configured headers must be carried to the probe: %+v", got[0].headers)
		}
	})

	t.Run("no context_servers key yields none", func(t *testing.T) {
		got, err := contextServerEndpoints(writeSettingsFile(t, nil))
		if err != nil {
			t.Fatalf("contextServerEndpoints: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("endpoints = %+v, want none", got)
		}
	})

	t.Run("missing settings file is an error", func(t *testing.T) {
		if _, err := contextServerEndpoints(filepath.Join(t.TempDir(), "absent.json")); err == nil {
			t.Fatal("a missing settings.json must not read as ready")
		}
	})

	t.Run("unparseable settings file is an error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := contextServerEndpoints(path); err == nil {
			t.Fatal("a corrupt settings.json must not read as ready")
		}
	})
}

// A declared-but-broken url is a config error: Zed forwards it to the agent
// regardless, and the agent's one-shot registration against it fails
// permanently. Skipping it is how an all-malformed config reported "ready".
func TestContextServerEndpointsRejectUnusableURLs(t *testing.T) {
	cases := map[string]interface{}{
		"unparseable":  "://nope",
		"empty":        "",
		"whitespace":   "   ",
		"no host":      "http://",
		"scheme-less":  "helix-api.internal:18080/api/v1/mcp/x",
		"wrong scheme": "ftp://helix-api.internal/mcp",
		"not a string": 42,
	}
	for name, badURL := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeSettingsFile(t, map[string]interface{}{
				"broken": map[string]interface{}{"url": badURL},
			})
			endpoints, err := contextServerEndpoints(path)
			if err == nil {
				t.Fatalf("endpoints = %+v, want an error for url %v", endpoints, badURL)
			}
			if !strings.Contains(err.Error(), "broken") {
				t.Fatalf("error should name the offending server: %v", err)
			}
		})
	}

	t.Run("one broken url fails even alongside a good one", func(t *testing.T) {
		path := writeSettingsFile(t, map[string]interface{}{
			"good":   map[string]interface{}{"url": "http://api:8080/api/v1/mcp/session"},
			"broken": map[string]interface{}{"url": "://nope"},
		})
		if _, err := contextServerEndpoints(path); err == nil {
			t.Fatal("a broken url must fail readiness even when another server is fine")
		}
	})
}

func TestProbeMCPEndpoints(t *testing.T) {
	t.Run("reachable origin is ready", func(t *testing.T) {
		// 404 is a fine answer: it proves the transport works, which is the
		// only thing that can break the agent's MCP registration.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		path := writeSettingsFile(t, map[string]interface{}{
			"helix-tasks": map[string]interface{}{"url": srv.URL + "/api/v1/mcp/helix-tasks"},
		})
		if err := newReadinessDaemon().probeMCPEndpoints(context.Background(), path); err != nil {
			t.Fatalf("probeMCPEndpoints: %v", err)
		}
	})

	t.Run("unreachable origin is not ready", func(t *testing.T) {
		path := writeSettingsFile(t, map[string]interface{}{
			"helix-tasks": map[string]interface{}{"url": deadOrigin(t) + "/api/v1/mcp/helix-tasks"},
		})
		if err := newReadinessDaemon().probeMCPEndpoints(context.Background(), path); err == nil {
			t.Fatal("probeMCPEndpoints succeeded against a closed port")
		}
	})

	t.Run("one dead origin fails even when another answers", func(t *testing.T) {
		live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer live.Close()

		path := writeSettingsFile(t, map[string]interface{}{
			"kodit":       map[string]interface{}{"url": live.URL + "/api/v1/mcp/kodit"},
			"helix-tasks": map[string]interface{}{"url": deadOrigin(t) + "/api/v1/mcp/helix-tasks"},
		})
		if err := newReadinessDaemon().probeMCPEndpoints(context.Background(), path); err == nil {
			t.Fatal("probeMCPEndpoints succeeded with one unreachable origin")
		}
	})

	t.Run("stdio-only agent is ready", func(t *testing.T) {
		path := writeSettingsFile(t, map[string]interface{}{
			"chrome-devtools": map[string]interface{}{"command": "/usr/bin/chrome-devtools-mcp"},
		})
		if err := newReadinessDaemon().probeMCPEndpoints(context.Background(), path); err != nil {
			t.Fatalf("probeMCPEndpoints: %v", err)
		}
	})

	t.Run("settings never written is not ready", func(t *testing.T) {
		// syncFromHelix writes settings.json only after a successful fetch, so
		// a failed initial sync leaves no file. Reporting ready would let Zed
		// start against an agent config that does not exist.
		absent := filepath.Join(t.TempDir(), "settings.json")
		if err := newReadinessDaemon().probeMCPEndpoints(context.Background(), absent); err == nil {
			t.Fatal("probeMCPEndpoints declared readiness with no settings file")
		}
	})
}

func TestWaitForMCPEndpointsRecoversWhenOriginComesBack(t *testing.T) {
	// Grab a port, hold it closed so the first probes are refused, then bind a
	// real server on the same address mid-flight. This is the transient the
	// gate exists to ride out: the API proxy is not listening yet.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}

	path := writeSettingsFile(t, map[string]interface{}{
		"helix-tasks": map[string]interface{}{"url": "http://" + addr + "/api/v1/mcp/helix-tasks"},
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

	waitErr := newReadinessDaemon().waitForMCPEndpoints(context.Background(), path, 10*time.Second, 100*time.Millisecond)
	srv := <-bound
	if srv == nil {
		t.Skip("could not rebind the probe port; another process took it")
	}
	defer srv.Close()
	if waitErr != nil {
		t.Fatalf("waitForMCPEndpoints never recovered: %v", waitErr)
	}
}

// The gate polls this before every Zed launch, so its verdict must track the
// current state of the world rather than a boot-time snapshot.
func TestMCPReadinessHandlerReflectsCurrentState(t *testing.T) {
	origSettingsPath := SettingsPath
	t.Cleanup(func() { SettingsPath = origSettingsPath })

	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer live.Close()

	d := newReadinessDaemon()
	gate := httptest.NewServer(http.HandlerFunc(d.mcpReadinessHandler))
	defer gate.Close()

	ask := func() (int, string) {
		t.Helper()
		resp, err := http.Get(gate.URL)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body := make([]byte, 2048)
		n, _ := resp.Body.Read(body)
		return resp.StatusCode, string(body[:n])
	}

	// Healthy: ready.
	SettingsPath = writeSettingsFile(t, map[string]interface{}{
		"helix-tasks": map[string]interface{}{"url": live.URL + "/api/v1/mcp/helix-tasks"},
	})
	if status, body := ask(); status != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", status, body)
	}

	// The world changes under a running container — an agent switch rewrites
	// settings.json and restartZed() relaunches Zed. The next launch must get
	// the new verdict, not the old one.
	SettingsPath = writeSettingsFile(t, map[string]interface{}{
		"helix-tasks": map[string]interface{}{"url": deadOrigin(t) + "/api/v1/mcp/helix-tasks"},
	})
	status, body := ask()
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d (%s), want 503 after the origin died", status, body)
	}
	if !strings.Contains(body, "probe ") {
		t.Fatalf("503 body must carry the reason for the gate to print: %q", body)
	}

	// And back again, so a transient failure does not latch.
	SettingsPath = writeSettingsFile(t, map[string]interface{}{
		"helix-tasks": map[string]interface{}{"url": live.URL + "/api/v1/mcp/helix-tasks"},
	})
	if status, body := ask(); status != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200 after recovery", status, body)
	}
}

// The probe must request the configured MCP URL itself. An earlier version
// asked a synthetic `origin + "/api/v1/config"`, which asserts the readiness of
// a different route: a reverse proxy can serve that while misrouting
// /api/v1/mcp/..., and a third-party MCP server need not serve it at all.
func TestProbeRequestsTheConfiguredURL(t *testing.T) {
	type seen struct {
		method string
		path   string
		query  string
		auth   string
	}
	got := make(chan seen, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")}
		// Mirrors production: HEAD on a correct MCP URL 404s, because the
		// router has no HEAD route. That must still count as reachable.
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	path := writeSettingsFile(t, map[string]interface{}{
		"helix-tasks": map[string]interface{}{
			"url":     srv.URL + "/api/v1/mcp/helix-tasks?rev=842d852b",
			"headers": map[string]interface{}{"Authorization": "Bearer hl-session-scoped"},
		},
	})
	if err := newReadinessDaemon().probeMCPEndpoints(context.Background(), path); err != nil {
		t.Fatalf("probeMCPEndpoints: %v", err)
	}

	select {
	case s := <-got:
		if s.method != http.MethodHead {
			t.Errorf("method = %s, want HEAD (POST allocates MCP session state, GET opens an SSE stream)", s.method)
		}
		if s.path != "/api/v1/mcp/helix-tasks" {
			t.Errorf("path = %q, want the configured MCP path", s.path)
		}
		if s.query != "rev=842d852b" {
			t.Errorf("query = %q, want the configured query preserved", s.query)
		}
		if s.auth != "Bearer hl-session-scoped" {
			t.Errorf("Authorization = %q, want the configured header so the probe crosses the same auth edge", s.auth)
		}
	default:
		t.Fatal("probe never reached the configured URL")
	}
}

// When the Helix API is down but the sandbox proxy is up, the proxy's
// ErrorHandler answers every MCP URL with 502 "Helix API unavailable".
// Measured, not assumed: stopping helix-api-1 turned all four context servers
// into 502s. Treating any HTTP response as success would call that ready.
func TestProbeRejectsProxyUpstreamFailure(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusInternalServerError} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "Helix API unavailable", status)
		}))
		path := writeSettingsFile(t, map[string]interface{}{
			"helix-tasks": map[string]interface{}{"url": srv.URL + "/api/v1/mcp/helix-tasks"},
		})
		err := newReadinessDaemon().probeMCPEndpoints(context.Background(), path)
		srv.Close()
		if err == nil {
			t.Fatalf("status %d must not read as ready", status)
		}
	}
}

// Status below 5xx is success on purpose. Through the sandbox proxy, HEAD on a
// correct MCP URL returns 404 while HEAD on a deliberately wrong path returns
// 200 from the frontend catch-all — status is anti-correlated with routing
// correctness, so anything stricter would reject the endpoints that work.
func TestProbeAcceptsNonServerErrorStatuses(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNoContent, http.StatusBadRequest,
		http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		path := writeSettingsFile(t, map[string]interface{}{
			"helix-tasks": map[string]interface{}{"url": srv.URL + "/api/v1/mcp/helix-tasks"},
		})
		err := newReadinessDaemon().probeMCPEndpoints(context.Background(), path)
		srv.Close()
		if err != nil {
			t.Fatalf("status %d must read as reachable: %v", status, err)
		}
	}
}

// Every configured server is probed, and the failure names the one at fault.
func TestProbeReportsEveryFailingServer(t *testing.T) {
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer live.Close()

	path := writeSettingsFile(t, map[string]interface{}{
		"kodit":         map[string]interface{}{"url": live.URL + "/api/v1/mcp/kodit"},
		"helix-tasks":   map[string]interface{}{"url": deadOrigin(t) + "/api/v1/mcp/helix-tasks"},
		"helix-session": map[string]interface{}{"url": deadOrigin(t) + "/api/v1/mcp/session"},
	})
	err := newReadinessDaemon().probeMCPEndpoints(context.Background(), path)
	if err == nil {
		t.Fatal("probeMCPEndpoints succeeded with two unreachable servers")
	}
	for _, name := range []string{"helix-tasks", "helix-session"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error must name %s so the operator knows which server failed: %v", name, err)
		}
	}
	if strings.Contains(err.Error(), "kodit") {
		t.Errorf("error should not blame the healthy server: %v", err)
	}
}
