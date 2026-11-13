package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// A project key (minted with POST /api/v1/api_keys and a project_id) reaches
// only its project, the LLM proxy and identity reads. Everything else is
// refused by the auth middleware before any handler runs.

const projectKeySecret = "hl-project-key-secret"

func projectKeyMiddleware(t *testing.T, owner *types.User, key *types.ApiKey) *authMiddleware {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	if key == nil {
		st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: projectKeySecret}).Return(nil, store.ErrNotFound).AnyTimes()
	} else {
		st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: projectKeySecret}).Return(key, nil).AnyTimes()
	}
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: owner.ID}).DoAndReturn(
		func(_, _ any) (*types.User, error) { copied := *owner; return &copied, nil }).AnyTimes()
	st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: owner.ID}, nil).AnyTimes()
	auth := newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, nil, nil)
	auth.lastSeenCache.Store(owner.ID, time.Now())
	return auth
}

func projectKeyRow(owner string) *types.ApiKey {
	return &types.ApiKey{Key: projectKeySecret, Name: "portal", Owner: owner, OwnerType: types.OwnerTypeUser,
		Type: types.APIkeytypeAPI, OrganizationID: "org_a", ProjectID: "prj_a"}
}

// serveWithProjectKey runs the request through the middleware that fronts its
// route and reports the status and whether the handler ran.
func serveWithProjectKey(auth *authMiddleware, method, target string) (int, bool) {
	served := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served = true
		w.WriteHeader(http.StatusNoContent)
	})
	r := httptest.NewRequest(method, target, nil)
	r.Header.Set("Authorization", "Bearer "+projectKeySecret)
	rr := httptest.NewRecorder()
	if len(target) >= 4 && target[:4] == "/v1/" {
		auth.auth(next)(rr, r)
	} else {
		auth.extractMiddleware(next).ServeHTTP(rr, r)
	}
	return rr.Code, served
}

func TestProjectKeyReachesOnlyItsProject(t *testing.T) {
	owner := &types.User{ID: "usr_owner", Admin: true}
	auth := projectKeyMiddleware(t, owner, projectKeyRow(owner.ID))

	allowed := []struct{ method, target string }{
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/v1/chat/completions?app_id=app_project_agent"},
		{http.MethodGet, "/v1/models"},
		{http.MethodGet, "/api/v1/status"},
		{http.MethodGet, "/api/v1/projects/prj_a"},
		{http.MethodPut, "/api/v1/projects/prj_a"},
		{http.MethodGet, "/api/v1/projects/prj_a/repositories"},
		{http.MethodGet, "/api/v1/projects/prj_a/secrets"},
		{http.MethodGet, "/api/v1/spec-tasks?project_id=prj_a"},
		{http.MethodPost, "/api/v1/spec-tasks/from-prompt"},
		{http.MethodGet, "/api/v1/spec-tasks/spt_1"},
		{http.MethodGet, "/api/v1/sessions/ses_1"},
		{http.MethodPost, "/api/v1/sessions/ses_1/messages"},
		{http.MethodGet, "/api/v1/git/repositories/repo_1/tree"},
	}
	for _, tc := range allowed {
		t.Run("allows "+tc.method+" "+tc.target, func(t *testing.T) {
			code, served := serveWithProjectKey(auth, tc.method, tc.target)
			assert.Equal(t, http.StatusNoContent, code)
			assert.True(t, served)
		})
	}

	refused := []struct{ method, target string }{
		// Other projects and the project listing.
		{http.MethodGet, "/api/v1/projects/prj_b"},
		{http.MethodGet, "/api/v1/projects/prj_ab"},
		{http.MethodGet, "/api/v1/projects"},
		{http.MethodPost, "/api/v1/projects"},
		// Decisions about its own project that belong to a person.
		{http.MethodDelete, "/api/v1/projects/prj_a"},
		{http.MethodGet, "/api/v1/projects/prj_a/api_keys"},
		{http.MethodDelete, "/api/v1/projects/prj_a/api_keys/apikey_x"},
		{http.MethodPost, "/api/v1/projects/prj_a/access-grants"},
		{http.MethodPost, "/api/v1/projects/prj_a/move"},
		{http.MethodPost, "/api/v1/projects/prj_a/move/preview"},
		// Key management.
		{http.MethodPost, "/api/v1/api_keys"},
		{http.MethodGet, "/api/v1/api_keys"},
		{http.MethodGet, "/api/v1/api_keys?types=all"},
		{http.MethodDelete, "/api/v1/api_keys?key=hl-other"},
		{http.MethodGet, "/api/v1/api_keys/check?key=hl-other"},
		// Organization and admin routes.
		{http.MethodGet, "/api/v1/organizations/org_a"},
		{http.MethodGet, "/api/v1/organizations/org_a/members"},
		{http.MethodPost, "/api/v1/organizations/org_a/api_keys"},
		{http.MethodGet, "/api/v1/organizations/org_a/sandboxes"},
		{http.MethodGet, "/api/v1/admin/users"},
		// Secrets, settings and providers outside the project.
		{http.MethodGet, "/api/v1/secrets"},
		{http.MethodPost, "/api/v1/secrets"},
		{http.MethodGet, "/api/v1/users/me/chat-settings"},
		{http.MethodPut, "/api/v1/users/me/chat-settings"},
		{http.MethodGet, "/api/v1/provider-endpoints"},
		{http.MethodGet, "/api/v1/oauth/connections"},
		{http.MethodGet, "/api/v1/apps"},
		// Objects created outside the project, and unscoped listings.
		{http.MethodPost, "/api/v1/sessions/chat"},
		{http.MethodGet, "/api/v1/sessions"},
		{http.MethodPost, "/api/v1/git/repositories"},
		{http.MethodGet, "/api/v1/git/repositories"},
		{http.MethodPost, "/api/v1/git/repositories/from-template"},
		{http.MethodGet, "/api/v1/mcp/session?session_id=ses_1"},
		// RAG sources and assistants are authorized by owner only.
		{http.MethodPost, "/v1/chat/completions?rag_source_id=rag_1"},
		{http.MethodPost, "/v1/chat/completions?assistant_id=0"},
		{http.MethodGet, "/v1/chat/completions"},
	}
	for _, tc := range refused {
		t.Run("refuses "+tc.method+" "+tc.target, func(t *testing.T) {
			code, served := serveWithProjectKey(auth, tc.method, tc.target)
			assert.Equal(t, http.StatusForbidden, code)
			assert.False(t, served, "the handler must not run")
		})
	}
}

