package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/types"
)

type failingAgentUpdater struct{}

func (failingAgentUpdater) UpdateAgent(context.Context, string, orgapi.AgentConfigPatch, *string, *string) error {
	return errors.New("agent update failed")
}

type interleavingFailingAgentUpdater struct {
	nodes store.Nodes
}

type countingNodeStore struct {
	store.Nodes
	updates int
}

func (s *countingNodeStore) Update(ctx context.Context, bot orgchart.Node) error {
	s.updates++
	return s.Nodes.Update(ctx, bot)
}

func (u interleavingFailingAgentUpdater) UpdateAgent(ctx context.Context, _ string, _ orgapi.AgentConfigPatch, _ *string, _ *string) error {
	bot, err := u.nodes.Get(ctx, "org-test", "b-agent")
	if err != nil {
		return err
	}
	if err := u.nodes.Update(ctx, bot.WithTools([]tool.Name{mcptools.ChatName})); err != nil {
		return err
	}
	return errors.New("agent update failed")
}

type recordingAgentPort struct {
	profile orgapi.AgentProfile
	patch   orgapi.AgentConfigPatch
}

type partialAgentReader struct{}

func (partialAgentReader) ReadAgent(_ context.Context, appID string) (orgapi.AgentProfile, error) {
	if appID == "app-invalid" {
		return orgapi.AgentProfile{}, fmt.Errorf("%w: linked App must contain exactly one assistant", orgapi.ErrInvalidAgentProfile)
	}
	return orgapi.AgentProfile{Name: "Canonical valid", Instructions: "Valid instructions"}, nil
}

type failingAgentReader struct{}

func (failingAgentReader) ReadAgent(context.Context, string) (orgapi.AgentProfile, error) {
	return orgapi.AgentProfile{}, errors.New("database unavailable")
}

func (p *recordingAgentPort) ReadAgent(context.Context, string) (orgapi.AgentProfile, error) {
	return p.profile, nil
}

func (p *recordingAgentPort) UpdateAgent(_ context.Context, _ string, patch orgapi.AgentConfigPatch, name, instructions *string) error {
	p.patch = patch
	if name != nil {
		p.profile.Name = *name
	}
	if instructions != nil {
		p.profile.Instructions = *instructions
	}
	if patch.CodeAgentRuntime != nil {
		p.profile.CodeAgentRuntime = *patch.CodeAgentRuntime
	}
	if patch.Model != nil {
		p.profile.Model = *patch.Model
	}
	return nil
}

func TestRESTBotResourceUsesBotDetail(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-agent", "stale", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	bot = bot.WithAgentID("app-agent")
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}
	port := &recordingAgentPort{profile: orgapi.AgentProfile{
		Name:             "Canonical",
		Instructions:     "Canonical instructions",
		CodeAgentRuntime: types.CodeAgentRuntimeCodexCLI,
		Model:            "gpt-5",
	}}
	deps.AgentReader = port
	deps.AgentUpdater = port
	handler := orgapi.Handler(deps)

	rec := do(t, handler, http.MethodGet, "/bots/b-agent", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d; body=%s", rec.Code, rec.Body)
	}
	var got orgapi.BotDetailDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Bot.Name != "Canonical" || got.Bot.Content != "Canonical instructions" ||
		got.Bot.CodeAgentRuntime != types.CodeAgentRuntimeCodexCLI || got.Bot.Model != "gpt-5" {
		t.Fatalf("bot detail = %#v", got)
	}

	runtime := types.CodeAgentRuntimeClaudeCode
	model := "opus"
	rec = do(t, handler, http.MethodPatch, "/bots/b-agent", orgapi.UpdateBotRequest{
		CodeAgentRuntime: &runtime,
		Model:            &model,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d; body=%s", rec.Code, rec.Body)
	}
	if port.patch.CodeAgentRuntime == nil || *port.patch.CodeAgentRuntime != runtime ||
		port.patch.Model == nil || *port.patch.Model != model {
		t.Fatalf("flat patch = %#v", port.patch)
	}
}

