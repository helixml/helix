// Tests for the pkg/github helpers that the helix-org streams
// auto-install flow depends on:
//
//   - apiHookURLToHTMLURL converts GitHub's API hook URL to the
//     operator-facing settings page (drives the "Edit on GitHub →"
//     deep link on the stream detail page).
//   - sameEvents compares two event lists irrespective of order, so
//     UpsertWebhook only PATCHes a hook when the desired events
//     actually differ from what GitHub already has.
//   - UpsertWebhook: idempotent on a fresh repo (CREATE), idempotent
//     on a repo with a matching hook already (NO-OP, adopt as-is),
//     and PATCH when the existing hook has the wrong content_type
//     (the bug we hit in production: GitHub was delivering
//     form-encoded bodies to a JSON-only handler).
//
// We point go-github at an httptest.Server instead of api.github.com
// so the tests run offline and we can assert what requests were
// made.
package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func TestWebhookSettingsURL(t *testing.T) {
	t.Parallel()
	got := WebhookSettingsURL("helixml", "helix", 123)
	want := "https://github.com/helixml/helix/settings/hooks/123"
	if got != want {
		t.Errorf("WebhookSettingsURL = %q, want %q", got, want)
	}
}

func TestSameEvents(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{"both empty", nil, nil, true},
		{"empty vs nil", []string{}, nil, true},
		{"identical order", []string{"issues", "pull_request"}, []string{"issues", "pull_request"}, true},
		{"reversed order", []string{"pull_request", "issues"}, []string{"issues", "pull_request"}, true},
		{"different lengths", []string{"issues"}, []string{"issues", "pull_request"}, false},
		{"one different element", []string{"issues", "pull_request"}, []string{"issues", "release"}, false},
		{"wildcard equality", []string{"*"}, []string{"*"}, true},
		{"wildcard vs explicit list (different)", []string{"*"}, []string{"issues"}, false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := sameEvents(tc.a, tc.b); got != tc.want {
				t.Errorf("sameEvents(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// fakeGitHub is a tiny mock of GitHub's REST API for webhook
// CRUD. It captures every request so tests can assert what was
// sent. Each route returns canned JSON shaped like go-github's
// expected response.
type fakeGitHub struct {
	mu    sync.Mutex
	hooks []map[string]any // current state, mutated by POST/PATCH
	calls []recordedCall   // every request received
}

type recordedCall struct {
	Method string
	Path   string
	Body   map[string]any
}

func (f *fakeGitHub) record(r *http.Request, body map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedCall{Method: r.Method, Path: r.URL.Path, Body: body})
}

func (f *fakeGitHub) recordedCalls() []recordedCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]recordedCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeGitHub) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/helixml/helix/hooks", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			f.record(r, nil)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(f.hooks)
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.record(r, body)
			// Synthesize a created hook with id=999.
			created := map[string]any{
				"id":     999,
				"url":    "https://api.github.com/repos/helixml/helix/hooks/999",
				"active": true,
				"events": body["events"],
				"config": body["config"],
			}
			f.mu.Lock()
			f.hooks = append(f.hooks, created)
			f.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		default:
			http.NotFound(w, r)
		}
	})
	// EditHook hits /repos/{owner}/{repo}/hooks/{id} via PATCH.
	mux.HandleFunc("/api/v3/repos/helixml/helix/hooks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.record(r, body)
		// Apply the patch shallowly onto the matching hook by id
		// (last segment of the URL).
		parts := strings.Split(strings.TrimSuffix(r.URL.Path, "/"), "/")
		id := parts[len(parts)-1]
		f.mu.Lock()
		defer f.mu.Unlock()
		for i, h := range f.hooks {
			if hid, ok := h["id"].(int); ok && itoa(hid) == id {
				if ev, ok := body["events"]; ok {
					f.hooks[i]["events"] = ev
				}
				if cfg, ok := body["config"]; ok {
					f.hooks[i]["config"] = cfg
				}
				if act, ok := body["active"]; ok {
					f.hooks[i]["active"] = act
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(f.hooks[i])
				return
			}
		}
		http.NotFound(w, r)
	})
	return mux
}

