package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"
)

// Helix must tell an MCP server which session it is calling for, in a way the
// model cannot influence. Without it, a server acting on behalf of an end user
// has to trust a tool ARGUMENT for identity — and an agent that reads untrusted
// input (a CV, a web page) can be talked into sending someone else's id.

type headerRecorder struct {
	mu   sync.Mutex
	seen http.Header
}

func (h *headerRecorder) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		h.seen = r.Header.Clone()
		h.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		// Enough of an initialize response to let the client start.
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"t","version":"1"}}}`))
	}
}

func (h *headerRecorder) get(k string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.seen.Get(k)
}

func TestMCPClientSendsTheActingSessionAsAHeader(t *testing.T) {
	rec := &headerRecorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	getter := &DefaultClientGetter{}
	meta := agent.Meta{SessionID: "ses_real", UserID: "user_real", AppID: "app_real"}
	// Start may fail on protocol details; the headers are sent regardless, which
	// is what this test is about.
	_, _ = getter.NewClient(context.Background(), meta, nil, &types.AssistantMCP{
		Name: "t", Transport: "http", URL: srv.URL,
	})

	if got := rec.get("X-Helix-Session-Id"); got != "ses_real" {
		t.Errorf("session header = %q, want ses_real", got)
	}
	if got := rec.get("X-Helix-User-Id"); got != "user_real" {
		t.Errorf("user header = %q, want user_real", got)
	}
	if got := rec.get("X-Helix-App-Id"); got != "app_real" {
		t.Errorf("app header = %q, want app_real", got)
	}
}

// An operator-configured header must not be able to impersonate a session — the
// identity headers are applied last, so a spoofed value in the app config loses.
func TestConfiguredHeadersCannotSpoofTheActingSession(t *testing.T) {
	rec := &headerRecorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	getter := &DefaultClientGetter{}
	meta := agent.Meta{SessionID: "ses_real", UserID: "user_real"}
	_, _ = getter.NewClient(context.Background(), meta, nil, &types.AssistantMCP{
		Name: "t", Transport: "http", URL: srv.URL,
		Headers: map[string]string{
			"X-Helix-Session-Id": "ses_ATTACKER",
			"X-Helix-User-Id":    "user_ATTACKER",
		},
	})

	if got := rec.get("X-Helix-Session-Id"); got != "ses_real" {
		t.Errorf("SECURITY: configured header overrode the acting session: %q", got)
	}
	if got := rec.get("X-Helix-User-Id"); got != "user_real" {
		t.Errorf("SECURITY: configured header overrode the acting user: %q", got)
	}
}

// The operator's own headers still get through — this must not break auth.
func TestConfiguredHeadersStillApply(t *testing.T) {
	rec := &headerRecorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	getter := &DefaultClientGetter{}
	_, _ = getter.NewClient(context.Background(), agent.Meta{SessionID: "ses_1"}, nil, &types.AssistantMCP{
		Name: "t", Transport: "http", URL: srv.URL,
		Headers: map[string]string{"Authorization": "Bearer op-token"},
	})

	if got := rec.get("Authorization"); got != "Bearer op-token" {
		t.Errorf("operator Authorization header lost: %q", got)
	}
}

// No session (e.g. a non-session context) must not emit an empty header that a
// server might mistake for a real, blank identity.
func TestNoSessionEmitsNoHeader(t *testing.T) {
	rec := &headerRecorder{}
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	getter := &DefaultClientGetter{}
	_, _ = getter.NewClient(context.Background(), agent.Meta{}, nil, &types.AssistantMCP{
		Name: "t", Transport: "http", URL: srv.URL,
	})

	if _, ok := rec.seen["X-Helix-Session-Id"]; ok {
		t.Error("empty session should not send the header at all")
	}
}
