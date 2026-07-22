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
	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/application/chartlayout"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/messages"
	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/application/reconcile"
	"github.com/helixml/helix/api/pkg/org/application/subscriptions"
	"github.com/helixml/helix/api/pkg/org/application/topics"
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
	topo := reconcile.New(reconcile.Deps{Bots: st.Bots, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions, Now: clock})

	botsSvc := bots.New(bots.Deps{
		Bots: st.Bots, Lines: st.ReportingLines, Reconciler: topo,
		Now: clock, NewID: newID, BaseTools: mcptools.BaseReadTools,
	})

	deps := orgapi.Deps{
		Topics:   topics.New(topics.Deps{Topics: st.Topics, Now: clock, NewID: newID}),
		Messages: messages.New(messages.Deps{Topics: st.Topics, Events: st.Events, Notifier: hub}),
		Bots:     botsSvc,
		// Create + Delete live on the lifecycle service. Bots is required
		// for Create (row creation + base-tool union). BotReconcilers wires
		// the topology reconcile. Helix/Mirror stay nil — the REST tests
		// don't exercise the Helix-side teardown.
		Lifecycle: &lifecycle.Service{
			Store: st, Bots: botsSvc, BotReconcilers: []lifecycle.BotReconciler{topo},
			Now: clock, NewID: newID,
		},
		Subscriptions: subscriptions.New(subscriptions.Deps{Subscriptions: st.Subscriptions, Topics: st.Topics, Bots: st.Bots, Now: clock}),
		Publishing:    publishing.New(publishing.Deps{Topics: st.Topics, Events: st.Events, Hub: hub, Now: clock, NewID: newID}),
		Queries:       queries.New(queries.Deps{Bots: st.Bots, ReportingLines: st.ReportingLines, Topics: st.Topics, Subscriptions: st.Subscriptions, Events: st.Events, Activations: st.Activations}),
		Activations:   activations.New(activations.Deps{Repo: st.Activations, Now: clock, NewID: newID}),
		Processors: processors.New(processors.Deps{
			Processors: st.Processors,
			Topics:     topics.New(topics.Deps{Topics: st.Topics, Now: clock, NewID: newID}),
			Now:        clock, NewID: newID,
		}),
		ChartLayout: chartlayout.New(chartlayout.Deps{Positions: st.ChartPositions, Now: clock}),
		Configs:     reg,
		Hub:         hub,
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

// seedBot creates a Bot row directly in the store with the given id +
// content. Mirrors what a create would persist, without the lifecycle
// cascade — tests that just need a row to read/edit use this.
func seedBot(t *testing.T, st *store.Store, ctx context.Context, id, content string) {
	t.Helper()
	b, err := orgchart.NewBot(orgchart.BotID(id), content, nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatalf("NewBot %s: %v", id, err)
	}
	if err := st.Bots.Create(ctx, b); err != nil {
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
	if len(overview.Bots) != 0 {
		t.Fatalf("expected empty bots, got %+v", overview.Bots)
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
	topicCfg, _ := json.Marshal(map[string]any{"repo": repo, "events": []string{"issues"}})
	topic, err := streaming.NewTopic(
		streaming.TopicID("s-gh-issues"), "issues", "",
		"b-owner", time.Now().UTC(),
		transport.Transport{Kind: transport.KindGitHub, Config: topicCfg},
		"org-test",
	)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("seed topic: %v", err)
	}

	// Wire the inbound github handler the way the composition root does.
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
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("status: got %d, want 2xx; body=%s", rec.Code, rec.Body)
	}
}

// TestGetTopic_IncludesRecentEvents pins the contract the topic detail
// page depends on: GET /topics/{id} carries a `recent_events` array of
// the most recent events on that topic, newest first.
func TestGetTopic_IncludesRecentEvents(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()

	cfg, _ := json.Marshal(map[string]any{})
	topic, err := streaming.NewTopic(
		streaming.TopicID("s-newsfeed"), "newsfeed", "",
		"b-owner", time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		transport.Transport{Kind: transport.KindLocal, Config: cfg},
		"org-test",
	)
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	if err := st.Topics.Create(ctx, topic); err != nil {
		t.Fatalf("create topic: %v", err)
	}

	for i, body := range []string{
		`{"from":"b-owner","subject":"first","body":"hello world"}`,
		`{"from":"b-alice","subject":"second","body":"reply"}`,
	} {
		ev, err := streaming.NewEvent(
			streaming.EventID(fmt.Sprintf("e-%d", i)),
			streaming.TopicID("s-newsfeed"),
			"b-owner",
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
	if got.RecentEvents[0].ID != "e-1" {
		t.Errorf("recent_events[0].id = %q, want e-1 (newest first)", got.RecentEvents[0].ID)
	}
	if got.RecentEvents[0].Subject != "second" {
		t.Errorf("recent_events[0].subject = %q, want \"second\"", got.RecentEvents[0].Subject)
	}
	if got.RecentEvents[0].From != "b-alice" {
		t.Errorf("recent_events[0].from = %q, want b-alice", got.RecentEvents[0].From)
	}
	if got.RecentEvents[1].ID != "e-0" {
		t.Errorf("recent_events[1].id = %q, want e-0", got.RecentEvents[1].ID)
	}
}

func TestGetGitLabTopic_DoesNotExposeSigningToken(t *testing.T) {
	deps, st, reg := newDeps(t)
	reg.Register(configregistry.Spec{Key: "transport.gitlab", Type: configregistry.TypeObject, Secrets: []string{"signing_token"}})
	if err := reg.Set(context.Background(), "org-test", "transport.gitlab", `{"signing_token":"whsec_must-not-leak"}`); err != nil {
		t.Fatal(err)
	}
	config, _ := json.Marshal(map[string]any{"repo": "helixml/project", "repository_id": "repo-1", "events": []string{"Merge Request Hook"}})
	topic, err := streaming.NewTopic("s-gitlab", "gitlab", "", "b-owner", time.Now(), transport.Transport{Kind: transport.KindGitLab, Config: config}, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Topics.Create(context.Background(), topic); err != nil {
		t.Fatal(err)
	}
	rec := do(t, orgapi.Handler(deps), http.MethodGet, "/topics/s-gitlab", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "whsec_must-not-leak") || strings.Contains(rec.Body.String(), "signing_token") {
		t.Fatalf("topic response leaks signing token: %s", rec.Body.String())
	}
}

func TestPublishGitLabTopicReturnsConflict(t *testing.T) {
	deps, st, _ := newDeps(t)
	config, _ := json.Marshal(map[string]any{"repo": "helixml/project", "repository_id": "repo-1", "events": []string{"Merge Request Hook"}})
	topic, err := streaming.NewTopic("s-gitlab-publish", "gitlab publish", "", "b-owner", time.Now(), transport.Transport{Kind: transport.KindGitLab, Config: config}, "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Topics.Create(context.Background(), topic); err != nil {
		t.Fatal(err)
	}
	rec := do(t, orgapi.Handler(deps), http.MethodPost, "/topics/s-gitlab-publish/publish", map[string]any{"body": "nope"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// seedGithubTopic is a per-file helper for the EffectivePublicURL
// tests — creates a github-transport topic inline.
func seedGithubTopic(t *testing.T, st *store.Store, id, repo string) {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{"repo": repo, "events": []string{"*"}})
	s, err := streaming.NewTopic(
		streaming.TopicID(id), id, "",
		"b-owner", time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
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
// SERVER_URL (via Deps.PublicServerURL).
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

// TestGetTopic_EffectivePublicURL_OnlyForInboundProviderTopics pins that the
// field is NOT populated for local topics.
func TestGetTopic_EffectivePublicURL_OnlyForInboundProviderTopics(t *testing.T) {
	deps, st, _ := newDeps(t)
	deps.PublicServerURL = "https://example.com"

	cfg, _ := json.Marshal(map[string]any{})
	s, _ := streaming.NewTopic(
		streaming.TopicID("s-local"), "s-local", "",
		"b-owner", time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
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
