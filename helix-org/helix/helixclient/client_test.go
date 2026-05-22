package helixclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
)

func newTestClient(t *testing.T, h http.Handler) Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL, APIKey: "tok"})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return c
}

func TestStartChatSendsHelixSessionChatRequest(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer tok"; got != want {
			t.Errorf("auth header: got %q want %q", got, want)
		}
		if r.URL.Path != "/api/v1/sessions/chat" {
			t.Errorf("path: %q", r.URL.Path)
		}
		var req StartChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.AgentType != "zed_external" || req.SessionRole != "job" {
			t.Errorf("unexpected req: %+v", req)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Errorf("messages: %+v", req.Messages)
		}
		if req.ExternalAgentConfig == nil {
			t.Errorf("ExternalAgentConfig is nil; expected default {}")
		}
		_ = json.NewEncoder(w).Encode(Session{ID: "ses_42", Interactions: []*Interaction{{ID: "ix1"}}})
	}))
	s, err := c.StartChat(context.Background(), StartChatRequest{
		ProjectID:   "p1",
		SessionRole: "job",
		AgentType:   "zed_external",
		Messages:    []SessionChatMessage{NewTextMessage("user", "hello")},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if s.ID != "ses_42" {
		t.Errorf("id: got %q want ses_42", s.ID)
	}
}

// TestStartChatSyntheticInteractionFromOpenAIShape verifies that the
// OpenAI-compatible /sessions/chat response shape (returned by
// helix_basic / openai-routed sessions) is normalised into a
// synthetic Interaction so callers see one shape regardless.
func TestStartChatSyntheticInteractionFromOpenAIShape(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"ses_oai","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hello back"}}]}`))
	}))
	s, err := c.StartChat(context.Background(), StartChatRequest{
		ProjectID: "p",
		AgentType: "helix_basic",
		Messages:  []SessionChatMessage{NewTextMessage("user", "hi")},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if s.ID != "ses_oai" || len(s.Interactions) != 1 || s.Interactions[0].ResponseMessage != "hello back" {
		t.Errorf("synthetic interaction: %+v", s)
	}
}

func TestApplyProject(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/projects/apply" {
			t.Errorf("expected PUT /api/v1/projects/apply, got %s %s", r.Method, r.URL.Path)
		}
		var req ProjectApplyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if req.Name != "w-eng" {
			t.Errorf("name: %q", req.Name)
		}
		if req.Spec.Agent == nil || req.Spec.Agent.Runtime != "claude_code" {
			t.Errorf("agent spec: %+v", req.Spec.Agent)
		}
		_ = json.NewEncoder(w).Encode(ProjectApplyResponse{ProjectID: "prj_x", AgentAppID: "app_x", Created: true})
	}))
	resp, err := c.ApplyProject(context.Background(), ProjectApplyRequest{
		Name: "w-eng",
		Spec: ProjectSpec{
			Description: "engineer",
			Agent: &ProjectAgentSpec{
				Name:    "engineer",
				Runtime: "claude_code",
				Model:   "claude-sonnet-4-6",
			},
		},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if resp.ProjectID != "prj_x" || resp.AgentAppID != "app_x" || !resp.Created {
		t.Errorf("resp: %+v", resp)
	}
}

func TestPutProjectSecret(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/secrets") {
			t.Errorf("path: %q", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "HELIX_ORG_URL" || body["value"] != "https://example" {
			t.Errorf("body: %+v", body)
		}
	}))
	if err := c.PutProjectSecret(context.Background(), "prj_x", "HELIX_ORG_URL", "https://example"); err != nil {
		t.Fatalf("put secret: %v", err)
	}
}

func TestDeleteProject(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.HasPrefix(r.URL.Path, "/api/v1/projects/prj_x") {
			t.Errorf("expected DELETE /api/v1/projects/prj_x, got %s %s", r.Method, r.URL.Path)
		}
	}))
	if err := c.DeleteProject(context.Background(), "prj_x"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestGetOutput(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/output") {
			t.Errorf("path: %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(Output{SessionID: "ses_x", Status: "complete", Output: "ok", DurationMs: 12})
	}))
	out, err := c.GetOutput(context.Background(), "ses_x")
	if err != nil {
		t.Fatalf("output: %v", err)
	}
	if !runtimehelix.IsTerminalOutput(out) || out.Output != "ok" {
		t.Errorf("output: %+v", out)
	}
}

func TestSubscribeUpdatesParsesEntryPatches(t *testing.T) {
	t.Parallel()
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("session_id") != "ses_w" {
			t.Errorf("session_id: %q", r.URL.Query().Get("session_id"))
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		defer func() { _ = conn.Close() }()
		_ = conn.WriteJSON(SessionUpdate{
			Type:          "interaction_patch",
			SessionID:     "ses_w",
			InteractionID: "ix1",
			EntryCount:    1,
			EntryPatches: []EntryPatch{{
				Index: 0, MessageID: "msg-a", Type: "text", Patch: "hello", PatchOffset: 0, TotalLength: 5,
			}},
		})
	}))
	defer srv.Close()
	c, err := New(Config{BaseURL: srv.URL, APIKey: "tok"})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, err := c.SubscribeUpdates(ctx, "ses_w")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	select {
	case u, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before frame")
		}
		if len(u.EntryPatches) != 1 || u.EntryPatches[0].Patch != "hello" {
			t.Errorf("frame: %+v", u)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for frame")
	}
}

func TestWhoAmI(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(UserStatus{Admin: true, User: "u-1", Slug: "phil"})
	}))
	us, err := c.WhoAmI(context.Background())
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if us.User != "u-1" || !us.Admin {
		t.Errorf("user: %+v", us)
	}
}

func TestPutFileBase64Encodes(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/contents") {
			t.Errorf("path: %q", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		decoded, err := base64.StdEncoding.DecodeString(body["content"])
		if err != nil {
			t.Fatalf("base64: %v", err)
		}
		if string(decoded) != "hello world" {
			t.Errorf("content: %q", decoded)
		}
		if body["path"] != "job/role.md" || body["branch"] != "helix-specs" {
			t.Errorf("body: %+v", body)
		}
	}))
	if err := c.PutFile(context.Background(), "r-1", PutFileRequest{Path: "job/role.md", Branch: "helix-specs", Message: "init", Content: "hello world"}); err != nil {
		t.Fatalf("putfile: %v", err)
	}
}

// TestSendSessionMessagePostsToMessagesEndpoint verifies the new
// /sessions/{id}/messages path is wired correctly: method, path,
// auth header, JSON body shape, and that the request_id /
// interaction_id come back into the typed response.
func TestSendSessionMessagePostsToMessagesEndpoint(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: %s", r.Method)
		}
		if r.URL.Path != "/api/v1/sessions/ses_42/messages" {
			t.Errorf("path: %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth: %q", got)
		}
		var body struct {
			Content      string `json:"content"`
			Interrupt    bool   `json:"interrupt,omitempty"`
			NotifyUserID string `json:"notify_user_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Content != "hello queue" || !body.Interrupt {
			t.Errorf("body: %+v", body)
		}
		_ = json.NewEncoder(w).Encode(SendMessageResponse{RequestID: "req_1", InteractionID: "ix_7"})
	}))
	resp, err := c.SendSessionMessage(context.Background(), "ses_42", "hello queue", SendMessageOptions{Interrupt: true})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.RequestID != "req_1" || resp.InteractionID != "ix_7" {
		t.Errorf("resp: %+v", resp)
	}
}