func TestRESTBotListKeepsOtherBotsWhenOneLinkedAppIsInvalid(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	for _, fixture := range []struct {
		id, appID, content string
	}{
		{id: "b-invalid", appID: "app-invalid", content: "Bot fallback"},
		{id: "b-valid", appID: "app-valid", content: "Stale"},
	} {
		bot, err := orgchart.NewNode(orgchart.NodeID(fixture.id), fixture.content, nil, time.Now().UTC(), "org-test")
		if err != nil {
			t.Fatal(err)
		}
		if err := st.Nodes.Create(ctx, bot.WithAgentID(fixture.appID)); err != nil {
			t.Fatal(err)
		}
	}
	deps.AgentReader = partialAgentReader{}

	rec := do(t, orgapi.Handler(deps), http.MethodGet, "/bots", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d; body=%s", rec.Code, rec.Body)
	}
	var got []orgapi.BotDTO
	decode(t, rec, &got)
	if len(got) != 2 {
		t.Fatalf("agents = %+v", got)
	}
	byID := map[string]orgapi.BotDTO{got[0].ID: got[0], got[1].ID: got[1]}
	if byID["b-invalid"].Content != "Bot fallback" {
		t.Fatalf("invalid fallback = %+v", byID["b-invalid"])
	}
	if byID["b-valid"].Name != "Canonical valid" || byID["b-valid"].Content != "Valid instructions" {
		t.Fatalf("valid canonical profile = %+v", byID["b-valid"])
	}
}

func TestRESTBotListFallsBackOnOperationalAppReadFailure(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-agent", "Fallback", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, bot.WithAgentID("app-agent")); err != nil {
		t.Fatal(err)
	}
	deps.AgentReader = failingAgentReader{}

	rec := do(t, orgapi.Handler(deps), http.MethodGet, "/bots", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var got []orgapi.BotDTO
	decode(t, rec, &got)
	if len(got) != 1 || got[0].ID != "b-agent" || got[0].Content != "Fallback" {
		t.Fatalf("Bot-owned fallback = %+v", got)
	}
}

// An unreadable legacy App must not blank out the execution config: the
// Bot owns it, so list and detail both have to serve the Bot's own
// CodeAgentConfig rather than empty runtime/model fields.
func TestRESTBotServesBotOwnedExecutionConfigWhenAppReadFails(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-agent", "Fallback", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	bot = bot.WithAgentID("app-agent").WithCodeAgentConfig(&types.CodeAgentExecutionConfig{
		Runtime:         types.CodeAgentRuntimeClaudeCode,
		CredentialType:  types.CodeAgentCredentialTypeAPIKey,
		ProviderRef:     "anthropic",
		Model:           "claude-opus-5",
		ReasoningEffort: "high",
	})
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}
	deps.AgentReader = failingAgentReader{}
	handler := orgapi.Handler(deps)

	assertConfig := func(t *testing.T, what string, dto orgapi.BotDTO) {
		t.Helper()
		if dto.CodeAgentRuntime != types.CodeAgentRuntimeClaudeCode ||
			dto.CodeAgentCredentialType != types.CodeAgentCredentialTypeAPIKey ||
			dto.Provider != "anthropic" || dto.Model != "claude-opus-5" || dto.ReasoningEffort != "high" {
			t.Fatalf("%s Bot-owned execution config = %+v", what, dto)
		}
	}

	rec := do(t, handler, http.MethodGet, "/bots", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var list []orgapi.BotDTO
	decode(t, rec, &list)
	if len(list) != 1 {
		t.Fatalf("bots = %+v", list)
	}
	assertConfig(t, "list", list[0])

	// getBot used to 500 on the same failure the list tolerated.
	rec = do(t, handler, http.MethodGet, "/bots/b-agent", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var detail orgapi.BotDetailDTO
	decode(t, rec, &detail)
	assertConfig(t, "detail", detail.Bot)
}

func TestRESTUpdateAgentRollbackPreservesConcurrentBotMutation(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-agent", "old content", []tool.Name{mcptools.ManagersName}, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	bot = bot.WithAgentID("app-agent").WithName("Old name")
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}
	deps.AgentUpdater = interleavingFailingAgentUpdater{nodes: st.Nodes}
	name := "New name"

	rec := do(t, orgapi.Handler(deps), http.MethodPatch, "/bots/b-agent", orgapi.UpdateBotRequest{Name: &name})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body)
	}
	got, err := st.Nodes.Get(ctx, "org-test", "b-agent")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Old name" {
		t.Fatalf("name = %q, want rolled back value", got.Name)
	}
	if !sameNames(got.Tools, []tool.Name{mcptools.ChatName}) {
		t.Fatalf("concurrent tools mutation was overwritten: %v", got.Tools)
	}
}

