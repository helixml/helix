package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// A project-scoped key (e.g. the session key injected into a spec-task
// desktop) acts as its owner, but only inside its project. These tests pin
// that the confinement runs before every owner / org-owner shortcut.

func newScopedKeyTestServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	return &HelixAPIServer{Store: st}, st
}

func requireScopeError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errors.Is(err, errProjectScopedKey), "expected errProjectScopedKey, got %v", err)
}

func TestAuthorizeUserToSession_ProjectScopedKey(t *testing.T) {
	ctx := context.Background()

	t.Run("stranger is denied a personal session", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		session := &types.Session{ID: "ses_1", Owner: "usr_owner"}
		require.Error(t, srv.authorizeUserToSession(ctx, &types.User{ID: "usr_stranger"}, session, types.ActionGet))
	})

	t.Run("owner without a scoped key is allowed", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		session := &types.Session{ID: "ses_1", Owner: "usr_owner", ProjectID: "prj_b"}
		require.NoError(t, srv.authorizeUserToSession(ctx, &types.User{ID: "usr_owner"}, session, types.ActionGet))
	})

	t.Run("owner's scoped key is denied a session in another project", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		session := &types.Session{ID: "ses_1", Owner: "usr_owner", ProjectID: "prj_b"}
		user := &types.User{ID: "usr_owner", ProjectID: "prj_a"}
		requireScopeError(t, srv.authorizeUserToSession(ctx, user, session, types.ActionGet))
	})

	t.Run("owner's scoped key is denied a chat outside any project", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		session := &types.Session{ID: "ses_chat", Owner: "usr_owner"}
		user := &types.User{ID: "usr_owner", ProjectID: "prj_a"}
		requireScopeError(t, srv.authorizeUserToSession(ctx, user, session, types.ActionGet))
	})

	t.Run("org owner's scoped key is denied another project's org session", func(t *testing.T) {
		// No store expectations: the scope check must refuse before the
		// org-owner shortcut is evaluated.
		srv, _ := newScopedKeyTestServer(t)
		session := &types.Session{ID: "ses_1", Owner: "usr_member", OrganizationID: "org_1", ProjectID: "prj_b"}
		user := &types.User{ID: "usr_org_owner", ProjectID: "prj_a"}
		requireScopeError(t, srv.authorizeUserToSession(ctx, user, session, types.ActionGet))
	})

	t.Run("scoped key reaches its own project's session", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		session := &types.Session{ID: "ses_1", Owner: "usr_owner", ProjectID: "prj_a"}
		user := &types.User{ID: "usr_owner", ProjectID: "prj_a"}
		require.NoError(t, srv.authorizeUserToSession(ctx, user, session, types.ActionGet))
	})

	t.Run("scoped key reaches the session it was minted for", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		session := &types.Session{ID: "ses_task", Owner: "usr_owner"}
		user := &types.User{ID: "usr_owner", ProjectID: "prj_a", SessionID: "ses_task"}
		require.NoError(t, srv.authorizeUserToSession(ctx, user, session, types.ActionGet))
	})
}