// TestValidateProviderModel covers the happy path plus each rejection
// branch: unknown provider, unknown model, disabled model, missing
// inputs. The validator is the operator-facing pre-flight that turns
// "your typo causes a confusing 422 from /zed-config when the desktop
// boots" into "your typo causes helix-org to refuse to start with a
// concrete error pointing at the bad key."
func TestValidateProviderModel(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/providers":
			_ = json.NewEncoder(w).Encode([]string{"openai", "anthropic"})
		case "/v1/models":
			provider := r.URL.Query().Get("provider")
			var models []Model
			switch provider {
			case "anthropic":
				models = []Model{
					{ID: "claude-opus-4-6", Enabled: true},
					{ID: "claude-shelved-1", Enabled: false},
				}
			case "openai":
				models = []Model{{ID: "gpt-4o-mini", Enabled: true}}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": models})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	cases := []struct {
		name        string
		provider    string
		model       string
		wantSubstr  string // empty = expect nil error
		mustContain []string
	}{
		{name: "happy", provider: "anthropic", model: "claude-opus-4-6"},
		{name: "empty provider", provider: "", model: "x", wantSubstr: "both provider and model"},
		{name: "empty model", provider: "anthropic", model: "", wantSubstr: "both provider and model"},
		{name: "unknown provider", provider: "bunker-minimax-m2.7", model: "minimax-m2.7", wantSubstr: "not configured", mustContain: []string{"bunker-minimax-m2.7", "openai"}},
		{name: "unknown model", provider: "anthropic", model: "claude-opus-9999", wantSubstr: "not found", mustContain: []string{"claude-opus-9999", "anthropic"}},
		{name: "disabled model", provider: "anthropic", model: "claude-shelved-1", wantSubstr: "disabled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateProviderModel(context.Background(), c, tc.provider, tc.model)
			if tc.wantSubstr == "" {
				if err != nil {
					t.Fatalf("expected nil error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantSubstr)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error %q does not contain %q", err, tc.wantSubstr)
			}
			for _, must := range tc.mustContain {
				if !strings.Contains(err.Error(), must) {
					t.Errorf("error %q missing required hint %q", err, must)
				}
			}
		})
	}
}

// TestCheckDesktopQuota covers the three branches: room (active < max),
// no room (active >= max), and unlimited (max == 0). The point of the
// helper is to fail fast with a clear error instead of letting
// helix-org spin up project plumbing only to bail at StartDesktop.
func TestCheckDesktopQuota(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		body       string
		wantSubstr string // empty = expect nil error
	}{
		{name: "room", body: `{"max_concurrent_desktops":2,"active_concurrent_desktops":1}`},
		{name: "exact-limit", body: `{"max_concurrent_desktops":2,"active_concurrent_desktops":2}`, wantSubstr: "2/2 active"},
		{name: "over-limit", body: `{"max_concurrent_desktops":2,"active_concurrent_desktops":3}`, wantSubstr: "3/2 active"},
		{name: "unlimited", body: `{"max_concurrent_desktops":0,"active_concurrent_desktops":99}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/config" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			err := CheckDesktopQuota(context.Background(), c)
			if tc.wantSubstr == "" {
				if err != nil {
					t.Fatalf("expected nil, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantSubstr)
			}
		})
	}
}

func TestSendSessionMessageRejectsEmptySID(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("server must not be called when sessionID is empty")
	}))
	if _, err := c.SendSessionMessage(context.Background(), "", "x", SendMessageOptions{}); err == nil {
		t.Fatal("expected error on empty sessionID")
	}
}

func TestNewRequiresFields(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := New(Config{BaseURL: "http://x"}); err == nil {
		t.Fatal("expected error")
	}
}