func TestRESTUpdateBotConfigFailureDoesNotWriteBot(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-agent", "content", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, bot.WithAgentID("app-agent")); err != nil {
		t.Fatal(err)
	}
	countingStore := &countingNodeStore{Nodes: st.Nodes}
	deps.Nodes = nodes.New(nodes.Deps{Nodes: countingStore})
	deps.AgentUpdater = failingAgentUpdater{}
	model := "new-model"

	rec := do(t, orgapi.Handler(deps), http.MethodPatch, "/bots/b-agent", orgapi.UpdateBotRequest{Model: &model})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body)
	}
	if countingStore.updates != 0 {
		t.Fatalf("Bot store updates = %d, want 0", countingStore.updates)
	}
}

func injectMCPPublishing(cfg *mcptools.Config) {
	deps := publishing.Deps{
		Triggers: cfg.Store.Triggers,
		Events:   cfg.Store.Events,
		Now:      cfg.Now,
		NewID:    cfg.NewID,
	}
	if cfg.Dispatcher != nil {
		deps.Router = cfg.Dispatcher
	}
	if cfg.Hub != nil {
		deps.Hub = cfg.Hub
	}
	cfg.Publishing = publishing.New(deps)
}

func TestRESTUpdateAgentRollsBackOrgProfile(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-agent", "old content", []tool.Name{mcptools.ManagersName}, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	bot = bot.WithAgentID("app-agent").
		WithName("Old name").
		WithProjectIDs([]string{"prj-old"}).
		WithPreserveContext(true)
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}
	deps.AgentUpdater = failingAgentUpdater{}
	name := "New name"
	content := "new content"
	preserve := false
	rec := do(t, orgapi.Handler(deps), http.MethodPatch, "/bots/b-agent", orgapi.UpdateBotRequest{
		Name:            &name,
		Content:         &content,
		Tools:           []string{mcptools.ChatName},
		ProjectIDs:      []string{"prj-new"},
		PreserveContext: &preserve,
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body)
	}
	got, err := st.Nodes.Get(ctx, "org-test", "b-agent")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != bot.Name || got.Content != bot.Content || got.PreserveContext != bot.PreserveContext {
		t.Fatalf("profile was not rolled back: %#v", got)
	}
	if !sameNames(got.Tools, bot.Tools) || len(got.ProjectIDs) != 1 || got.ProjectIDs[0] != "prj-old" {
		t.Fatalf("profile collections were not rolled back: %#v", got)
	}
}

func TestRESTUpdateLinkedAgentRequiresUpdaterBeforeMutation(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-agent", "old content", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	bot = bot.WithAgentID("app-agent").WithName("Old name")
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}
	name := "New name"
	content := "new content"

	rec := do(t, orgapi.Handler(deps), http.MethodPatch, "/bots/b-agent", orgapi.UpdateBotRequest{
		Name:    &name,
		Content: &content,
	})

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body)
	}
	got, err := st.Nodes.Get(ctx, "org-test", "b-agent")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != bot.Name || got.Content != bot.Content {
		t.Fatalf("missing updater mutated bot: %#v", got)
	}
}