func TestAuthorizeUserToApp_ProjectScopedKey(t *testing.T) {
	ctx := context.Background()
	project := &types.Project{ID: "prj_a", DefaultHelixAppID: "app_project"}

	t.Run("stranger is denied", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		app := &types.App{ID: "app_x", Owner: "usr_owner"}
		require.Error(t, srv.authorizeUserToApp(ctx, &types.User{ID: "usr_stranger"}, app, types.ActionGet))
	})

	t.Run("owner without a scoped key is allowed", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		app := &types.App{ID: "app_x", Owner: "usr_owner"}
		require.NoError(t, srv.authorizeUserToApp(ctx, &types.User{ID: "usr_owner"}, app, types.ActionUpdate))
	})

	t.Run("owner's scoped key is denied an app its project does not use", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		st.EXPECT().GetProject(gomock.Any(), "prj_a").Return(project, nil)
		app := &types.App{ID: "app_x", Owner: "usr_owner"}
		requireScopeError(t, srv.authorizeUserToApp(ctx, &types.User{ID: "usr_owner", ProjectID: "prj_a"}, app, types.ActionGet))
	})

	t.Run("org owner's scoped key is denied an org app outside its project", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		st.EXPECT().GetProject(gomock.Any(), "prj_a").Return(project, nil)
		app := &types.App{ID: "app_x", Owner: "usr_member", OrganizationID: "org_1"}
		requireScopeError(t, srv.authorizeUserToApp(ctx, &types.User{ID: "usr_org_owner", ProjectID: "prj_a"}, app, types.ActionGet))
	})

	t.Run("scoped key reaches its project's app", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		st.EXPECT().GetProject(gomock.Any(), "prj_a").Return(project, nil)
		app := &types.App{ID: "app_project", Owner: "usr_owner"}
		require.NoError(t, srv.authorizeUserToApp(ctx, &types.User{ID: "usr_owner", ProjectID: "prj_a"}, app, types.ActionGet))
	})

	t.Run("scoped key reads the app of the session it was minted for", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		st.EXPECT().GetProject(gomock.Any(), "prj_a").Return(project, nil)
		st.EXPECT().GetSession(gomock.Any(), "ses_task").Return(&types.Session{ID: "ses_task", ParentApp: "app_session"}, nil)
		app := &types.App{ID: "app_session", Owner: "usr_owner"}
		user := &types.User{ID: "usr_owner", ProjectID: "prj_a", SessionID: "ses_task"}
		require.NoError(t, srv.authorizeUserToApp(ctx, user, app, types.ActionGet))
	})
}

func TestAuthorizeUserToRepository_ProjectScopedKey(t *testing.T) {
	ctx := context.Background()
	repo := &types.GitRepository{ID: "repo_1", OwnerID: "usr_owner"}

	t.Run("stranger is denied", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		require.Error(t, srv.authorizeUserToRepository(ctx, &types.User{ID: "usr_stranger"}, repo, types.ActionGet))
	})

	t.Run("owner without a scoped key is allowed", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		require.NoError(t, srv.authorizeUserToRepository(ctx, &types.User{ID: "usr_owner"}, repo, types.ActionUpdate))
	})

	t.Run("owner's scoped key is denied a repository outside its project", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		st.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{ProjectID: "prj_a", RepositoryID: "repo_1"}).Return(nil, nil)
		requireScopeError(t, srv.authorizeUserToRepository(ctx, &types.User{ID: "usr_owner", ProjectID: "prj_a"}, repo, types.ActionUpdate))
	})

	t.Run("org owner's scoped key is denied an org repository outside its project", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		orgRepo := &types.GitRepository{ID: "repo_2", OwnerID: "usr_member", OrganizationID: "org_1"}
		st.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{ProjectID: "prj_a", RepositoryID: "repo_2"}).Return(nil, nil)
		requireScopeError(t, srv.authorizeUserToRepository(ctx, &types.User{ID: "usr_org_owner", ProjectID: "prj_a"}, orgRepo, types.ActionGet))
	})

	t.Run("scoped key reaches a repository attached to its project", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		st.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{ProjectID: "prj_a", RepositoryID: "repo_1"}).
			Return([]*types.ProjectRepository{{ProjectID: "prj_a", RepositoryID: "repo_1"}}, nil)
		require.NoError(t, srv.authorizeUserToRepository(ctx, &types.User{ID: "usr_owner", ProjectID: "prj_a"}, repo, types.ActionUpdate))
	})
}

