package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Endpoints that hand out a session's sandbox configuration, drive its
// desktop or read its contents must apply session RBAC (and with it the
// project scope of the caller's key), not just compare owners.

func sessionRequest(method, path, sessionID string, user *types.User) *http.Request {
	req := httptest.NewRequest(method, path, http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": sessionID})
	return req.WithContext(setRequestUser(req.Context(), *user))
}

// sandboxAccessCases cover the three caller shapes every gated endpoint must
// refuse: a stranger, the owner's key scoped to another project, and the
// owner's key bound to another session.
func sandboxAccessCases() []struct {
	name    string
	user    *types.User
	session *types.Session
} {
	return []struct {
		name    string
		user    *types.User
		session *types.Session
	}{
		{"stranger", &types.User{ID: "usr_stranger"}, &types.Session{ID: "ses_b", Owner: "usr_owner", ProjectID: "prj_b"}},
		{"owner key scoped to another project", &types.User{ID: "usr_owner", TokenType: types.TokenTypeAPIKey, ProjectID: "prj_a"}, &types.Session{ID: "ses_b", Owner: "usr_owner", ProjectID: "prj_b"}},
	}
}

func TestGetZedConfig_RefusesUnauthorizedCallers(t *testing.T) {
	cases := sandboxAccessCases()
	cases = append(cases, struct {
		name    string
		user    *types.User
		session *types.Session
	}{
		"owner key bound to another session",
		&types.User{ID: "usr_owner", TokenType: types.TokenTypeAPIKey, ProjectID: "prj_b", SessionID: "ses_other"},
		&types.Session{ID: "ses_b", Owner: "usr_owner", ProjectID: "prj_b"},
	})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, st := newScopedKeyTestServer(t)
			st.EXPECT().GetSession(gomock.Any(), tc.session.ID).Return(tc.session, nil).Times(3)

			_, herr := srv.getZedConfig(nil, sessionRequest(http.MethodGet, "/api/v1/sessions/ses_b/zed-config", tc.session.ID, tc.user))
			require.NotNil(t, herr)
			assert.Equal(t, http.StatusForbidden, herr.StatusCode)

			_, herr = srv.getMergedZedSettings(nil, sessionRequest(http.MethodGet, "/api/v1/sessions/ses_b/zed-settings", tc.session.ID, tc.user))
			require.NotNil(t, herr)
			assert.Equal(t, http.StatusForbidden, herr.StatusCode)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/ses_b/zed-config/user", strings.NewReader(`{}`))
			req = mux.SetURLVars(req, map[string]string{"id": tc.session.ID})
			req = req.WithContext(setRequestUser(req.Context(), *tc.user))
			_, herr = srv.updateZedUserSettings(nil, req)
			require.NotNil(t, herr)
			assert.Equal(t, http.StatusForbidden, herr.StatusCode)
		})
	}
}

func TestAuthorizeSessionSandboxConfig_Allows(t *testing.T) {
	srv, _ := newScopedKeyTestServer(t)
	session := &types.Session{ID: "ses_b", Owner: "usr_owner", ProjectID: "prj_b", SandboxID: "host1"}
	ctx := context.Background()

	require.NoError(t, srv.authorizeSessionSandboxConfig(ctx, &types.User{ID: "usr_owner"}, session), "owner")
	require.NoError(t, srv.authorizeSessionSandboxConfig(ctx,
		&types.User{ID: "usr_owner", TokenType: types.TokenTypeAPIKey, ProjectID: "prj_b", SessionID: "ses_b"}, session),
		"the session's own key")
	require.NoError(t, srv.authorizeSessionSandboxConfig(ctx,
		&types.User{ID: "runner-system", TokenType: types.TokenTypeRunner, RunnerID: "host1"}, session),
		"per-runner token for the session's host")

	// A project member who is not the owner still may not read the owner's
	// session key.
	require.Error(t, srv.authorizeSessionSandboxConfig(ctx, &types.User{ID: "usr_member"}, session))
}

func TestGetSessionRDPConnection_AuthorizesSession(t *testing.T) {
	for _, tc := range sandboxAccessCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, st := newScopedKeyTestServer(t)
			st.EXPECT().GetSession(gomock.Any(), tc.session.ID).Return(tc.session, nil)
			rec := httptest.NewRecorder()
			srv.getSessionRDPConnection(rec, sessionRequest(http.MethodGet, "/api/v1/sessions/ses_b/rdp-connection", tc.session.ID, tc.user))
			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}

	t.Run("owner passes authorization", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		session := &types.Session{ID: "ses_b", Owner: "usr_owner", ProjectID: "prj_b"}
		st.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
		rec := httptest.NewRecorder()
		srv.getSessionRDPConnection(rec, sessionRequest(http.MethodGet, "/api/v1/sessions/ses_b/rdp-connection", session.ID, &types.User{ID: "usr_owner"}))
		// Not an external-agent session, so it stops after authorization.
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestGetSessionTOC_AuthorizesSession(t *testing.T) {
	for _, tc := range sandboxAccessCases() {
		t.Run(tc.name, func(t *testing.T) {
			srv, st := newScopedKeyTestServer(t)
			st.EXPECT().GetSession(gomock.Any(), tc.session.ID).Return(tc.session, nil)
			_, herr := srv.getSessionTOC(nil, sessionRequest(http.MethodGet, "/api/v1/sessions/ses_b/toc", tc.session.ID, tc.user))
			require.NotNil(t, herr)
			assert.Equal(t, http.StatusForbidden, herr.StatusCode)
		})
	}

	t.Run("owner is allowed", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		session := &types.Session{ID: "ses_b", Owner: "usr_owner", ProjectID: "prj_b"}
		st.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil)
		st.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil)
		_, herr := srv.getSessionTOC(nil, sessionRequest(http.MethodGet, "/api/v1/sessions/ses_b/toc", session.ID, &types.User{ID: "usr_owner"}))
		require.Nil(t, herr)
	})
}