// mcpRegistry builds a tools registry over a fresh store with the given
// deterministic clock/id, for driving MCP tools in parity tests.
func mcpRegistry(t *testing.T, st *store.Store, clock func() time.Time, newID func() string) *mcptools.Registry {
	t.Helper()
	deps := mcptools.DefaultDeps(st)
	deps.AgentCreator = fakeAgentCreator{}
	deps.Now = clock
	deps.NewID = newID
	injectMCPPublishing(&deps)
	reg := mcptools.NewRegistry()
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	return reg
}

// ownerCaller is the tool.Caller a parity test acts as. The MCP server
// builds the equivalent adapter at the boundary.
func ownerCaller(t *testing.T) tool.Caller {
	t.Helper()
	return mcpCaller{id: "b-owner", orgID: "org-test"}
}

type mcpCaller struct{ id, orgID string }

func (c mcpCaller) ID() string             { return c.id }
func (c mcpCaller) OrganizationID() string { return c.orgID }

func sameNames(a, b []tool.Name) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRESTCreateBot_EmptyToolsGetsDefaultWorkerSet pins the default granted
// during the in-browser demo of helixml/helix#2546: the chart UI's "New
// Bot" dialog may post an empty additions list. The REST handler unions
// DefaultBotTools the same way the MCP create_bot tool does.
func TestRESTCreateBot_EmptyToolsGetsDefaultWorkerSet(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/bots", orgapi.CreateBotRequest{
		ID:      "b-qa-engineer",
		Content: "# QA Engineer",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201; body=%s", rec.Code, rec.Body)
	}
	var out orgapi.CreateBotResponse
	decode(t, rec, &out)
	if out.ID != "b-qa-engineer" {
		t.Fatalf("created id = %q, want b-qa-engineer", out.ID)
	}

	bot, err := st.Nodes.Get(context.Background(), "org-test", "b-qa-engineer")
	if err != nil {
		t.Fatalf("get created bot: %v", err)
	}
	got := make(map[tool.Name]bool, len(bot.Tools))
	for _, name := range bot.Tools {
		got[name] = true
	}
	for _, name := range mcptools.DefaultBotTools() {
		if !got[name] {
			t.Errorf("default tool %q missing from REST-created bot; got: %v", name, bot.Tools)
		}
	}
}

func TestRESTCreateBot_MemberCannotGrantManagerTools(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)

	standard := doAsRole(t, h, http.MethodPost, "/bots", orgapi.CreateBotRequest{
		ID:      "b-member-worker",
		Content: "# Worker",
	}, types.OrganizationRoleMember, false)
	if standard.Code != http.StatusCreated {
		t.Fatalf("standard create status = %d, want 201; body=%s", standard.Code, standard.Body)
	}

	for _, request := range []orgapi.CreateBotRequest{
		{ID: "b-owner", Content: "# Owner", Owner: true},
		{ID: "b-custom", Content: "# Custom", Tools: []string{string(mcptools.CreateBotName)}},
	} {
		rec := doAsRole(t, h, http.MethodPost, "/bots", request, types.OrganizationRoleMember, false)
		if rec.Code != http.StatusForbidden {
			t.Errorf("request %+v status = %d, want 403; body=%s", request, rec.Code, rec.Body)
		}
		if _, err := st.Nodes.Get(context.Background(), "org-test", orgchart.NodeID(request.ID)); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("rejected create %s left a bot row: %v", request.ID, err)
		}
	}
}

