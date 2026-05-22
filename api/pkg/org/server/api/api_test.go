package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/config"
	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	orgapi "github.com/helixml/helix/api/pkg/org/server/api"
	"github.com/helixml/helix/api/pkg/org/store"
	orggorm "github.com/helixml/helix/api/pkg/org/store/gorm"
	"github.com/helixml/helix/api/pkg/org/streamhub"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/pubsub"
)

// newDeps builds a fresh in-memory store + config registry + hub for
// one test. The registry has no specs registered — individual tests
// add the ones they need.
func newDeps(t *testing.T) (orgapi.Deps, *store.Store, *config.Registry) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		t.Fatalf("new in-memory nats: %v", err)
	}
	hub := streamhub.New(ps)
	reg := config.New(st.Configs)

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
	reg.Register(config.Spec{
		Key:         "transport.postmark",
		Type:        config.TypeObject,
		Secrets:     []string{"token"},
		Description: "postmark creds",
	})
	h := orgapi.Handler(deps)

	rawValue := `{"token":"sekrit-XXXX","from":"ops@example.com"}`
	if err := reg.Set(context.Background(), "transport.postmark", rawValue, "w-owner"); err != nil {
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
	reg.Register(config.Spec{
		Key:         "worker.runtime",
		Type:        config.TypeString,
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
	ro, err := role.New("r-owner", "# Owner\nseed", nil, nil, time.Now().UTC())
	if err != nil {
		t.Fatalf("role.New: %v", err)
	}
	if err := st.Roles.Create(ctx, ro); err != nil {
		t.Fatalf("create role: %v", err)
	}
	pos, err := domain.NewPosition("p-root", "r-owner", nil)
	if err != nil {
		t.Fatalf("NewPosition: %v", err)
	}
	if err := st.Positions.Create(ctx, pos); err != nil {
		t.Fatalf("create position: %v", err)
	}
}

func mustCreateAIWorker(t *testing.T, st *store.Store, ctx context.Context, id, pos, identity string) {
	t.Helper()
	w, err := domain.NewAIWorker(worker.ID(id), position.ID(pos), identity)
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if err := st.Workers.Create(ctx, w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
}