func TestDesktopMCP_RefusesScopedKeyOutsideItsProject(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	backend := NewDesktopMCPBackend(st, nil, (&HelixAPIServer{Store: st}).authorizeUserToSession)
	st.EXPECT().GetSession(gomock.Any(), "ses_b").Return(&types.Session{ID: "ses_b", Owner: "usr_owner", ProjectID: "prj_b"}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/desktop?session_id=ses_b", http.NoBody)
	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "usr_owner", ProjectID: "prj_a"})
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// Keys minted for a project or session must never carry the owner's global
// admin flag: every isAdmin shortcut would skip their confinement.
func TestAPIKeyAdminFlag_ClearedForScopedKeys(t *testing.T) {
	cases := []struct {
		name      string
		key       *types.ApiKey
		wantAdmin bool
	}{
		{"plain key keeps admin", &types.ApiKey{Key: "hl-plain", Owner: "usr_admin", Type: types.APIkeytypeAPI}, true},
		{"project key drops admin", &types.ApiKey{Key: "hl-project", Owner: "usr_admin", Type: types.APIkeytypeAPI, ProjectID: "prj_a"}, false},
		{"session key drops admin", &types.ApiKey{Key: "hl-session", Owner: "usr_admin", Type: types.APIkeytypeAPI, SessionID: "ses_a"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: tc.key.Key}).Return(tc.key, nil)
			st.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: "usr_admin", Admin: true}, nil)
			st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: "usr_admin"}, nil).AnyTimes()
			auth := newAuthMiddleware(nil, nil, st, authMiddlewareConfig{adminUserIDs: []string{"usr_admin"}}, nil, nil)

			user, err := auth.getUserFromToken(context.Background(), tc.key.Key)
			require.NoError(t, err)
			assert.Equal(t, tc.wantAdmin, user.Admin)
		})
	}
}

// The agent holding a task's key must not approve its own pull requests or
// its own completion.
func TestProposalDecisions_RefuseAgentScopedKeys(t *testing.T) {
	srv, _ := newScopedKeyTestServer(t)
	for _, user := range []*types.User{
		{ID: "usr_owner", ProjectID: "prj_a"},
		{ID: "usr_owner", SessionID: "ses_a"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/spt_1/pr-proposals/prp_1/decide", strings.NewReader(`{"decision":"approve"}`))
		req = mux.SetURLVars(req, map[string]string{"spec_task_id": "spt_1", "proposal_id": "prp_1"})
		req = req.WithContext(setRequestUser(req.Context(), *user))
		rec := httptest.NewRecorder()
		srv.decideSpecTaskPRProposal(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)

		req = httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/spt_1/completion/decide", strings.NewReader(`{"decision":"approve"}`))
		req = mux.SetURLVars(req, map[string]string{"spec_task_id": "spt_1"})
		req = req.WithContext(setRequestUser(req.Context(), *user))
		rec = httptest.NewRecorder()
		srv.decideSpecTaskCompletion(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	}
	assert.False(t, isAgentScopedKey(&types.User{ID: "usr_owner"}), "an ordinary login decides as before")
}

func TestCanRedirectRepositoryUpstream_RefusesScopedKey(t *testing.T) {
	srv, _ := newScopedKeyTestServer(t)
	repo := &types.GitRepository{ID: "repo_1", OwnerID: "usr_owner"}
	assert.True(t, srv.canRedirectRepositoryUpstream(context.Background(), &types.User{ID: "usr_owner"}, repo))
	assert.False(t, srv.canRedirectRepositoryUpstream(context.Background(), &types.User{ID: "usr_owner", ProjectID: "prj_a"}, repo))
}

// owner_id lists are bounded to the projects the caller may read; for a
// scoped key that must intersect with its own project.
func TestListSessions_OwnerIDPath_ScopedKey(t *testing.T) {
	srv, st := newScopedKeyTestServer(t)
	user := types.User{ID: "usr_org_owner", Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey, ProjectID: "prj_a"}
	org := &types.Organization{ID: "org_1"}

	st.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(org, nil).AnyTimes()
	st.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(&types.OrganizationMembership{
		OrganizationID: "org_1", UserID: user.ID, Role: types.OrganizationRoleOwner,
	}, nil).AnyTimes()
	st.EXPECT().ListProjects(gomock.Any(), gomock.Any()).Return([]*types.Project{
		{ID: "prj_a", OrganizationID: "org_1"},
		{ID: "prj_b", OrganizationID: "org_1"},
	}, nil)
	st.EXPECT().ListSessions(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q store.ListSessionsQuery) ([]*types.Session, int64, error) {
			assert.Equal(t, "usr_member", q.Owner)
			assert.True(t, q.RestrictToProjects)
			assert.Equal(t, []string{"prj_a"}, q.ProjectIDs)
			return nil, 0, nil
		})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions?org_id=org_1&owner_id=usr_member", http.NoBody)
	req = req.WithContext(setRequestUser(req.Context(), user))
	_, err := srv.listSessions(httptest.NewRecorder(), req)
	require.NoError(t, err)
}