func TestRESTUpdateBot_MemberCannotGrantManagerTools(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-member-worker", "# Worker")
	h := orgapi.Handler(deps)

	updatedContent := "# Updated worker"
	contentOnly := doAsRole(t, h, http.MethodPatch, "/bots/b-member-worker", orgapi.UpdateBotRequest{
		Content: &updatedContent,
	}, types.OrganizationRoleMember, false)
	if contentOnly.Code != http.StatusOK {
		t.Fatalf("content-only update status = %d, want 200; body=%s", contentOnly.Code, contentOnly.Body)
	}

	rec := doAsRole(t, h, http.MethodPatch, "/bots/b-member-worker", orgapi.UpdateBotRequest{
		Tools: []string{string(mcptools.ChatName), string(mcptools.CreateBotName)},
	}, types.OrganizationRoleMember, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("privileged tools update status = %d, want 403; body=%s", rec.Code, rec.Body)
	}
	bot, err := st.Nodes.Get(ctx, "org-test", "b-member-worker")
	if err != nil {
		t.Fatal(err)
	}
	if bot.Content != updatedContent {
		t.Fatalf("content = %q, want prior successful update %q", bot.Content, updatedContent)
	}
	for _, name := range bot.Tools {
		if name == mcptools.CreateBotName {
			t.Fatal("rejected manager tool was persisted")
		}
	}
}

// TestRESTCreateBot_UnionWithCallerTools pins the union semantics for the
// REST path — caller-supplied tools are preserved alongside the worker set,
// deduped.
func TestRESTCreateBot_UnionWithCallerTools(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/bots", orgapi.CreateBotRequest{
		ID:      "b-mixed",
		Content: "# Mixed",
		Tools:   []string{"chat", "managers", "attach_worker"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201; body=%s", rec.Code, rec.Body)
	}

	bot, err := st.Nodes.Get(context.Background(), "org-test", "b-mixed")
	if err != nil {
		t.Fatalf("get created bot: %v", err)
	}
	// managers (also in the baseline) must appear exactly once.
	var managersCount int
	got := make(map[tool.Name]bool, len(bot.Tools))
	for _, n := range bot.Tools {
		got[n] = true
		if n == mcptools.ManagersName {
			managersCount++
		}
	}
	if managersCount != 1 {
		t.Errorf("managers should appear exactly once after dedup; got %d in %v", managersCount, bot.Tools)
	}
	for _, name := range []tool.Name{"chat", "attach_worker"} {
		if !got[name] {
			t.Errorf("caller tool %q missing; got: %v", name, bot.Tools)
		}
	}
	for _, name := range mcptools.DefaultBotTools() {
		if !got[name] {
			t.Errorf("default tool %q missing from union; got: %v", name, bot.Tools)
		}
	}
}