// A project key acts without its owner's global admin, as every scoped key.
func TestProjectKeyDropsOwnersAdmin(t *testing.T) {
	owner := &types.User{ID: "usr_owner", Admin: true}
	auth := projectKeyMiddleware(t, owner, projectKeyRow(owner.ID))
	user, err := auth.getUserFromToken(t.Context(), projectKeySecret)
	require.NoError(t, err)
	assert.False(t, user.Admin)
	assert.Equal(t, "prj_a", user.ProjectID)
	assert.Equal(t, "org_a", user.OrganizationID)
}

// A revoked key is deleted, so the next request with it is unauthenticated.
func TestRevokedProjectKeyIsUnauthorized(t *testing.T) {
	owner := &types.User{ID: "usr_owner"}
	auth := projectKeyMiddleware(t, owner, nil)
	for _, target := range []string{"/api/v1/projects/prj_a", "/v1/chat/completions"} {
		code, served := serveWithProjectKey(auth, http.MethodPost, target)
		assert.Equal(t, http.StatusUnauthorized, code, target)
		assert.False(t, served, target)
	}
}

// A waitlisted or deactivated owner's project key reaches nothing, not even
// the routes its project scope allows.
func TestProjectKeyOfInactiveOwnerIsRefused(t *testing.T) {
	for state, set := range accountStates {
		t.Run(state, func(t *testing.T) {
			owner := &types.User{ID: "usr_owner"}
			set(owner)
			auth := projectKeyMiddleware(t, owner, projectKeyRow(owner.ID))
			for _, target := range []string{"/api/v1/projects/prj_a", "/v1/chat/completions"} {
				code, served := serveWithProjectKey(auth, http.MethodPost, target)
				assert.Equal(t, http.StatusForbidden, code, target)
				assert.False(t, served, target)
			}
		})
	}
}

// Proxy usage with a project key is recorded against its project and
// organization.
func TestProjectKeyChatCompletionIsAttributedToItsProject(t *testing.T) {
	upstream := newFakeOpenAI(t)
	server, mockStore, capture := codeAgentTestServer(t, upstream)
	mockStore.EXPECT().GetUserMeta(gomock.Any(), "user_test").Return(&types.UserMeta{ID: "user_test"}, nil).AnyTimes()

	recorder := httptest.NewRecorder()
	server.createChatCompletion(recorder, codeAgentChatRequest(t, types.User{
		ID:             "user_test",
		TokenType:      types.TokenTypeAPIKey,
		APIKeyType:     types.APIkeytypeAPI,
		OrganizationID: "org_a",
		ProjectID:      "prj_a",
	}))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Eventually(t, func() bool { return capture.get() != nil }, 5*time.Second, 20*time.Millisecond)
	call := capture.get()
	assert.Equal(t, "prj_a", call.ProjectID)
	assert.Equal(t, "org_a", call.OrganizationID)
	assert.Equal(t, "user_test", call.UserID)
	assert.Empty(t, call.SpecTaskID)
}
