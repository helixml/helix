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
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: sandboxKeyToken}).Return(&types.ApiKey{
		Key:            sandboxKeyToken,
		Owner:          "usr_owner",
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: "org_1",
		Type:           types.APIkeytypeAPI,
		SessionID:      sessionID,
		ProjectID:      projectID,
		AppID:          &sql.NullString{},
	}, nil).AnyTimes()
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_owner"}).Return(&types.User{ID: "usr_owner"}, nil).AnyTimes()
	st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: "usr_owner"}, nil).AnyTimes()
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
		{"GET", "/v1/chat/completions"},
	} {
		extract, wrapped := serveWithKey(auth, c.method, c.target)
		require.Equal(t, http.StatusForbidden, extract, "%s %s", c.method, c.target)
		require.Equal(t, http.StatusForbidden, wrapped, "%s %s", c.method, c.target)
	}
}

// Route confinement is for sandboxes outside a project: a project sandbox's
// key is confined to its project by the project-scope checks, and keys bound
// to chat sessions are unchanged.
func TestSandboxKeyConfinementLeavesOtherScopedKeys(t *testing.T) {
	for _, c := range []struct{ sessionID, projectID string }{
		{"sbx_A", "prj_1"},
		{"ses_A", ""},
	} {
		auth := sandboxKeyMiddleware(t, c.sessionID, c.projectID)
		extract, _ := serveWithKey(auth, "GET", "/api/v1/projects")
		require.Equal(t, http.StatusOK, extract, "session %s project %s", c.sessionID, c.projectID)
	}
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