func TestAccessGrantManagement_ProjectScopedKey(t *testing.T) {
	ctx := context.Background()
	user := &types.User{ID: "usr_owner", ProjectID: "prj_a"}

	srv, _ := newScopedKeyTestServer(t)
	requireScopeError(t, srv.authorizeUserToRepositoryAccessGrants(ctx, user, &types.GitRepository{ID: "repo_1", OwnerID: "usr_owner"}))
	requireScopeError(t, srv.authorizeUserToAppAccessGrants(ctx, user, &types.App{ID: "app_1", Owner: "usr_owner", OrganizationID: "org_1"}, types.ActionCreate))
	requireScopeError(t, srv.authorizeUserToProjectAccessGrants(ctx, user, &types.Project{ID: "prj_a", UserID: "usr_owner", OrganizationID: "org_1"}, types.ActionCreate))

	// Unscoped owner keeps managing their repository's grants.
	require.NoError(t, srv.authorizeUserToRepositoryAccessGrants(ctx, &types.User{ID: "usr_owner"}, &types.GitRepository{ID: "repo_1", OwnerID: "usr_owner"}))
}

func TestListSessions_ProjectScopedKey(t *testing.T) {
	user := types.User{ID: "usr_owner", Type: types.OwnerTypeUser, ProjectID: "prj_a"}

	t.Run("list is confined to the key's project", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		st.EXPECT().ListSessions(gomock.Any(), store.ListSessionsQuery{
			Owner:              user.ID,
			OwnerType:          user.Type,
			Page:               0,
			PerPage:            50,
			ExcludeArchived:    true,
			RestrictToProjects: true,
			ProjectIDs:         []string{"prj_a"},
		}).Return(nil, int64(0), nil)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", http.NoBody)
		req = req.WithContext(setRequestUser(req.Context(), user))
		_, err := srv.listSessions(httptest.NewRecorder(), req)
		require.NoError(t, err)
	})

	t.Run("another project is refused", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions?project_id=prj_b", http.NoBody)
		req = req.WithContext(setRequestUser(req.Context(), user))
		_, err := srv.listSessions(httptest.NewRecorder(), req)
		var httpErr *system.HTTPError
		require.ErrorAs(t, err, &httpErr)
		assert.Equal(t, http.StatusForbidden, httpErr.StatusCode)
	})
}

// The user session stream carries a session's full content, so subscribing
// needs read access to that session.
func TestUserWebSocket_AuthorizesSession(t *testing.T) {
	const token = "token-123"
	cases := []struct {
		name     string
		user     *types.User
		session  *types.Session
		wantCode int
	}{
		{
			name:     "stranger is refused",
			user:     &types.User{ID: "usr_stranger"},
			session:  &types.Session{ID: "ses_1", Owner: "usr_owner"},
			wantCode: http.StatusForbidden,
		},
		{
			name:     "scoped key is refused a session outside its project",
			user:     &types.User{ID: "usr_owner", ProjectID: "prj_a"},
			session:  &types.Session{ID: "ses_1", Owner: "usr_owner", ProjectID: "prj_b"},
			wantCode: http.StatusForbidden,
		},
		{
			// httptest cannot hijack, so an authorized request fails at the
			// websocket upgrade (400) rather than the authorization check.
			name:     "owner passes authorization",
			user:     &types.User{ID: "usr_owner"},
			session:  &types.Session{ID: "ses_1", Owner: "usr_owner"},
			wantCode: http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			mockAuth := authpkg.NewMockAuthenticator(ctrl)
			mockAuth.EXPECT().ValidateUserToken(gomock.Any(), token).Return(tc.user, nil)
			st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: tc.user.ID}, nil)
			st.EXPECT().GetSession(gomock.Any(), tc.session.ID).Return(tc.session, nil)
			srv := &HelixAPIServer{Store: st, authMiddleware: newAuthMiddleware(mockAuth, nil, st, authMiddlewareConfig{}, nil, nil)}
			router := mux.NewRouter()
			srv.startUserWebSocketServer(context.Background(), router, "/api/v1/ws/user")

			req := httptest.NewRequest(http.MethodGet, "/api/v1/ws/user?session_id="+tc.session.ID, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			assert.Equal(t, tc.wantCode, rec.Code)
		})
	}
}
