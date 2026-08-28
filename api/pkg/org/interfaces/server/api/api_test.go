package api_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/assets"
	"github.com/helixml/helix/api/pkg/org/application/attachments"
	"github.com/helixml/helix/api/pkg/org/application/chartlayout"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/messages"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/application/triggers"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	githubtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/github"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/pubsub"
)

// newDeps builds a fresh store + config registry + hub for one test,
// with all application services constructed over them (the Phase-D
// shape: the REST adapter holds services, not the store). The registry
// has no specs registered — individual tests add the ones they need.
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
	topo := reconcile.New(reconcile.Deps{Nodes: st.Nodes, ReportingLines: st.ReportingLines, Triggers: st.Triggers, Attachments: st.WorkerAttachments, Now: clock})

	// The MCP registry is the authority on which tool names exist. The
	// REST harness wires it exactly as the composition root does, so a
	// handler test sees the same tool-name validation production does.
	toolReg := mcpRegistry(t, st, clock, newID)
	botsSvc := nodes.New(nodes.Deps{
		Nodes: st.Nodes, Lines: st.ReportingLines, Reconciler: topo,
		Now: clock, NewID: newID, BaseTools: mcptools.BaseReadTools,
		KnownTools: mcptools.Catalogue(toolReg),
	})
	assetsSvc, err := assets.New(assets.Deps{
		Assets: st.Assets, Links: st.AssetLinks, Nodes: st.Nodes,
		GenerateKey: func() (string, string, error) { return "private-key", "ssh-ed25519 public-key", nil },
		Encrypt:     func(plaintext []byte) (string, error) { return "encrypted:" + string(plaintext), nil },
		Now:         clock, NewID: newID,
	})
	if err != nil {
		t.Fatalf("new assets service: %v", err)
	}

	deps := orgapi.Deps{
		Triggers: triggers.New(triggers.Deps{
			Triggers: st.Triggers, Attachments: st.WorkerAttachments, Events: st.Events,
			Now: clock, NewID: newID,
			Provisioners: map[transport.Kind]trigger.Inbound{},
		}),
		Messages: messages.New(messages.Deps{Triggers: st.Triggers, Events: st.Events, Notifier: hub}),
		Nodes:    botsSvc,
		// Create + Delete live on the lifecycle service. Nodes is required
		// for Create (row creation + base-tool union). NodeReconcilers wires
		// the topology reconcile. Helix/Mirror stay nil — the REST tests
		// don't exercise the Helix-side teardown.
		Lifecycle: &lifecycle.Service{
			Store: st, Nodes: botsSvc, NodeReconcilers: []lifecycle.NodeReconciler{topo},
			Now: clock, NewID: newID,
		},
		Attachments: attachments.New(attachments.Deps{Store: st, Now: clock, NewID: newID}),
		Publishing:  publishing.New(publishing.Deps{Triggers: st.Triggers, Events: st.Events, Hub: hub, Now: clock, NewID: newID}),
		Queries: queries.New(queries.Deps{
			Nodes: st.Nodes, ReportingLines: st.ReportingLines, Triggers: st.Triggers,
			Attachments: st.WorkerAttachments, Processors: st.Processors, Events: st.Events, Activations: st.Activations,
		}),
		Activations: activations.New(activations.Deps{Repo: st.Activations, Now: clock, NewID: newID}),
		Assets:      assetsSvc,
		Processors: processors.New(processors.Deps{
			Processors:  st.Processors,
			Triggers:    st.Triggers,
			Attachments: st.WorkerAttachments,
			Now:         clock, NewID: newID,
		}),
		ChartLayout: chartlayout.New(chartlayout.Deps{Positions: st.ChartPositions, Now: clock}),
		Configs:     reg,
		Hub:         hub,
		Tools:       toolReg,
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

type fakeInbound struct {
	state trigger.InboundState
}

func (f fakeInbound) Install(context.Context, string, trigger.Trigger) (trigger.InstallResult, error) {
	return trigger.InstallResult{}, nil
}

func (f fakeInbound) Status(context.Context, string, trigger.Trigger) (trigger.InboundState, error) {
	return f.state, nil
}

// seedBot creates a Bot row directly in the store with the given id +
// content. Mirrors what a create would persist, without the lifecycle
// cascade — tests that just need a row to read/edit use this.
func seedBot(t *testing.T, st *store.Store, ctx context.Context, id, content string) {
	t.Helper()
	b, err := orgchart.NewNode(orgchart.NodeID(id), content, nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatalf("NewNode %s: %v", id, err)
	}
	if err := st.Nodes.Create(ctx, b); err != nil {
		t.Fatalf("create bot %s: %v", id, err)
	}
}

// TestGetOrgOverview_EmptyStore_Returns200WithEmptyBots pins the
// empty-store contract: a fresh org has no bots, the overview endpoint
// must still respond 200 with an empty array.
func TestGetOrgOverview_EmptyStore_Returns200WithEmptyBots(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "GET", "/overview", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var overview orgapi.OrgOverview
	decode(t, rec, &overview)
	if len(overview.Nodes) != 0 {
		t.Fatalf("expected empty bots, got %+v", overview.Nodes)
	}
}

// TestGetBots_ListsSeededBots seeds two bots and asserts the JSON list
// mirrors them. Verifies the wire shape and that listBots reads through
// to the underlying store.
func TestGetBots_ListsSeededBots(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	ctx := context.Background()

	seedBot(t, st, ctx, "b-alice", "# Alice")
	seedBot(t, st, ctx, "b-bob", "# Bob")

	rec := do(t, h, "GET", "/bots", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var got []orgapi.BotDTO
	decode(t, rec, &got)
	if len(got) != 2 {
		t.Fatalf("expected 2 bots, got %d: %+v", len(got), got)
	}
	byID := map[string]string{got[0].ID: got[0].Content, got[1].ID: got[1].Content}
	if byID["b-alice"] != "# Alice" {
		t.Errorf("b-alice content: got %q, want %q", byID["b-alice"], "# Alice")
	}
	if byID["b-bob"] != "# Bob" {
		t.Errorf("b-bob content: got %q, want %q", byID["b-bob"], "# Bob")
	}
}

// TestGetSettings_RedactsSecretValues registers an object spec with a
// secret field and verifies GET /settings returns the redacted form.
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

// TestPatchBot_UpdatesContent seeds a bot, PATCHes new content, then
// GETs the bot and asserts the markdown updated.
func TestPatchBot_UpdatesContent(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	ctx := context.Background()

	seedBot(t, st, ctx, "b-alice", "# Owner\noriginal body")

	content := "# Owner v2\nupdated body"
	rec := do(t, h, "PATCH", "/bots/b-alice", orgapi.UpdateBotRequest{
		Content:    &content,
		ProjectIDs: []string{"prj_own", "prj_extra"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}

	rec = do(t, h, "GET", "/bots/b-alice", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var detail orgapi.BotDetailDTO
	decode(t, rec, &detail)
	if detail.Bot.Content != "# Owner v2\nupdated body" {
		t.Errorf("bot content: got %q, want updated", detail.Bot.Content)
	}
	if got := strings.Join(detail.Bot.ProjectIDs, ","); got != "prj_own,prj_extra" {
		t.Errorf("project ids: got %q, want prj_own,prj_extra", got)
	}
}

// TestBotParents_AddRemoveCycleMulti pins the chart's reporting-line
// endpoints: adding a manager persists, a Bot can hold multiple
// managers, removing one drops just that line, an unknown manager 404s,
// and an edge that would close a reporting loop is rejected with 409
// (the DAG cycle guard).
func TestBotParents_AddRemoveCycleMulti(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)
	ctx := context.Background()

	seedBot(t, st, ctx, "b-owner", "# owner")
	seedBot(t, st, ctx, "b-alice", "# alice")
	seedBot(t, st, ctx, "b-bob", "# bob")

	parentsOf := func(id string) []string {
		rec := do(t, h, "GET", "/bots/"+id, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status: got %d, want 200; body=%s", id, rec.Code, rec.Body)
		}
		var detail orgapi.BotDetailDTO
		decode(t, rec, &detail)
		return detail.Bot.ParentIDs
	}
	hasParent := func(id, parent string) bool {
		for _, p := range parentsOf(id) {
			if p == parent {
				return true
			}
		}
		return false
	}

	// Add: b-alice reports to b-owner.
	rec := do(t, h, "POST", "/bots/b-alice/parents", orgapi.AddBotParentRequest{ParentID: "b-owner"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("add parent status: got %d, want 204; body=%s", rec.Code, rec.Body)
	}
	if !hasParent("b-alice", "b-owner") {
		t.Fatalf("b-alice parents: got %v, want to include b-owner", parentsOf("b-alice"))
	}

	// Multi-manager: b-alice also reports to b-bob.
	rec = do(t, h, "POST", "/bots/b-alice/parents", orgapi.AddBotParentRequest{ParentID: "b-bob"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("add second parent status: got %d, want 204; body=%s", rec.Code, rec.Body)
	}
	if got := parentsOf("b-alice"); len(got) != 2 {
		t.Fatalf("b-alice parents after second add: got %v, want 2", got)
	}

	// Cycle guard: b-bob → b-alice would close b-alice→...→b-bob→b-alice
	// (b-bob is already a manager of b-alice).
	rec = do(t, h, "POST", "/bots/b-bob/parents", orgapi.AddBotParentRequest{ParentID: "b-alice"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("cycle status: got %d, want 409; body=%s", rec.Code, rec.Body)
	}

	// Unknown manager → 404.
	rec = do(t, h, "POST", "/bots/b-alice/parents", orgapi.AddBotParentRequest{ParentID: "b-ghost"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown manager status: got %d, want 404; body=%s", rec.Code, rec.Body)
	}

	// Remove: drop just the b-owner line; b-bob remains.
	rec = do(t, h, "DELETE", "/bots/b-alice/parents/b-owner", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("remove parent status: got %d, want 204; body=%s", rec.Code, rec.Body)
	}
	if hasParent("b-alice", "b-owner") {
		t.Fatalf("b-alice still reports to b-owner after remove: %v", parentsOf("b-alice"))
	}
	if !hasParent("b-alice", "b-bob") {
		t.Fatalf("b-alice should still report to b-bob: %v", parentsOf("b-alice"))
	}

	// Removing a line that doesn't exist → 404.
	rec = do(t, h, "DELETE", "/bots/b-alice/parents/b-owner", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("remove missing line status: got %d, want 404; body=%s", rec.Code, rec.Body)
	}
}

// TestPostGitHubWebhook_RoutesToInboundHandler pins the regression
// behind "GitHub topics created but never receive anything": the topics
// API accepts kind=github and the inbound transport handler exists in
// infrastructure/transports/github, but the route must be wired into the
// org-scoped API mux. POSTing a properly-signed GitHub delivery to
// /github/webhook must dispatch (any 2xx), not 404.
func TestPostGitHubWebhook_RoutesToInboundHandler(t *testing.T) {
	deps, st, reg := newDeps(t)
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
	triggerCfg, _ := json.Marshal(map[string]any{"repo": repo, "events": []string{"issues"}})
	row, err := trigger.New("s-gh-issues", "org-test", "issues", "", transport.KindGitHub, triggerCfg, "b-owner", time.Now().UTC())
	if err != nil {
		t.Fatalf("new trigger: %v", err)
	}
	if err := st.Triggers.Create(ctx, row); err != nil {
		t.Fatalf("seed trigger: %v", err)
	}

	// Wire the inbound github handler the way the composition root does.
	publisher := publishing.New(publishing.Deps{
		Triggers: st.Triggers, Events: st.Events,
		Now: func() time.Time { return time.Now().UTC() }, NewID: func() string { return "gh-1" },
	})
	deps.GitHubInbound = func(orgID string) http.Handler {
		return githubtransport.New(orgID, reg, st, publisher, slog.Default()).HandleInbound()
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
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status: got %d, want 2xx; body=%s", rec.Code, rec.Body)
	}
}

func TestGetGitHubWebhookStatus_IncludesLiveEvents(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	row, err := trigger.New("s-gh-events", "org-test", "github events", "", transport.KindGitHub,
		json.RawMessage(`{"repo":"helixml/helix","events":["issues"]}`), "b-owner", time.Now().UTC())
	if err != nil {
		t.Fatalf("new trigger: %v", err)
	}
	if err := st.Triggers.Create(ctx, row); err != nil {
		t.Fatalf("seed trigger: %v", err)
	}
	deps.Triggers = triggers.New(triggers.Deps{
		Triggers: st.Triggers,
		Provisioners: map[transport.Kind]trigger.Inbound{
			transport.KindGitHub: fakeInbound{state: trigger.InboundState{
				State: "installed", Events: []string{"pull_request"},
			}},
		},
	})

	rec := do(t, orgapi.Handler(deps), http.MethodGet, "/triggers/s-gh-events/github/webhook-status", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var got orgapi.GitHubWebhookStatusResponse
	decode(t, rec, &got)
	if len(got.Events) != 1 || got.Events[0] != "pull_request" {
		t.Fatalf("events = %v, want live [pull_request]", got.Events)
	}
}
