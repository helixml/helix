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
	if !out.IsTerminal() || out.Output != "ok" {
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

func TestNewRequiresFields(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := New(Config{BaseURL: "http://x"}); err == nil {
		t.Fatal("expected error")
	}
}
