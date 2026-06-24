package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/application/topics"
	"github.com/helixml/helix/api/pkg/org/application/subscriptions"
	"github.com/helixml/helix/api/pkg/org/application/workers"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	githubtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/github"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/pubsub"
)

// newDeps builds a fresh in-memory store + config registry + hub for
// one test, with all application services constructed over them (the
// Phase-D shape: the REST adapter holds services, not the store). The
// registry has no specs registered — individual tests add the ones they
// need.
func newDeps(t *testing.T) (orgapi.Deps, *store.Store, *configregistry.Registry) {
	return newDepsClock(t,
		func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
		func() string { return "test-id" },
	)
}

// newDepsClock is newDeps with an explicit clock + id-generator so
// parity tests can pin deterministic store state across both adapters.
func newDepsClock(t *testing.T, clock func() time.Time, newID func() string) (orgapi.Deps, *store.Store, *configregistry.Registry) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		t.Fatalf("new in-memory nats: %v", err)
	}
	hub := wakebus.New(ps)
	reg := configregistry.New(st.Configs)
	topo := reconcile.New(reconcile.Deps{Workers: st.Workers, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions, Now: clock})

	rolesSvc := roles.New(roles.Deps{Roles: st.Roles, Now: clock, NewID: newID, BaseTools: mcptools.BaseReadTools})

	deps := orgapi.Deps{
		Topics: topics.New(topics.Deps{Topics: st.Topics, Now: clock, NewID: newID}),
		Roles:   rolesSvc,
		Workers: workers.New(workers.Deps{
			Workers: st.Workers, Roles: rolesSvc, Lines: st.ReportingLines, Reconciler: topo,
		}),
		// Hire + Fire live on the lifecycle service. EnvsDir/Now/NewID
		// power Hire; Owner guards Fire. Helix/Mirror stay nil — the REST
		// tests don't exercise the Helix-side teardown.
		Lifecycle: &lifecycle.Service{
			Store: st, Reconciler: topo,
			Now: clock, NewID: newID,
		},
		Subscriptions: subscriptions.New(subscriptions.Deps{Subscriptions: st.Subscriptions, Topics: st.Topics, Workers: st.Workers, Now: clock}),
		Publishing:    publishing.New(publishing.Deps{Topics: st.Topics, Events: st.Events, Hub: hub, Now: clock, NewID: newID}),
		Queries:       queries.New(queries.Deps{Roles: st.Roles, Workers: st.Workers, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions, Events: st.Events, Activations: st.Activations}),
		Activations:   activations.New(activations.Deps{Repo: st.Activations, Now: clock, NewID: newID}),
		Processors: processors.New(processors.Deps{
			Processors: st.Processors,
			Topics:     topics.New(topics.Deps{Topics: st.Topics, Now: clock, NewID: newID}),
			Now:        clock, NewID: newID,
		}),
		Configs: reg,
		Hub:     hub,
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

// TestGetOrgOverview_EmptyStore_Returns200WithEmptyGroups pins the
// empty-store contract: a fresh org has no roles, the overview
// endpoint must still respond 200 with empty arrays.
func TestGetOrgOverview_EmptyStore_Returns200WithEmptyGroups(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "GET", "/overview", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var overview orgapi.OrgOverview
	decode(t, rec, &overview)
	if len(overview.Groups) != 0 {
		t.Fatalf("expected empty groups, got %+v", overview.Groups)
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
	mustCreateAIWorker(t, st, ctx, "w-alice", "r-owner", "alice identity")
	mustCreateAIWorker(t, st, ctx, "w-bob", "r-owner", "bob identity")

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
	if err := reg.Set(context.Background(), "org-test", "transport.postmark", rawValue); err != nil {
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
	mustCreateAIWorker(t, st, ctx, "w-alice", "r-owner", "alice identity")

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

// TestWorkerParents_AddRemoveCycleMulti pins the chart's reporting-line
// endpoints: adding a manager persists, a Worker can hold multiple
// managers, removing one drops just that line, an unknown manager 404s,
// and an edge that would close a reporting loop is rejected with 409
// (the DAG cycle guard).
func TestWorkerParents_AddRemoveCycleMulti(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	ctx := context.Background()

	seedOwnerPosition(t, st, ctx)
	mustCreateAIWorker(t, st, ctx, "w-owner", "r-owner", "owner identity")
	mustCreateAIWorker(t, st, ctx, "w-alice", "r-owner", "alice identity")
	mustCreateAIWorker(t, st, ctx, "w-bob", "r-owner", "bob identity")

	parentsOf := func(id string) []string {
		rec := do(t, h, "GET", "/workers/"+id, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status: got %d, want 200; body=%s", id, rec.Code, rec.Body)
		}
		var detail orgapi.WorkerDetailDTO
		decode(t, rec, &detail)
		return detail.Worker.ParentIDs
	}
	hasParent := func(id, parent string) bool {
		for _, p := range parentsOf(id) {
			if p == parent {
				return true
			}
		}
		return false
	}

	// Add: w-alice reports to w-owner.
	rec := do(t, h, "POST", "/workers/w-alice/parents", orgapi.AddWorkerParentRequest{ParentID: "w-owner"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("add parent status: got %d, want 204; body=%s", rec.Code, rec.Body)
	}
	if !hasParent("w-alice", "w-owner") {
		t.Fatalf("w-alice parents: got %v, want to include w-owner", parentsOf("w-alice"))
	}

	// Multi-manager: w-alice also reports to w-bob.
	rec = do(t, h, "POST", "/workers/w-alice/parents", orgapi.AddWorkerParentRequest{ParentID: "w-bob"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("add second parent status: got %d, want 204; body=%s", rec.Code, rec.Body)
	}
	if got := parentsOf("w-alice"); len(got) != 2 {
		t.Fatalf("w-alice parents after second add: got %v, want 2", got)
	}

	// Cycle guard: w-bob → w-alice would close w-alice→...→w-bob→w-alice
	// (w-bob is already a manager of w-alice).
	rec = do(t, h, "POST", "/workers/w-bob/parents", orgapi.AddWorkerParentRequest{ParentID: "w-alice"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("cycle status: got %d, want 409; body=%s", rec.Code, rec.Body)
	}

	// Unknown manager → 404.
	rec = do(t, h, "POST", "/workers/w-alice/parents", orgapi.AddWorkerParentRequest{ParentID: "w-ghost"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown manager status: got %d, want 404; body=%s", rec.Code, rec.Body)
	}

	// Remove: drop just the w-owner line; w-bob remains.
	rec = do(t, h, "DELETE", "/workers/w-alice/parents/w-owner", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("remove parent status: got %d, want 204; body=%s", rec.Code, rec.Body)
	}
	if hasParent("w-alice", "w-owner") {
		t.Fatalf("w-alice still reports to w-owner after remove: %v", parentsOf("w-alice"))
	}
	if !hasParent("w-alice", "w-bob") {
		t.Fatalf("w-alice should still report to w-bob: %v", parentsOf("w-alice"))
	}

	// Removing a line that doesn't exist → 404.
	rec = do(t, h, "DELETE", "/workers/w-alice/parents/w-owner", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("remove missing line status: got %d, want 404; body=%s", rec.Code, rec.Body)
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
}

func mustCreateAIWorker(t *testing.T, st *store.Store, ctx context.Context, id, role, identity string) {
	t.Helper()
	w, err := orgchart.NewAIWorker(orgchart.WorkerID(id), orgchart.RoleID(role), identity, "org-test")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if err := st.Workers.Create(ctx, w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
}

// TestPostGitHubWebhook_RoutesToInboundHandler pins the regression
// behind "GitHub topics created but never receive anything": the
// topics API accepts kind=github and the inbound transport handler
// exists in infrastructure/transports/github, but the route was
// never wired into the org-scoped API mux. POSTing a properly-signed
// GitHub delivery to /github/webhook used to 404; tail the API logs
// and you'd see nothing — the user thought their webhook was
// misconfigured when in fact helix was deaf to it.
//
// Set up: configure transport.github with a webhook_secret + token,
// seed a github-kind Topic for repo "owner/name" listening for
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
	if err := reg.Set(ctx, "org-test", "transport.github", string(rawCfg)); err != nil {
		t.Fatalf("set transport.github: %v", err)
	}
	topicCfg, _ := json.Marshal(map[string]any{"repo": repo, "events": []string{"issues"}})
	topic, err := streaming.NewTopic(
		streaming.TopicID("s-gh-issues"), "issues", "",
		"w-owner", time.Now().UTC(),
		transport.Transport{Kind: transport.KindGitHub, Config: topicCfg},
		"org-test",
	)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("seed topic: %v", err)
	}

	// Wire the inbound github handler the way the composition root does:
	// a per-org transport built over the store. (In production this is
	// constructed in helix_org.go; the api adapter only serves it.)
	deps.GitHubInbound = func(orgID string) http.Handler {
		return githubtransport.New(orgID, reg, st, nil, nil, slog.Default()).HandleInbound()
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
	// 200 on no-op (no topics). We're only asserting the route
	// dispatches — semantic correctness of the transport itself is
	// covered by its own tests.
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status: got %d, want 2xx; body=%s", rec.Code, rec.Body)
	}
}

// TestGetTopic_IncludesRecentEvents pins the contract the topic
// detail page depends on: GET /topics/{id} carries a `recent_events`
// array of the most recent events on that topic, newest first.
// Without this the per-topic "messages flowing through" view would
// have nothing to render on first paint and would have to wait for an
// SSE frame before showing anything.
func TestGetTopic_IncludesRecentEvents(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()

	cfg, _ := json.Marshal(map[string]any{})
	topic, err := streaming.NewTopic(
		streaming.TopicID("s-newsfeed"), "newsfeed", "",
		"w-owner", time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		transport.Transport{Kind: transport.KindLocal, Config: cfg},
		"org-test",
	)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}

	// Append two events; the API must surface both in recent_events.
	for i, body := range []string{
		`{"from":"w-owner","subject":"first","body":"hello world"}`,
		`{"from":"w-alice","subject":"second","body":"reply"}`,
	} {
		ev, err := streaming.NewEvent(
			streaming.EventID(fmt.Sprintf("e-%d", i)),
			streaming.TopicID("s-newsfeed"),
			"w-owner",
			body,
			time.Date(2026, 5, 22, 12, i, 0, 0, time.UTC),
			"org-test",
		)
		if err != nil {
			t.Fatalf("new event %d: %v", i, err)
		}
		if err := st.Events.Append(ctx, ev); err != nil {
			t.Fatalf("append event %d: %v", i, err)
		}
	}

	h := orgapi.Handler(deps)
	rec := do(t, h, "GET", "/topics/s-newsfeed", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var got orgapi.TopicDTO
	decode(t, rec, &got)
	if got.ID != "s-newsfeed" {
		t.Errorf("id: got %q, want s-newsfeed", got.ID)
	}
	if len(got.RecentEvents) != 2 {
		t.Fatalf("recent_events: got %d, want 2: %+v", len(got.RecentEvents), got.RecentEvents)
	}
	// Newest-first ordering: e-1 should land before e-0.
	if got.RecentEvents[0].ID != "e-1" {
		t.Errorf("recent_events[0].id = %q, want e-1 (newest first)", got.RecentEvents[0].ID)
	}
	if got.RecentEvents[0].Subject != "second" {
		t.Errorf("recent_events[0].subject = %q, want \"second\"", got.RecentEvents[0].Subject)
	}
	if got.RecentEvents[0].From != "w-alice" {
		t.Errorf("recent_events[0].from = %q, want w-alice", got.RecentEvents[0].From)
	}
	if got.RecentEvents[1].ID != "e-0" {
		t.Errorf("recent_events[1].id = %q, want e-0", got.RecentEvents[1].ID)
	}
}

// seedGithubTopic is a per-file helper for the
// EffectivePublicURL tests — creates a github-transport topic
// inline so the table tests don't duplicate the verbose
// streaming.NewTopic + Topics.Create dance.
func seedGithubTopic(t *testing.T, st *store.Store, id, repo string) {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{"repo": repo, "events": []string{"*"}})
	s, err := streaming.NewTopic(
		streaming.TopicID(id), id, "",
		"w-owner", time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		transport.Transport{Kind: transport.KindGitHub, Config: cfg}, "org-test",
	)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(context.Background(), s); err != nil {
		t.Fatalf("create topic: %v", err)
	}
}

// TestGetTopic_EffectivePublicURL_UsesServerURL pins that the field is
// SERVER_URL (via Deps.PublicServerURL). The detail page's loopback
// warning evaluates this field.
func TestGetTopic_EffectivePublicURL_UsesServerURL(t *testing.T) {
	deps, st, _ := newDeps(t)
	deps.PublicServerURL = "https://server-url-env.example.com"

	seedGithubTopic(t, st, "s-fallback", "helixml/helix")

	rec := do(t, orgapi.Handler(deps), "GET", "/topics/s-fallback", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d", rec.Code)
	}
	var got orgapi.TopicDTO
	decode(t, rec, &got)
	if got.EffectivePublicURL != "https://server-url-env.example.com" {
		t.Errorf("EffectivePublicURL = %q, want SERVER_URL fallback", got.EffectivePublicURL)
	}
}

// TestGetTopic_EffectivePublicURL_OnlyForGithubTopics pins that
// the field is NOT populated for non-github topics. (Local /
// webhook / postmark topics don't need a public URL, so leaving
// the field zero avoids leaking the SERVER_URL value to UIs that
// don't show it.)
func TestGetTopic_EffectivePublicURL_OnlyForGithubTopics(t *testing.T) {
	deps, st, _ := newDeps(t)
	deps.PublicServerURL = "https://example.com"

	// Local-kind topic — EffectivePublicURL should NOT be set.
	cfg, _ := json.Marshal(map[string]any{})
	s, _ := streaming.NewTopic(
		streaming.TopicID("s-local"), "s-local", "",
		"w-owner", time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		transport.Transport{Kind: transport.KindLocal, Config: cfg}, "org-test",
	)
	if err := st.Topics.Create(context.Background(), s); err != nil {
		t.Fatalf("create: %v", err)
	}

	rec := do(t, orgapi.Handler(deps), "GET", "/topics/s-local", nil)
	var got orgapi.TopicDTO
	decode(t, rec, &got)
	if got.EffectivePublicURL != "" {
		t.Errorf("EffectivePublicURL = %q, want empty for non-github topic", got.EffectivePublicURL)
	}
}