func itoa(i int) string {
	// Small helper — the test's id is always 1-3 digits.
	if i == 0 {
		return "0"
	}
	var out []byte
	for i > 0 {
		out = append([]byte{byte('0' + i%10)}, out...)
		i /= 10
	}
	return string(out)
}

// newTestGitHubClient builds a Client that talks to the given
// httptest.Server instead of api.github.com. go-github's
// WithEnterpriseURLs() is what we abuse — set both URLs to our
// fake server's base.
func newTestGitHubClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := NewGithubClient(ClientOptions{
		Ctx:     context.Background(),
		Token:   "test-token",
		BaseURL: srv.URL + "/",
	})
	if err != nil {
		t.Fatalf("NewGithubClient: %v", err)
	}
	return c
}

// TestUpsertWebhookCreatesWhenAbsent pins the happy path —
// fresh repo, no existing hook → POST hooks → return the new id +
// HTML URL.
func TestUpsertWebhookCreatesWhenAbsent(t *testing.T) {
	t.Parallel()
	fake := &fakeGitHub{}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c := newTestGitHubClient(t, srv)

	got, err := c.UpsertWebhook("helixml", "helix", "web",
		"https://helix.example.com/webhook", []string{"*"}, "shh")
	if err != nil {
		t.Fatalf("UpsertWebhook: %v", err)
	}
	if got.ID != 999 {
		t.Errorf("ID = %d, want 999", got.ID)
	}
	// One GET (list) + one POST (create).
	calls := fake.recordedCalls()
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(calls))
	}
	if calls[0].Method != "GET" || calls[1].Method != "POST" {
		t.Errorf("methods = %s, %s; want GET, POST", calls[0].Method, calls[1].Method)
	}
	cfg, _ := calls[1].Body["config"].(map[string]any)
	if ct, _ := cfg["content_type"].(string); ct != "application/json" {
		t.Errorf("created hook content_type = %q, want application/json", ct)
	}
}

// TestUpsertWebhookAdoptsMatchingHook pins the no-op idempotent
// path — an existing hook with the same URL AND content_type=json
// AND matching events is adopted without any PATCH.
func TestUpsertWebhookAdoptsMatchingHook(t *testing.T) {
	t.Parallel()
	fake := &fakeGitHub{
		hooks: []map[string]any{{
			"id":     42,
			"url":    "https://api.github.com/repos/helixml/helix/hooks/42",
			"active": true,
			"events": []any{"*"},
			"config": map[string]any{
				"url":          "https://helix.example.com/webhook",
				"content_type": "application/json",
			},
		}},
	}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c := newTestGitHubClient(t, srv)

	got, err := c.UpsertWebhook("helixml", "helix", "web",
		"https://helix.example.com/webhook", []string{"*"}, "shh")
	if err != nil {
		t.Fatalf("UpsertWebhook: %v", err)
	}
	if got.ID != 42 {
		t.Errorf("ID = %d, want 42 (adopt existing)", got.ID)
	}
	// Only the GET (list) — no PATCH, no POST.
	for _, call := range fake.recordedCalls() {
		if call.Method == "PATCH" || call.Method == "POST" {
			t.Errorf("unexpected %s call: %+v", call.Method, call)
		}
	}
}

