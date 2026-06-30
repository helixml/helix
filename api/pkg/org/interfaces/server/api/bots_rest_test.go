package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// mcpRegistry builds a tools registry over a fresh store with the given
// deterministic clock/id, for driving MCP tools in parity tests.
func mcpRegistry(t *testing.T, st *store.Store, clock func() time.Time, newID func() string) *mcptools.Registry {
	t.Helper()
	deps := mcptools.DefaultDeps(st)
	deps.Now = clock
	deps.NewID = newID
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

// TestRESTCreateBot_EmptyToolsGetsBaseline pins the bug fix discovered
// during the in-browser demo of helixml/helix#2546: the chart UI's "New
// Bot" dialog only collects ID + content (no tools picker) and posts to
// POST /bots with an empty tools list. The REST handler unions
// BaseReadTools the same way the MCP create_bot tool does, so the
// resulting Bot still has a usable MCP surface.
func TestRESTCreateBot_EmptyToolsGetsBaseline(t *testing.T) {
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

	bot, err := st.Bots.Get(context.Background(), "org-test", "b-qa-engineer")
	if err != nil {
		t.Fatalf("get created bot: %v", err)
	}
	got := make(map[tool.Name]bool, len(bot.Tools))
	for _, name := range bot.Tools {
		got[name] = true
	}
	for _, name := range mcptools.BaseReadTools {
		if !got[name] {
			t.Errorf("baseline tool %q missing from REST-created bot; got: %v", name, bot.Tools)
		}
	}
}

// TestRESTCreateBot_UnionWithCallerTools pins the union semantics for the
// REST path — caller-supplied tools are preserved alongside the baseline,
// deduped.
func TestRESTCreateBot_UnionWithCallerTools(t *testing.T) {
	deps, st, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/bots", orgapi.CreateBotRequest{
		ID:      "b-mixed",
		Content: "# Mixed",
		Tools:   []string{"publish", "managers", "subscribe"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want 201; body=%s", rec.Code, rec.Body)
	}

	bot, err := st.Bots.Get(context.Background(), "org-test", "b-mixed")
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
	for _, name := range []tool.Name{"publish", "subscribe"} {
		if !got[name] {
			t.Errorf("caller tool %q missing; got: %v", name, bot.Tools)
		}
	}
	for _, name := range mcptools.BaseReadTools {
		if !got[name] {
			t.Errorf("baseline tool %q missing from union; got: %v", name, bot.Tools)
		}
	}
}

// TestCreateBotParity_RESTvsMCP: the REST POST /bots handler and the MCP
// create_bot tool both go through lifecycle.Create, so both must produce
// identical bot rows — same content, same baseline-unioned tools, same
// topics.
func TestCreateBotParity_RESTvsMCP(t *testing.T) {
	clock := func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }
	newID := func() string { return "fixed" }

	restDeps, restStore, _ := newDepsClock(t, clock, newID)
	h := orgapi.Handler(restDeps)
	rec := do(t, h, "POST", "/bots", orgapi.CreateBotRequest{
		ID:      "b-qa",
		Content: "# QA",
		Tools:   []string{"publish", "subscribe"},
		Topics:  []string{"s-a"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("REST create bot: %d body=%s", rec.Code, rec.Body)
	}

	mcpStore := orggorm.GetOrgTestDB(t)
	reg := mcpRegistry(t, mcpStore, clock, newID)
	createBot, _ := reg.Get(mcptools.CreateBotName)
	args, _ := json.Marshal(map[string]any{
		"id":      "b-qa",
		"content": "# QA",
		"tools":   []string{"publish", "subscribe"},
		"topics":  []string{"s-a"},
	})
	if _, err := createBot.Invoke(context.Background(), tool.Invocation{Caller: ownerCaller(t), Args: args}); err != nil {
		t.Fatalf("MCP create_bot: %v", err)
	}

	restBot, err := restStore.Bots.Get(context.Background(), "org-test", "b-qa")
	if err != nil {
		t.Fatalf("REST bot get: %v", err)
	}
	mcpBot, err := mcpStore.Bots.Get(context.Background(), "org-test", "b-qa")
	if err != nil {
		t.Fatalf("MCP bot get: %v", err)
	}
	if restBot.Content != mcpBot.Content {
		t.Errorf("Content differs: REST=%q MCP=%q", restBot.Content, mcpBot.Content)
	}
	if !sameNames(restBot.Tools, mcpBot.Tools) {
		t.Errorf("Tools differ: REST=%v MCP=%v", restBot.Tools, mcpBot.Tools)
	}
	if len(restBot.Topics) != len(mcpBot.Topics) || (len(restBot.Topics) > 0 && restBot.Topics[0] != mcpBot.Topics[0]) {
		t.Errorf("Topics differ: REST=%v MCP=%v", restBot.Topics, mcpBot.Topics)
	}
}

// TestUpdateBotParity_RESTvsMCP: REST PATCH /bots/{id} and MCP update_bot
// both go through the bots service — both leave the bot's content in the
// same state, preserving tools on a content-only patch.
func TestUpdateBotParity_RESTvsMCP(t *testing.T) {
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
	updateBot, _ := reg.Get(mcptools.UpdateBotName)
	args, _ := json.Marshal(map[string]any{"id": "b-eng", "content": "rewritten content"})
	if _, err := updateBot.Invoke(ctx, tool.Invocation{Caller: ownerCaller(t), Args: args}); err != nil {
		t.Fatalf("MCP update_bot: %v", err)
	}

	restBot, _ := restStore.Bots.Get(ctx, "org-test", "b-eng")
	mcpBot, _ := mcpStore.Bots.Get(ctx, "org-test", "b-eng")
	if restBot.Content != mcpBot.Content {
		t.Errorf("content differs: REST=%q MCP=%q", restBot.Content, mcpBot.Content)
	}
	if restBot.Content != "rewritten content" {
		t.Errorf("content not applied: %q", restBot.Content)
	}
}