// TestCreateBotParity_RESTvsMCP: the REST POST /bots handler and the MCP
// create_bot tool both go through lifecycle.Create, so both must produce
// identical bot rows — same content, same default-unioned tools.
func TestCreateBotParity_RESTvsMCP(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }
	newID := func() string { return "fixed" }

	restDeps, restStore, _ := newDepsClock(t, clock, newID)
	h := orgapi.Handler(restDeps)
	rec := do(t, h, "POST", "/bots", orgapi.CreateBotRequest{
		ID:      "b-qa",
		Content: "# QA",
		Tools:   []string{"chat", "attach_worker"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("REST create bot: %d body=%s", rec.Code, rec.Body)
	}

	mcpStore := orggorm.GetOrgTestDB(t)
	reg := mcpRegistry(t, mcpStore, clock, newID)
	createBot, _ := reg.Get(mcptools.CreateBotName)
	args, _ := json.Marshal(map[string]any{
		"id":       "b-qa",
		"content":  "# QA",
		"tools":    []string{"chat", "attach_worker"},
		"triggers": []string{},
	})
	if _, err := createBot.Invoke(context.Background(), tool.Invocation{Caller: ownerCaller(t), Args: args}); err != nil {
		t.Fatalf("MCP create_bot: %v", err)
	}

	restBot, err := restStore.Nodes.Get(context.Background(), "org-test", "b-qa")
	if err != nil {
		t.Fatalf("REST bot get: %v", err)
	}
	mcpBot, err := mcpStore.Nodes.Get(context.Background(), "org-test", "b-qa")
	if err != nil {
		t.Fatalf("MCP bot get: %v", err)
	}
	if restBot.Content != mcpBot.Content {
		t.Errorf("Content differs: REST=%q MCP=%q", restBot.Content, mcpBot.Content)
	}
	if !sameNames(restBot.Tools, mcpBot.Tools) {
		t.Errorf("Tools differ: REST=%v MCP=%v", restBot.Tools, mcpBot.Tools)
	}
}

// TestSetBotContentParity_RESTvsMCP: REST PATCH /bots/{id} and MCP
// set_bot_content both go through the bots service — both leave the bot's
// content in the same state, preserving tools.
func TestSetBotContentParity_RESTvsMCP(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }
	newID := func() string { return "fixed" }
	ctx := context.Background()

	restDeps, restStore, _ := newDepsClock(t, clock, newID)
	seedBot(t, restStore, ctx, "b-eng", "# Eng v1")
	h := orgapi.Handler(restDeps)
	newContent := "rewritten content"
	rec := do(t, h, "PATCH", "/bots/b-eng", orgapi.UpdateBotRequest{Content: &newContent})
	if rec.Code != http.StatusOK {
		t.Fatalf("REST update bot: %d body=%s", rec.Code, rec.Body)
	}

	mcpStore := orggorm.GetOrgTestDB(t)
	seedBot(t, mcpStore, ctx, "b-eng", "# Eng v1")
	reg := mcpRegistry(t, mcpStore, clock, newID)
	setContent, _ := reg.Get(mcptools.SetBotContentName)
	args, _ := json.Marshal(map[string]any{"botId": "b-eng", "content": "rewritten content"})
	if _, err := setContent.Invoke(ctx, tool.Invocation{Caller: ownerCaller(t), Args: args}); err != nil {
		t.Fatalf("MCP set_bot_content: %v", err)
	}

	restBot, _ := restStore.Nodes.Get(ctx, "org-test", "b-eng")
	mcpBot, _ := mcpStore.Nodes.Get(ctx, "org-test", "b-eng")
	if restBot.Content != mcpBot.Content {
		t.Errorf("content differs: REST=%q MCP=%q", restBot.Content, mcpBot.Content)
	}
	if restBot.Content != "rewritten content" {
		t.Errorf("content not applied: %q", restBot.Content)
	}
}

// TestRESTCreateBot_RejectsUnknownTool: a tool name the registry does not
// know must fail the create rather than being merged into the baseline
// and stored. A stored dead name is silently ignored at invoke time, so
// a typo would leave the operator with a Bot missing a capability they
// believe it has.
func TestRESTCreateBot_RejectsUnknownTool(t *testing.T) {
	deps, st, _ := newDeps(t)

	rec := do(t, orgapi.Handler(deps), http.MethodPost, "/bots", orgapi.CreateBotRequest{
		ID:      "b-typo",
		Content: "# Typo",
		Tools:   []string{mcptools.ChatName, "not_a_real_tool_abc"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "not_a_real_tool_abc") {
		t.Errorf("error should name the offending tool; body=%s", rec.Body)
	}
	if _, err := st.Nodes.Get(context.Background(), "org-test", "b-typo"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("rejected create left a bot row: %v", err)
	}
}

// TestRESTUpdateBot_RejectsUnknownTool: the same guard on the patch path.
func TestRESTUpdateBot_RejectsUnknownTool(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	bot, err := orgchart.NewNode("b-eng", "# Eng", []tool.Name{mcptools.ChatName}, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, bot); err != nil {
		t.Fatal(err)
	}

	rec := do(t, orgapi.Handler(deps), http.MethodPatch, "/bots/b-eng", orgapi.UpdateBotRequest{
		Tools: []string{mcptools.ChatName, "totally_fake_tool_xyz"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "totally_fake_tool_xyz") {
		t.Errorf("error should name the offending tool; body=%s", rec.Body)
	}
	got, err := st.Nodes.Get(ctx, "org-test", "b-eng")
	if err != nil {
		t.Fatal(err)
	}
	if !sameNames(got.Tools, []tool.Name{mcptools.ChatName}) {
		t.Fatalf("rejected patch mutated the row: %v", got.Tools)
	}
}
