package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/types"
)

func TestOrgBotGetUpdateRoundTrip(t *testing.T) {
	bot := orgapi.BotDTO{ID: "b-1", Name: "one", Content: "old", Tools: []string{"a"},
		InstanceProfile: types.BotInstanceProfile{MCPServers: []string{"chrome-devtools"}, Tools: []string{}}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/orgs/org_1/bots/b-1", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(orgapi.BotDetailDTO{Bot: bot, LegacyAppID: "app_1", ProjectID: "prj_1"})
	})
	mux.HandleFunc("PATCH /api/v1/orgs/org_1/bots/b-1", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var fields map[string]any
		_ = json.Unmarshal(raw, &fields)
		// Only the set fields go on the wire: nil pointers mean "unchanged".
		if len(fields) != 2 || fields["content"] != "new" || fields["instance_profile"] == nil {
			t.Errorf("unexpected patch body: %s", raw)
		}
		var req orgapi.UpdateBotRequest
		_ = json.Unmarshal(raw, &req)
		bot.Content = *req.Content
		bot.InstanceProfile = *req.InstanceProfile
		_ = json.NewEncoder(w).Encode(bot)
	})
	mux.HandleFunc("GET /api/v1/orgs/org_1/bots/missing", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	detail, err := c.GetOrgBot(ctx, "org_1", "b-1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Bot.Content != "old" || detail.LegacyAppID != "app_1" || detail.ProjectID != "prj_1" ||
		detail.Bot.InstanceProfile.MCPServers[0] != "chrome-devtools" {
		t.Fatalf("unexpected detail: %+v", detail)
	}
	content := "new"
	profile := types.BotInstanceProfile{SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu, MCPServers: []string{}, Tools: []string{}}
	updated, err := c.UpdateOrgBot(ctx, "org_1", "b-1", &orgapi.UpdateBotRequest{Content: &content, InstanceProfile: &profile})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Content != "new" || updated.InstanceProfile.SandboxRuntime != types.SandboxRuntimeHeadlessUbuntu {
		t.Fatalf("update not applied: %+v", updated)
	}
	if _, err := c.GetOrgBot(ctx, "org_1", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing bot: want ErrNotFound, got %v", err)
	}
}

func TestOrgBotInstancesCreateList(t *testing.T) {
	var created []string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/orgs/org_1/bots/b-1/instances", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		created = append(created, string(raw))
		var req orgapi.CreateBotInstanceRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("create body is not JSON: %q", raw) // the handler rejects an empty body
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(orgapi.BotInstanceDTO{SessionID: "ses_1", BotID: "b-1", Name: req.Name, SandboxRuntime: req.SandboxRuntime})
	})
	mux.HandleFunc("GET /api/v1/orgs/org_1/bots/b-1/instances", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode([]orgapi.BotInstanceDTO{{SessionID: "ses_1", BotID: "b-1", SandboxStatus: "running"}})
	})
	mux.HandleFunc("DELETE /api/v1/orgs/org_1/bots/b-1/instances/ses_1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()
	ctx := context.Background()

	inst, err := c.CreateOrgBotInstance(ctx, "org_1", "b-1", &orgapi.CreateBotInstanceRequest{Name: "t", SandboxRuntime: types.SandboxRuntimeHeadlessUbuntu})
	if err != nil || inst.SessionID != "ses_1" || inst.SandboxRuntime != types.SandboxRuntimeHeadlessUbuntu {
		t.Fatalf("create: %+v %v", inst, err)
	}
	if _, err := c.CreateOrgBotInstance(ctx, "org_1", "b-1", nil); err != nil {
		t.Fatalf("create with nil request: %v", err)
	}
	if len(created) != 2 || created[1] != "{}" {
		t.Fatalf("nil request should send {}: %q", created)
	}
	list, err := c.ListOrgBotInstances(ctx, "org_1", "b-1")
	if err != nil || len(list) != 1 || list[0].SandboxStatus != "running" {
		t.Fatalf("list: %+v %v", list, err)
	}
	if err := c.DeleteOrgBotInstance(ctx, "org_1", "b-1", "ses_1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestListInteractionsSendsZeroBasedPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sessions/ses_1/interactions", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page") != "0" || q.Get("per_page") != "5" || q.Get("order") != "desc" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(types.PaginatedInteractions{Interactions: []*types.Interaction{{ID: "int_2"}, {ID: "int_1"}}})
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()

	page, err := c.ListInteractions(context.Background(), "ses_1", &InteractionFilter{PerPage: 5, Order: "desc"})
	if err != nil || len(page.Interactions) != 2 || page.Interactions[0].ID != "int_2" {
		t.Fatalf("list interactions: %+v %v", page, err)
	}
}

func TestListAppLLMCallsQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agents/app_1/llm-calls", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("session") != "ses_1" || q.Get("page") != "2" || q.Get("pageSize") != "50" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(types.PaginatedLLMCalls{Calls: []*types.LLMCall{{ID: "llm_1", PromptTokens: 10}}, Page: 2, TotalPages: 2})
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()

	page, err := c.ListAppLLMCalls(context.Background(), "app_1", &LLMCallFilter{SessionID: "ses_1", Page: 2, PageSize: 50})
	if err != nil || len(page.Calls) != 1 || page.Calls[0].PromptTokens != 10 {
		t.Fatalf("llm calls: %+v %v", page, err)
	}
}

func TestMakeRequestDeadline(t *testing.T) {
	prev := defaultRequestTimeout
	defaultRequestTimeout = 100 * time.Millisecond
	defer func() { defaultRequestTimeout = prev }()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/slow", func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`"ok"`))
	})
	c, srv := newTestClient(t, mux)
	defer srv.Close()

	// No deadline on ctx: the default timeout applies.
	var out string
	if err := c.makeRequest(context.Background(), http.MethodGet, "/slow", nil, &out); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("without a caller deadline: want deadline exceeded, got %v", err)
	}
	// A longer caller deadline wins over the default.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.makeRequest(ctx, http.MethodGet, "/slow", nil, &out); err != nil || out != "ok" {
		t.Fatalf("with a longer caller deadline: out=%q err=%v", out, err)
	}
	// A shorter caller deadline still cuts the request.
	short, cancelShort := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelShort()
	if err := c.makeRequest(short, http.MethodGet, "/slow", nil, &out); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("with a shorter caller deadline: want deadline exceeded, got %v", err)
	}
}
