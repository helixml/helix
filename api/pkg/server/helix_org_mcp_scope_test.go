package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/types"
)

type mcpScopeAuditRecorder struct{ entries chan orgaudit.Entry }

func (r *mcpScopeAuditRecorder) Record(_ context.Context, entry orgaudit.Entry) error {
	r.entries <- entry
	return nil
}

// newMCPScopeOrgHandlers builds the real embedded helix-org handlers over an
// in-memory store holding a chief-of-staff bot in both the caller's org
// (org_acme) and a victim org (org_victim).
func newMCPScopeOrgHandlers(t *testing.T) (*helixOrgHandlers, *mcpScopeAuditRecorder) {
	t.Helper()
	st := orgmemory.New()
	reg := mcptools.NewRegistry()
	if err := reg.Register(mcptools.Ping{}); err != nil {
		t.Fatalf("register ping: %v", err)
	}
	recorder := &mcpScopeAuditRecorder{entries: make(chan orgaudit.Entry, 4)}
	orgServer := helixorgserver.NewFromStore(st, reg, nil, nil, nil).WithAudit(
		recorder,
		func(context.Context, string, string) (string, error) { return "project-1", nil },
	)
	for _, orgID := range []string{"org_acme", "org_victim"} {
		bot, err := orgchart.NewNode("chief-of-staff", "# CoS", []tool.Name{mcptools.PingName}, time.Now().UTC(), orgID)
		if err != nil {
			t.Fatalf("new bot: %v", err)
		}
		if err := st.Nodes.Create(context.Background(), bot); err != nil {
			t.Fatalf("seed bot: %v", err)
		}
	}
	return &helixOrgHandlers{
		api:       orgServer.Handler(),
		botMCP:    orgServer.BotMCPHandler(),
		mcpServer: orgServer,
		store:     st,
		scope:     &helixOrgScope{bootstrapped: map[string]bool{"org_acme": true, "org_victim": true}},
	}, recorder
}

const mcpInitializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`

// TestHelixOrgNestedOrgMCPPathIsRejected pins the fix for the cross-org MCP
// bypass: the authenticated org-scope mount must never forward a nested
// /orgs/{B}/workers/{bot}/mcp path to a per-bot MCP server, whether B is
// another org or the caller's own.
func TestHelixOrgNestedOrgMCPPathIsRejected(t *testing.T) {
	handlers, _ := newMCPScopeOrgHandlers(t)
	server := &HelixAPIServer{Store: &helixOrgRouteTestStore{role: types.OrganizationRoleMember}}
	root := mux.NewRouter()
	server.registerHelixOrgAuthenticatedRoutes(root.PathPrefix(APIPrefix).Subrouter(), handlers)

	paths := []string{
		"/api/v1/orgs/acme/orgs/org_victim/workers/chief-of-staff/mcp",
		"/api/v1/orgs/acme/orgs/org_acme/workers/chief-of-staff/mcp",
		"/api/v1/orgs/acme/workers/chief-of-staff/mcp",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(mcpInitializeBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("X-Helix-Org-User-Id", "user-spoofed")
			req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "user-1"}))
			rec := httptest.NewRecorder()
			root.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "helix-org") {
				t.Fatalf("response leaked MCP server: %s", rec.Body.String())
			}
		})
	}
}

func TestStripOrgScopedPrefixRejectsNestedOrgPaths(t *testing.T) {
	reached := false
	h := stripOrgScopedPrefix(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, path := range []string{
		"/api/v1/orgs/acme/orgs/org_victim/workers/b/mcp",
		"/api/v1/orgs/acme//orgs/org_victim/workers/b/mcp",
		"/api/v1/orgs/acme/./orgs/org_victim",
		"/api/v1/orgs/acme/orgs",
		"/api/v1/orgs/acmeorgs/x",
	} {
		reached = false
		req := mux.SetURLVars(httptest.NewRequest(http.MethodGet, path, nil), map[string]string{"org": "acme"})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound || reached {
			t.Fatalf("%s: status = %d reached = %v, want 404 and not forwarded", path, rec.Code, reached)
		}
	}
	req := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/v1/orgs/acme/bots/orgs", nil), map[string]string{"org": "acme"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("legit path status = %d, want 204", rec.Code)
	}
}

// TestHelixOrgMCPBackendServesSessionBotAsAuthenticatedUser drives the
// legitimate gateway path end-to-end through the real MCP server: the bot
// and org come from the session, and the audit row's user is the
// authenticated principal even when the client sends a spoofed
// X-Helix-Org-User-Id header.
func TestHelixOrgMCPBackendServesSessionBotAsAuthenticatedUser(t *testing.T) {
	handlers, recorder := newMCPScopeOrgHandlers(t)
	user := &types.User{ID: "user-1", TokenType: types.TokenTypeAPIKey, SessionID: "ses-worker", OrganizationID: "org_acme"}
	server := &HelixAPIServer{Store: &helixOrgRouteTestStore{session: &types.Session{
		ID: "ses-worker", Owner: "user-1", OrganizationID: "org_acme",
		Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff"},
	}}}
	backend := NewHelixOrgMCPBackend(server, handlers)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backend.ServeHTTP(w, mux.SetURLVars(r, map[string]string{"path": ""}), user)
	}))
	t.Cleanup(httpServer.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             httpServer.URL + "/api/v1/mcp/helix-org",
		DisableStandaloneSSE: true,
		HTTPClient:           &http.Client{Transport: headerTransport{"X-Helix-Org-User-Id": "user-spoofed"}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: mcptools.PingName, Arguments: map[string]string{"message": "hi"},
	}); err != nil {
		t.Fatalf("call ping: %v", err)
	}
	select {
	case entry := <-recorder.entries:
		if entry.OrganizationID != "org_acme" || entry.ActorID != "chief-of-staff" || entry.UserID != "user-1" {
			t.Fatalf("audit entry = org %q actor %q user %q, want org_acme/chief-of-staff/user-1", entry.OrganizationID, entry.ActorID, entry.UserID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for MCP audit entry")
	}
}

type headerTransport map[string]string

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h {
		r.Header.Set(k, v)
	}
	return http.DefaultTransport.RoundTrip(r)
}