// TestUpsertWebhookPatchesFormContentType pins the regression fix:
// an existing hook with form content_type gets PATCHed to JSON
// (instead of being silently adopted, which caused every delivery
// to 400 because the handler is JSON-only — well, was, before
// decodeWebhookPayload also handled form bodies).
func TestUpsertWebhookPatchesFormContentType(t *testing.T) {
	t.Parallel()
	fake := &fakeGitHub{
		hooks: []map[string]any{{
			"id":     42,
			"url":    "https://api.github.com/repos/helixml/helix/hooks/42",
			"active": true,
			"events": []any{"*"},
			"config": map[string]any{
				"url":          "https://helix.example.com/webhook",
				"content_type": "form", // wrong!
			},
		}},
	}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c := newTestGitHubClient(t, srv)

	got, err := c.UpsertWebhook("helixml", "helix", "web",
		"https://helix.example.com/webhook", []string{"*"}, "shh")
	if err != nil {
		t.Fatalf("UpsertWebhook: %v", err)
	}
	if got.ID != 42 {
		t.Errorf("ID = %d, want 42 (adopt by patch, not new create)", got.ID)
	}
	var patched bool
	for _, call := range fake.recordedCalls() {
		if call.Method != "PATCH" {
			continue
		}
		patched = true
		cfg, _ := call.Body["config"].(map[string]any)
		if ct, _ := cfg["content_type"].(string); ct != "application/json" {
			t.Errorf("PATCH content_type = %q, want application/json", ct)
		}
	}
	if !patched {
		t.Fatalf("expected a PATCH to fix content_type; calls=%+v", fake.recordedCalls())
	}
}

// TestUpsertWebhookPatchesEventsDrift pins another patch case:
// the existing hook's events list doesn't match what we asked for
// (e.g. it was set to ["issues"] but we now want ["*"]). Should
// PATCH to bring events in line.
func TestUpsertWebhookPatchesEventsDrift(t *testing.T) {
	t.Parallel()
	fake := &fakeGitHub{
		hooks: []map[string]any{{
			"id":     42,
			"url":    "https://api.github.com/repos/helixml/helix/hooks/42",
			"active": true,
			"events": []any{"issues"},
			"config": map[string]any{
				"url":          "https://helix.example.com/webhook",
				"content_type": "application/json",
			},
		}},
	}
	srv := httptest.NewServer(fake.handler())
	defer srv.Close()
	c := newTestGitHubClient(t, srv)

	_, err := c.UpsertWebhook("helixml", "helix", "web",
		"https://helix.example.com/webhook", []string{"*"}, "shh")
	if err != nil {
		t.Fatalf("UpsertWebhook: %v", err)
	}
	var patched bool
	for _, call := range fake.recordedCalls() {
		if call.Method == "PATCH" {
			patched = true
		}
	}
	if !patched {
		t.Errorf("expected a PATCH to fix events drift; calls=%+v", fake.recordedCalls())
	}
}

// TestLoadReposUsesAuthenticatedUserEndpoint pins the fix for
// "LoadRepos called /users//repos (empty username) → 404". The
// correct API is /user/repos via ListByAuthenticatedUser.
func TestLoadReposUsesAuthenticatedUserEndpoint(t *testing.T) {
	t.Parallel()
	var hits []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/user/repos", func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path+"?"+r.URL.RawQuery)
		// Return one fake repo so the loop terminates cleanly.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"full_name":"philwinder/somerepo"}]`))
	})
	mux.HandleFunc("/api/v3/users/", func(w http.ResponseWriter, r *http.Request) {
		// If we ever hit the BROKEN path (empty username =>
		// /users//repos), this fires and fails the test. That's
		// the regression we're guarding against.
		t.Errorf("unexpected request to %s — should be calling /user/repos, not /users//repos", r.URL.Path)
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := newTestGitHubClient(t, srv)

	repos, err := c.LoadRepos()
	if err != nil {
		t.Fatalf("LoadRepos: %v", err)
	}
	if len(repos) != 1 || repos[0] != "philwinder/somerepo" {
		t.Errorf("repos = %v", repos)
	}
	if len(hits) == 0 {
		t.Errorf("expected at least one /user/repos hit")
	}
	// And confirm the right sort param is sent so the dropdown
	// surfaces the operator's most-actively-pushed repos first.
	q, _ := url.ParseQuery(strings.TrimPrefix(hits[0], "/api/v3/user/repos?"))
	if q.Get("sort") != "pushed" {
		t.Errorf("sort = %q, want pushed", q.Get("sort"))
	}
}
