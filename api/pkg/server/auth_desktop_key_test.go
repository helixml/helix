package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

const sandboxKeyToken = "hl-sandbox-key"

// sandboxKeyMiddleware authenticates sandboxKeyToken as the key
// ensureSandboxAPIToken mints: owned by usr_owner, bound to sbx_A, with the
// given project scope ("" for a sandbox outside any project).
func sandboxKeyMiddleware(t *testing.T, sessionID, projectID string) *authMiddleware {
	return desktopKeyMiddleware(t, &types.ApiKey{
		OrganizationID: "org_1",
		Type:           types.APIkeytypeAPI,
		SessionID:      sessionID,
		ProjectID:      projectID,
	}, nil)
}

// desktopKeyMiddleware authenticates sandboxKeyToken as key, owned by
// usr_owner. session is what the store returns for the key's session; nil
// means it is not found.
func desktopKeyMiddleware(t *testing.T, key *types.ApiKey, session *types.Session) *authMiddleware {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	key.Key = sandboxKeyToken
	key.Owner = "usr_owner"
	key.OwnerType = types.OwnerTypeUser
	if key.AppID == nil {
		key.AppID = &sql.NullString{}
	}
	st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: sandboxKeyToken}).Return(key, nil).AnyTimes()
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_owner"}).Return(&types.User{ID: "usr_owner"}, nil).AnyTimes()
	st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: "usr_owner"}, nil).AnyTimes()
	st.EXPECT().TouchUserLastSeen(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	if session != nil {
		st.EXPECT().GetSession(gomock.Any(), key.SessionID).Return(session, nil).AnyTimes()
	} else {
		st.EXPECT().GetSession(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	}
	return newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, nil, nil)
}

// serveWithKey runs a request through the API middleware (the /api/v1 routes)
// and the per-route auth wrapper (the /v1 LLM proxy routes), returning the
// status each produced.
func serveWithKey(auth *authMiddleware, method, target string) (extract, wrapped int) {
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

	r := httptest.NewRequest(method, target, nil)
	r.Header.Set("Authorization", "Bearer "+sandboxKeyToken)
	rec := httptest.NewRecorder()
	auth.extractMiddleware(http.HandlerFunc(ok)).ServeHTTP(rec, r)
	extract = rec.Code

	r = httptest.NewRequest(method, target, nil)
	r.Header.Set("Authorization", "Bearer "+sandboxKeyToken)
	rec = httptest.NewRecorder()
	auth.auth(ok)(rec, r)
	return extract, rec.Code
}

