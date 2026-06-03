package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/streamhub"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/pubsub"
)

// newDeps builds a fresh in-memory store + config registry + hub for
// one test. The registry has no specs registered — individual tests
// add the ones they need.
func newDeps(t *testing.T) (orgapi.Deps, *store.Store, *configregistry.Registry) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		t.Fatalf("new in-memory nats: %v", err)
	}
	hub := streamhub.New(ps)
	reg := configregistry.New(st.Configs)

	deps := orgapi.Deps{
		Store:   st,
		Configs: reg,
		Hub:     hub,
		Owner:   "w-owner",
		NewID:   func() string { return "test-id" },
		Now:     func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
	}
	return deps, st, reg
}

// do drives a JSON request through the handler under test and returns
// the raw response recorder.
func do(t *testing.T, h http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		buf = bytes.NewBuffer(raw)
	} else {
		buf = &bytes.Buffer{}
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	// Inject the org scope the middleware would otherwise set so the
	// handlers don't 400 on resolveOrgID.
	req = req.WithContext(helixorgserver.WithOrgID(req.Context(), "org-test"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if rec.Body.Len() == 0 {
		t.Fatalf("response body empty, status=%d", rec.Code)
	}
	if err := json.NewDecoder(rec.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v (body=%q)", err, rec.Body.String())
	}
}

// TestGetOrgChart_EmptyStore_Returns200WithEmptyTree pins the
// empty-store contract: a fresh org has no positions, the chart
// endpoint must still respond 200 with an empty roots array (vs
// failing, returning null, or 204).
func TestGetOrgChart_EmptyStore_Returns200WithEmptyTree(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "GET", "/chart", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var chart orgapi.Chart
	decode(t, rec, &chart)
	if len(chart.Roots) != 0 {
		t.Fatalf("expected empty roots, got %+v", chart.Roots)
	}
}

// TestGetWorkers_ListsSeededWorkers seeds two AI workers and asserts
// the JSON list mirrors them. Verifies the wire shape and that
// listWorkers reads through to the underlying store.
func TestGetWorkers_ListsSeededWorkers(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	ctx := context.Background()

	seedOwnerPosition(t, st, ctx)
	mustCreateAIWorker(t, st, ctx, "w-alice", "p-root", "alice identity")
	mustCreateAIWorker(t, st, ctx, "w-bob", "p-root", "bob identity")

	rec := do(t, h, "GET", "/workers", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var workers []orgapi.WorkerDTO
	decode(t, rec, &workers)
	if len(workers) != 2 {
		t.Fatalf("expected 2 workers, got %d: %+v", len(workers), workers)
	}
	ids := map[string]string{workers[0].ID: workers[0].IdentityContent, workers[1].ID: workers[1].IdentityContent}
	if ids["w-alice"] != "alice identity" {
		t.Errorf("w-alice identity: got %q, want %q", ids["w-alice"], "alice identity")
	}
	if ids["w-bob"] != "bob identity" {
		t.Errorf("w-bob identity: got %q, want %q", ids["w-bob"], "bob identity")
	}
}

// TestGetSettings_RedactsSecretValues registers an object spec with a
// secret field and verifies GET /settings returns the redacted form.
// Anchors the SettingsResponse → registry.GetRedacted plumbing — a
// future refactor that drops redaction from the API path would leak
// the value to anyone who can list settings.
func TestGetSettings_RedactsSecretValues(t *testing.T) {
	deps, _, reg := newDeps(t)
	reg.Register(configregistry.Spec{
		Key:         "transport.postmark",
		Type:        configregistry.TypeObject,
		Secrets:     []string{"token"},
		Description: "postmark creds",
	})
	h := orgapi.Handler(deps)

	rawValue := `{"token":"sekrit-XXXX","from":"ops@example.com"}`
	if err := reg.Set(context.Background(), "org-test", "transport.postmark", rawValue, orgchart.WorkerID("w-owner")); err != nil {
		t.Fatalf("set value: %v", err)
	}

	rec := do(t, h, "GET", "/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var resp orgapi.SettingsResponse
	decode(t, rec, &resp)
	if len(resp.Specs) != 1 {
		t.Fatalf("expected 1 spec, got %d: %+v", len(resp.Specs), resp.Specs)
	}
	got := resp.Specs[0]
	if got.Key != "transport.postmark" {
		t.Errorf("key: got %q, want %q", got.Key, "transport.postmark")
	}
	if !got.Configured {
		t.Error("expected Configured=true after Set")
	}
	if strings.Contains(got.Value, "sekrit-XXXX") {
		t.Errorf("secret leaked into Value: %q", got.Value)
	}
	if !strings.Contains(got.Value, "...") {
		t.Errorf("expected redaction marker in Value, got %q", got.Value)
	}
	// The non-secret field must still be present so the UI can render
	// the row meaningfully.
	if !strings.Contains(got.Value, "ops@example.com") {
		t.Errorf("non-secret field missing from Value: %q", got.Value)
	}
}

// TestPutSetting_PersistsValue PUTs a setting, GETs it back, asserts
// round-trip. Anchors the registry.Set → store → GetRedacted path.
func TestPutSetting_PersistsValue(t *testing.T) {
	deps, _, reg := newDeps(t)
	reg.Register(configregistry.Spec{
		Key:         "worker.runtime",
		Type:        configregistry.TypeString,
		Default:     `"claude_code"`,
		Description: "runtime",
	})
	h := orgapi.Handler(deps)

	rec := do(t, h, "PUT", "/settings/worker.runtime", orgapi.SetSettingRequest{Value: `"zed_agent"`})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT status: got %d, want 204; body=%s", rec.Code, rec.Body)
	}

	rec = do(t, h, "GET", "/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var resp orgapi.SettingsResponse
	decode(t, rec, &resp)
	if len(resp.Specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(resp.Specs))
	}
	row := resp.Specs[0]
	if !row.Configured {
		t.Error("Configured should be true after PUT")
	}
	if row.Value != `"zed_agent"` {
		t.Errorf("value: got %q, want %q", row.Value, `"zed_agent"`)
	}
}

// TestPostWorkerRole_UpdatesRoleAssignment seeds a position+role+
// worker, POSTs new content to /workers/{id}/role, then GETs the
// worker and asserts the role markdown updated.
func TestPostWorkerRole_UpdatesRoleAssignment(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	ctx := context.Background()

	seedOwnerPosition(t, st, ctx)
	mustCreateAIWorker(t, st, ctx, "w-alice", "p-root", "alice identity")

	rec := do(t, h, "POST", "/workers/w-alice/role", orgapi.UpdateWorkerRoleRequest{Content: "# Owner v2\nupdated body"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("POST status: got %d, want 204; body=%s", rec.Code, rec.Body)
	}

	rec = do(t, h, "GET", "/workers/w-alice", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var detail orgapi.WorkerDetailDTO
	decode(t, rec, &detail)
	if detail.Role == nil {
		t.Fatalf("role missing from worker detail: %+v", detail)
	}
	if detail.Role.Content != "# Owner v2\nupdated body" {
		t.Errorf("role content: got %q, want updated", detail.Role.Content)
	}
}

// seedOwnerPosition creates the canonical r-owner / p-root pair that
// tests hire workers under. Mirrors bootstrap.Run's first two writes
// without dragging in the bootstrap package's environment-dir
// requirement.
func seedOwnerPosition(t *testing.T, st *store.Store, ctx context.Context) {
	t.Helper()
	ro, err := orgchart.NewRole("r-owner", "# Owner\nseed", nil, nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatalf("orgchart.NewRole: %v", err)
	}
	if err := st.Roles.Create(ctx, ro); err != nil {
		t.Fatalf("create role: %v", err)
	}
	pos, err := orgchart.NewPosition("p-root", "r-owner", nil, "org-test")
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	if err := st.Positions.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}
}

func mustCreateAIWorker(t *testing.T, st *store.Store, ctx context.Context, id, pos, identity string) {
	t.Helper()
	w, err := orgchart.NewAIWorker(orgchart.WorkerID(id), orgchart.PositionID(pos), identity, "org-test")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if err := st.Workers.Create(ctx, w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
}

// TestPostGitHubWebhook_RoutesToInboundHandler pins the regression
// behind "GitHub streams created but never receive anything": the
// streams API accepts kind=github and the inbound transport handler
// exists in infrastructure/transports/github, but the route was
// never wired into the org-scoped API mux. POSTing a properly-signed
// GitHub delivery to /github/webhook used to 404; tail the API logs
// and you'd see nothing — the user thought their webhook was
// misconfigured when in fact helix was deaf to it.
//
// Set up: configure transport.github with a webhook_secret + token,
// seed a github-kind Stream for repo "owner/name" listening for
// `issues`, sign a body with that secret, POST it. Expect 204 (the
// transport's success status for "event appended"), not 404.
func TestPostGitHubWebhook_RoutesToInboundHandler(t *testing.T) {
	deps, st, reg := newDeps(t)
	// transport.github must be registered before set, otherwise the
	// registry rejects the key. Spec lives in api/pkg/server but the
	// shape we need here is the same — register a permissive object
	// spec locally to keep this test isolated from the helix-org
	// wiring.
	reg.Register(configregistry.Spec{
		Key:         "transport.github",
		Type:        configregistry.TypeObject,
		Secrets:     []string{"token", "webhook_secret"},
		Description: "test",
	})
	ctx := context.Background()
	const (
		webhookSecret = "test-secret"
		token         = "test-token"
		repo          = "octocat/hello-world"
	)
	rawCfg, _ := json.Marshal(map[string]any{"token": token, "webhook_secret": webhookSecret})
	if err := reg.Set(ctx, "org-test", "transport.github", string(rawCfg), orgchart.WorkerID("")); err != nil {
		t.Fatalf("set transport.github: %v", err)
	}
	streamCfg, _ := json.Marshal(map[string]any{"repo": repo, "events": []string{"issues"}})
	stream, err := streaming.NewStream(
		streaming.StreamID("s-gh-issues"), "issues", "",
		"w-owner", time.Now().UTC(),
		transport.Transport{Kind: transport.KindGitHub, Config: streamCfg},
		"org-test",
	)
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(ctx, stream); err != nil {
		t.Fatalf("seed stream: %v", err)
	}

	h := orgapi.Handler(deps)
	body, _ := json.Marshal(map[string]any{
		"action":     "opened",
		"repository": map[string]any{"full_name": repo},
		"issue":      map[string]any{"number": 1, "title": "hi"},
		"sender":     map[string]any{"login": "octocat"},
	})
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest("POST", "/github/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "del-1")
	req.Header.Set("X-Hub-Signature-256", sig)
	req = req.WithContext(helixorgserver.WithOrgID(req.Context(), "org-test"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatalf("status: got 404 — github webhook route not mounted on org-scoped mux")
	}
	// Any 2xx is acceptable: the transport returns 204 on success,
	// 200 on no-op (no streams). We're only asserting the route
	// dispatches — semantic correctness of the transport itself is
	// covered by its own tests.
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status: got %d, want 2xx; body=%s", rec.Code, rec.Body)
	}
}