// What code inside a sandbox outside any project needs keeps working.
func TestSandboxKeyAllowsItsOwnSandboxNeeds(t *testing.T) {
	auth := sandboxKeyMiddleware(t, "sbx_A", "")
	for _, c := range []struct{ method, target string }{
		{"POST", "/v1/chat/completions"},
		{"POST", "/v1/responses"},
		{"POST", "/v1/messages"},
		{"POST", "/v1/embeddings"},
		{"GET", "/v1/models"},
		{"OPTIONS", "/v1/messages"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/sbx_A"},
		{"POST", "/api/v1/organizations/org_1/sandboxes/sbx_A/commands"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/sbx_A/commands/cmd_1/logs"},
		{"POST", "/api/v1/organizations/org_1/sandboxes/sbx_A/commands/cmd_1/kill"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/sbx_A/files?path=/a"},
		{"PUT", "/api/v1/organizations/org_1/sandboxes/sbx_A/files?path=/a"},
		{"DELETE", "/api/v1/organizations/org_1/sandboxes/sbx_A/files?path=/a"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/sbx_A/files/list"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/sbx_A/screenshot"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/sbx_A/terminal"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/sbx_A/terminal/sessions"},
		{"GET", "/api/v1/sessions/sbx_A/zed-config"},
		{"POST", "/api/v1/sessions/sbx_A/zed-config/user"},
		{"POST", "/api/v1/sessions/sbx_A/agent-startup-error"},
		{"POST", "/api/v1/sessions/sbx_A/agent-config-applied"},
		{"GET", "/api/v1/ws/user?session_id=sbx_A"},
		{"GET", "/api/v1/revdial?runnerid=desktop-sbx_A"},
		{"GET", "/api/v1/revdial?revdial.dialer=dialer-token"},
		{"POST", "/api/v1/mcp/desktop?session_id=sbx_A"},
		{"POST", "/api/v1/mcp/visualization?session_id=sbx_A"},
	} {
		extract, wrapped := serveWithKey(auth, c.method, c.target)
		require.Equal(t, http.StatusOK, extract, "%s %s", c.method, c.target)
		require.Equal(t, http.StatusOK, wrapped, "%s %s", c.method, c.target)
	}
}

// Everything else the owner could reach is refused, including the owner's
// other sandboxes, sessions and desktops.
func TestSandboxKeyDeniesOwnerAuthority(t *testing.T) {
	auth := sandboxKeyMiddleware(t, "sbx_A", "")
	for _, c := range []struct{ method, target string }{
		{"GET", "/api/v1/projects"},
		{"GET", "/api/v1/apps"},
		{"GET", "/api/v1/secrets"},
		{"POST", "/api/v1/api_keys"},
		{"GET", "/api/v1/users/me"},
		{"GET", "/api/v1/organizations"},
		{"GET", "/api/v1/git/repositories"},
		{"GET", "/api/v1/organizations/org_1/sandboxes"},
		{"POST", "/api/v1/organizations/org_1/sandboxes"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/sbx_B"},
		{"POST", "/api/v1/organizations/org_1/sandboxes/sbx_AB/commands"},
		{"PATCH", "/api/v1/organizations/org_1/sandboxes/sbx_A"},
		{"DELETE", "/api/v1/organizations/org_1/sandboxes/sbx_A"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/sbx_A/billing"},
		{"GET", "/api/v1/organizations/org_2/sandboxes/sbx_A"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/sbx_A/filesystem"},
		{"GET", "/api/v1/sessions"},
		{"GET", "/api/v1/sessions/ses_other"},
		{"GET", "/api/v1/sessions/sbx_A"},
		{"DELETE", "/api/v1/sessions/sbx_A"},
		{"POST", "/api/v1/sessions/sbx_A/messages"},
		{"POST", "/api/v1/sessions/sbx_A/fork"},
		{"POST", "/api/v1/sessions/sbx_A/switch-agent"},
		{"POST", "/api/v1/sessions/sbx_A/zed-config"},
		{"GET", "/api/v1/ws/user?session_id=ses_other"},
		{"GET", "/api/v1/revdial?runnerid=desktop-ses_other"},
		{"POST", "/api/v1/mcp/desktop?session_id=ses_other"},
		{"POST", "/api/v1/mcp/desktop"},
		{"POST", "/api/v1/mcp/visualization"},
		{"POST", "/api/v1/mcp/session"},
		{"POST", "/api/v1/mcp/session?session_id=sbx_A"},
		{"POST", "/api/v1/mcp/helix"},
		{"POST", "/api/v1/mcp/kodit"},
		{"POST", "/api/v1/mcp/helix-org"},
		{"POST", "/api/v1/mcp/external/crm"},
		{"POST", "/v1/chat/completions?rag_source_id=rag_1"},
		{"POST", "/v1/chat/completions?assistant_id=0"},
		{"GET", "/v1/chat/completions"},
	} {
		extract, wrapped := serveWithKey(auth, c.method, c.target)
		require.Equal(t, http.StatusForbidden, extract, "%s %s", c.method, c.target)
		require.Equal(t, http.StatusForbidden, wrapped, "%s %s", c.method, c.target)
	}
}

// Route confinement is for keys outside a project: a project sandbox's key is
// confined to its project by the project-scope checks.
func TestSandboxKeyConfinementLeavesProjectKeys(t *testing.T) {
	auth := sandboxKeyMiddleware(t, "sbx_A", "prj_1")
	extract, _ := serveWithKey(auth, "GET", "/api/v1/projects")
	require.Equal(t, http.StatusOK, extract)
}

// chatSession is a plain chat session with no project, whose desktop holds a
// session key (services.GetOrCreateSessionAPIKey).
func chatSession() *types.Session {
	return &types.Session{ID: "ses_A", Owner: "usr_owner", ParentApp: "app_A"}
}

func sessionKeyMiddleware(t *testing.T, session *types.Session) *authMiddleware {
	return desktopKeyMiddleware(t, &types.ApiKey{Type: types.APIkeytypeAPI, SessionID: "ses_A"}, session)
}

// What the desktop of a session outside any project needs keeps working:
// the settings-sync daemon, Zed and its MCP servers, the desktop bridge and
// the LLM proxy.
func TestSessionKeyAllowsItsOwnDesktopNeeds(t *testing.T) {
	auth := sessionKeyMiddleware(t, chatSession())
	for _, c := range []struct{ method, target string }{
		{"POST", "/v1/chat/completions"},
		{"POST", "/v1/chat/completions?app_id=app_A"},
		{"POST", "/v1/responses"},
		{"POST", "/v1/messages"},
		{"POST", "/v1/embeddings"},
		{"GET", "/v1/models"},
		{"GET", "/api/v1/v1/models"},
		{"OPTIONS", "/v1/messages"},
		{"GET", "/api/v1/sessions/ses_A"},
		{"GET", "/api/v1/sessions/ses_A/interactions"},
		{"GET", "/api/v1/sessions/ses_A/zed-config"},
		{"POST", "/api/v1/sessions/ses_A/zed-config/user"},
		{"POST", "/api/v1/sessions/ses_A/agent-startup-error"},
		{"POST", "/api/v1/sessions/ses_A/agent-config-applied"},
		{"GET", "/api/v1/sessions/ses_A/claude-credentials"},
		{"PUT", "/api/v1/sessions/ses_A/claude-credentials"},
		{"GET", "/api/v1/sessions/ses_A/codex-credentials"},
		{"PUT", "/api/v1/sessions/ses_A/codex-credentials"},
		{"GET", "/api/v1/ws/user?session_id=ses_A"},
		{"GET", "/api/v1/external-agents/sync?session_id=ses_A"},
		{"GET", "/api/v1/revdial?runnerid=desktop-ses_A"},
		{"GET", "/api/v1/revdial?revdial.dialer=dialer-token"},
		{"POST", "/api/v1/mcp/desktop?session_id=ses_A"},
		{"OPTIONS", "/api/v1/mcp/desktop?session_id=ses_A"},
		{"POST", "/api/v1/mcp/visualization?session_id=ses_A"},
		{"POST", "/api/v1/mcp/session?session_id=ses_A"},
		{"POST", "/api/v1/mcp/external/crm"},
		{"POST", "/api/v1/mcp/external/crm/sse?session_id=ses_A"},
		{"POST", "/api/v1/mcp/helix?app_id=app_A&session_id=ses_A"},
		{"POST", "/api/v1/mcp/helix/message?app_id=app_A"},
	} {
		extract, wrapped := serveWithKey(auth, c.method, c.target)
		require.Equal(t, http.StatusOK, extract, "%s %s", c.method, c.target)
		require.Equal(t, http.StatusOK, wrapped, "%s %s", c.method, c.target)
	}
}

// Everything else its owner could reach is refused: the owner's other
// sessions, projects, repositories, secrets, keys, filestore, organizations
// and sandboxes, and the session's own chat and lifecycle routes.
func TestSessionKeyDeniesOwnerAuthority(t *testing.T) {
	auth := sessionKeyMiddleware(t, chatSession())
	for _, c := range []struct{ method, target string }{
		{"GET", "/api/v1/projects"},
		{"GET", "/api/v1/projects/prj_1"},
		{"POST", "/api/v1/projects"},
		{"GET", "/api/v1/projects/prj_1/repositories"},
		{"GET", "/api/v1/apps"},
		{"GET", "/api/v1/git/repositories"},
		{"POST", "/api/v1/git/repositories"},
		{"GET", "/api/v1/git/repositories/repo_1"},
		{"GET", "/api/v1/secrets"},
		{"POST", "/api/v1/secrets"},
		{"DELETE", "/api/v1/secrets/sec_1"},
		{"GET", "/api/v1/api_keys"},
		{"POST", "/api/v1/api_keys"},
		{"GET", "/api/v1/filestore/list?path=apps"},
		{"POST", "/api/v1/filestore/upload?path=x"},
		{"GET", "/api/v1/users/me"},
		{"GET", "/api/v1/organizations"},
		{"GET", "/api/v1/organizations/org_1/sandboxes"},
		{"POST", "/api/v1/organizations/org_1/sandboxes"},
		{"DELETE", "/api/v1/organizations/org_1/sandboxes/sbx_B"},
		{"POST", "/api/v1/organizations/org_1/sandboxes/sbx_B/commands"},
		{"GET", "/api/v1/organizations/org_1/sandboxes/ses_A"},
		{"GET", "/api/v1/spec-tasks"},
		{"GET", "/api/v1/search?q=x"},
		{"GET", "/api/v1/sessions"},
		{"GET", "/api/v1/sessions/ses_other"},
		{"GET", "/api/v1/sessions/ses_other/interactions"},
		{"GET", "/api/v1/sessions/ses_AB"},
		{"PUT", "/api/v1/sessions/ses_A"},
		{"DELETE", "/api/v1/sessions/ses_A"},
		{"POST", "/api/v1/sessions/ses_A/messages"},
		{"POST", "/api/v1/sessions/ses_A/fork"},
		{"POST", "/api/v1/sessions/ses_A/zed-config"},
		{"POST", "/api/v1/sessions/chat"},
		{"POST", "/api/v1/external-agents/ses_other/exec"},
		{"POST", "/api/v1/external-agents/ses_A/exec"},
		{"GET", "/api/v1/external-agents/sync?session_id=ses_other"},
		{"GET", "/api/v1/ws/user?session_id=ses_other"},
		{"GET", "/api/v1/revdial?runnerid=desktop-ses_other"},
		{"POST", "/api/v1/mcp/desktop?session_id=ses_other"},
		{"POST", "/api/v1/mcp/desktop"},
		{"POST", "/api/v1/mcp/session"},
		{"POST", "/api/v1/mcp/session?session_id=ses_other"},
		// A session outside any project has no repositories to search.
		{"POST", "/api/v1/mcp/kodit?session_id=ses_A"},
		{"POST", "/api/v1/mcp/kodit?session_id=ses_other"},
		{"POST", "/api/v1/mcp/external/crm?session_id=ses_other"},
		{"POST", "/api/v1/mcp/helix?app_id=app_other"},
		{"POST", "/api/v1/mcp/helix?app_id=app_A&session_id=ses_other"},
		{"POST", "/api/v1/mcp/helix-org"},
		{"POST", "/api/v1/mcp/helix-tasks"},
		{"POST", "/v1/chat/completions?app_id=app_other"},
		// RAG sources and assistants are authorized by owner only.
		{"POST", "/v1/chat/completions?rag_source_id=rag_1"},
		{"POST", "/v1/chat/completions?app_id=app_A&assistant_id=0"},
		{"POST", "/v1/messages?rag_source_id=rag_1"},
		{"GET", "/v1/chat/completions"},
	} {
		extract, wrapped := serveWithKey(auth, c.method, c.target)
		require.Equal(t, http.StatusForbidden, extract, "%s %s", c.method, c.target)
		require.Equal(t, http.StatusForbidden, wrapped, "%s %s", c.method, c.target)
	}
}

// A session with no agent cannot name one on the LLM proxy or the Helix
// tools backend, and a key whose session is gone is confined the same way.
func TestSessionKeyWithoutAgent(t *testing.T) {
	for name, session := range map[string]*types.Session{
		"no agent":          {ID: "ses_A", Owner: "usr_owner"},
		"session not found": nil,
	} {
		auth := sessionKeyMiddleware(t, session)
		for _, c := range []struct {
			method, target string
			want           int
		}{
			{"POST", "/v1/chat/completions", http.StatusOK},
			{"GET", "/api/v1/sessions/ses_A/zed-config", http.StatusOK},
			{"POST", "/v1/chat/completions?app_id=app_A", http.StatusForbidden},
			{"POST", "/api/v1/mcp/helix?app_id=app_A&session_id=ses_A", http.StatusForbidden},
			{"POST", "/api/v1/mcp/helix?session_id=ses_A", http.StatusForbidden},
			{"GET", "/api/v1/projects", http.StatusForbidden},
		} {
			extract, _ := serveWithKey(auth, c.method, c.target)
			require.Equal(t, c.want, extract, "%s: %s %s", name, c.method, c.target)
		}
	}
}

// A helix-org worker's session key acts as its hiring user by design, and a
// key carrying a project is left to the project-scope checks; neither is
// confined by route. Other restricted key types have their own allowlists.
func TestSessionKeyConfinementLeavesOtherKeys(t *testing.T) {
	worker := &types.Session{ID: "ses_A", Owner: "usr_owner", Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff"}}
	for name, auth := range map[string]*authMiddleware{
		"helix-org worker": desktopKeyMiddleware(t, &types.ApiKey{Type: types.APIkeytypeAPI, OrganizationID: "org_1", SessionID: "ses_A"}, worker),
		"project session":  desktopKeyMiddleware(t, &types.ApiKey{Type: types.APIkeytypeAPI, SessionID: "ses_A", ProjectID: "prj_1"}, chatSession()),
		"personal key":     desktopKeyMiddleware(t, &types.ApiKey{Type: types.APIkeytypeAPI}, nil),
		"org key":          desktopKeyMiddleware(t, &types.ApiKey{Type: types.APIkeytypeAPI, OrganizationID: "org_1"}, nil),
	} {
		extract, _ := serveWithKey(auth, "GET", "/api/v1/projects")
		require.Equal(t, http.StatusOK, extract, name)
	}
}

// The GitHub-compatible API refuses a confined session key like a sandbox
// key; a worker's key is unaffected.
func TestCompatCredentialRejectionForDesktopKeys(t *testing.T) {
	user := &types.User{ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, SessionID: "ses_A"}
	require.NotEmpty(t, compatCredentialRejection(user, &desktopKey{kind: desktopKeySession, id: "ses_A"}))
	require.NotEmpty(t, compatCredentialRejection(user, &desktopKey{kind: desktopKeySandbox, id: "sbx_A"}))
	require.Empty(t, compatCredentialRejection(user, nil))
}

// The LLM proxy attributes calls to the key's chat session; a sandbox key has
// none, and must not fail looking one up.
func TestProxyUsageAttributionForSandboxKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	server := &HelixAPIServer{Store: store.NewMockStore(ctrl)} // no GetSession expected
	attribution, err := server.resolveProxyUsageAttribution(context.Background(), &types.User{
		ID: "usr_owner", TokenType: types.TokenTypeAPIKey, SessionID: "sbx_A",
	}, "resp_1")
	require.NoError(t, err)
	require.Equal(t, "resp_1", attribution.SessionID)
}
