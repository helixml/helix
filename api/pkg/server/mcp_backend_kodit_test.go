//go:build !nokodit

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	apiskill "github.com/helixml/helix/api/pkg/agent/skill/api_skills"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit"
)

// ---------------------------------------------------------------------------
// Backend-level tests
// ---------------------------------------------------------------------------

func TestKoditMCPBackend_Disabled(t *testing.T) {
	backend := NewKoditMCPBackend(nil, false, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/mcp/kodit/", nil)
	backend.ServeHTTP(rec, req, &types.User{ID: "u"})
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestKoditMCPBackend_RequiresSessionID(t *testing.T) {
	// An enabled backend with nil kodit client should still be disabled
	backend := NewKoditMCPBackend(nil, true, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/mcp/kodit/", nil)
	backend.ServeHTTP(rec, req, &types.User{ID: "u"})
	// nil koditClient makes it disabled
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

// ---------------------------------------------------------------------------
// Repo resolver tests
// ---------------------------------------------------------------------------

func indexedRepo(id string, koditID int64) *types.GitRepository {
	return &types.GitRepository{ID: id, KoditIndexing: true, Metadata: map[string]interface{}{"kodit_repo_id": koditID}}
}

// allowRepos is a RepositoryAuthorizer that grants exactly the listed
// repository ids.
func allowRepos(ids ...string) RepositoryAuthorizer {
	allowed := make(map[string]bool, len(ids))
	for _, id := range ids {
		allowed[id] = true
	}
	return func(_ context.Context, _ *types.User, repo *types.GitRepository, _ types.Action) error {
		if allowed[repo.ID] {
			return nil
		}
		return errors.New("no access")
	}
}

func TestResolveKoditRepoScope(t *testing.T) {
	ctx := context.Background()
	projectSession := &types.Session{ID: "ses_p", Owner: "usr_a", OrganizationID: "org_1", ProjectID: "prj_1"}
	koditProject := &types.Project{ID: "prj_1", OrganizationID: "org_1", KoditEnabled: true}

	cases := []struct {
		name      string
		session   *types.Session
		user      *types.User
		authorize RepositoryAuthorizer
		setup     func(st *store.MockStore)
		want      []int64
	}{
		{
			// Organization-wide code search stays, but only over the
			// repositories the caller may read: repo_org_hidden has no grant.
			name:      "project session searches the org repos the caller may read",
			session:   projectSession,
			user:      &types.User{ID: "usr_a", OrganizationID: "org_1"},
			authorize: allowRepos("repo_project", "repo_org_visible"),
			setup: func(st *store.MockStore) {
				st.EXPECT().GetProject(gomock.Any(), "prj_1").Return(koditProject, nil)
				st.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: "prj_1"}).
					Return([]*types.GitRepository{indexedRepo("repo_project", 10)}, nil)
				st.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{OrganizationID: "org_1"}).
					Return([]*types.GitRepository{
						indexedRepo("repo_project", 10),
						indexedRepo("repo_org_visible", 20),
						indexedRepo("repo_org_hidden", 30),
					}, nil)
			},
			want: []int64{10, 20},
		},
		{
			// A key minted for the project reaches only the project's
			// repositories over REST, so the org listing is never consulted.
			name:      "project-scoped key sees only its project's repos",
			session:   projectSession,
			user:      &types.User{ID: "usr_a", OrganizationID: "org_1", ProjectID: "prj_1", SessionID: "ses_p"},
			authorize: allowRepos("repo_project", "repo_org_visible"),
			setup: func(st *store.MockStore) {
				st.EXPECT().GetProject(gomock.Any(), "prj_1").Return(koditProject, nil)
				st.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: "prj_1"}).
					Return([]*types.GitRepository{indexedRepo("repo_project", 10)}, nil)
			},
			want: []int64{10},
		},
		{
			name:      "project taken from session metadata",
			session:   &types.Session{ID: "ses_m", Owner: "usr_a", Metadata: types.SessionMetadata{ProjectID: "prj_1"}},
			user:      &types.User{ID: "usr_a", ProjectID: "prj_1"},
			authorize: allowRepos("repo_project"),
			setup: func(st *store.MockStore) {
				st.EXPECT().GetProject(gomock.Any(), "prj_1").Return(koditProject, nil)
				st.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: "prj_1"}).
					Return([]*types.GitRepository{indexedRepo("repo_project", 10)}, nil)
			},
			want: []int64{10},
		},
		{
			name:      "project with Kodit disabled",
			session:   projectSession,
			user:      &types.User{ID: "usr_a"},
			authorize: allowRepos("repo_project"),
			setup: func(st *store.MockStore) {
				st.EXPECT().GetProject(gomock.Any(), "prj_1").Return(&types.Project{ID: "prj_1", OrganizationID: "org_1"}, nil)
			},
		},
		{
			// The org comes from the session, not from the caller's key.
			name:      "helix-org worker uses the session's org",
			session:   &types.Session{ID: "ses_o", Owner: "usr_a", OrganizationID: "org_session", Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff"}},
			user:      &types.User{ID: "usr_a", OrganizationID: "org_key"},
			authorize: allowRepos("repo_a"),
			setup: func(st *store.MockStore) {
				st.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{OrganizationID: "org_session"}).
					Return([]*types.GitRepository{indexedRepo("repo_a", 100), indexedRepo("repo_b", 200)}, nil)
			},
			want: []int64{100},
		},
		{
			name:      "helix-org worker without an org uses the session owner's repos",
			session:   &types.Session{ID: "ses_u", Owner: "usr_solo", Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff"}},
			user:      &types.User{ID: "usr_solo"},
			authorize: allowRepos("repo_x"),
			setup: func(st *store.MockStore) {
				st.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{OwnerID: "usr_solo"}).
					Return([]*types.GitRepository{{ID: "repo_x", KoditIndexing: true, Metadata: map[string]interface{}{"kodit_repo_id": float64(42)}}}, nil)
			},
			want: []int64{42},
		},
		{
			name:      "unindexed repos and repos without a Kodit id are skipped",
			session:   &types.Session{ID: "ses_u", Owner: "usr_solo", Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff"}},
			user:      &types.User{ID: "usr_solo"},
			authorize: allowRepos("repo_off", "repo_no_meta", "repo_no_id", "repo_zero"),
			setup: func(st *store.MockStore) {
				st.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{
					{ID: "repo_off", KoditIndexing: false, Metadata: map[string]interface{}{"kodit_repo_id": int64(1)}},
					{ID: "repo_no_meta", KoditIndexing: true},
					{ID: "repo_no_id", KoditIndexing: true, Metadata: map[string]interface{}{"some_key": "value"}},
					indexedRepo("repo_zero", 0),
				}, nil)
			},
		},
		{
			// A plain chat outside any project lists no repositories at all.
			name:      "plain chat outside any project",
			session:   &types.Session{ID: "ses_chat", Owner: "usr_a", OrganizationID: "org_1"},
			user:      &types.User{ID: "usr_a", OrganizationID: "org_1"},
			authorize: allowRepos("repo_a"),
			setup:     func(*store.MockStore) {},
		},
		{
			name:    "no repository authorizer fails closed",
			session: &types.Session{ID: "ses_u", Owner: "usr_solo"},
			user:    &types.User{ID: "usr_solo"},
			setup:   func(*store.MockStore) {},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMockStore(gomock.NewController(t))
			tc.setup(st)
			scope, err := resolveKoditRepoScope(ctx, st, tc.authorize, tc.session, tc.user)
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.want, scope.idSlice)
			assert.Len(t, scope.repoIDs, len(tc.want))
		})
	}
}

// ---------------------------------------------------------------------------
// Integration: enable endpoint → Kodit backend route
// ---------------------------------------------------------------------------

// TestEnableSkillRouteToKoditBackend verifies the end-to-end wiring:
//  1. The enable endpoint produces an AssistantMCP whose URL ends with /api/v1/mcp/kodit.
//  2. A request to that URL path (served by the disabled Kodit backend) returns 501,
//     confirming the route is correctly registered and the URL is right.
func TestEnableSkillRouteToKoditBackend(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	serverURL := "http://helix.test"
	userID := "user-route"
	appID := "app-route"
	apiKey := "hl-route-key"

	app := &types.App{
		ID:    appID,
		Owner: userID,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{Name: "Agent"}},
			},
		},
	}

	// Enable endpoint mock expectations.
	mockStore.EXPECT().GetApp(gomock.Any(), appID).Return(app, nil)
	mockStore.EXPECT().ListAPIKeys(gomock.Any(), gomock.Any()).Return([]*types.ApiKey{{Key: apiKey}}, nil)
	mockStore.EXPECT().UpdateApp(gomock.Any(), gomock.Any()).DoAndReturn(func(_ interface{}, updated *types.App) (*types.App, error) {
		return updated, nil
	})

	sm := apiskill.NewManager()
	assert.NoError(t, sm.LoadSkills(context.Background()))

	srv := &HelixAPIServer{
		Store: mockStore,
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{URL: serverURL},
		},
		skillManager: sm,
		kodit:        &KoditResult{mcpBackend: NewKoditMCPBackend(nil, false, mockStore)},
	}

	// Step 1: call the enable endpoint and capture the MCP config.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/"+appID+"/skills/code-intelligence/enable", nil)
	req = mux.SetURLVars(req, map[string]string{"id": appID, "skill": "code-intelligence"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID, Type: types.OwnerTypeUser}))
	rr := httptest.NewRecorder()
	srv.handleEnableSkill(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Step 2: verify the produced URL points to the Kodit MCP backend path.
	updatedApp := app.Config.Helix.Assistants[0]
	assert.Len(t, updatedApp.MCPs, 1)
	mcpURL := updatedApp.MCPs[0].URL
	assert.Equal(t, serverURL+"/api/v1/mcp/kodit", mcpURL)

	// Step 3: confirm the Kodit backend (disabled) is reachable at that path
	// and returns 501 Not Implemented (not 404, which would mean the route is missing).
	koditReq := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/kodit", nil)
	koditRec := httptest.NewRecorder()
	srv.kodit.mcpBackend.ServeHTTP(koditRec, koditReq, &types.User{ID: userID})
	assert.Equal(t, http.StatusNotImplemented, koditRec.Code, "Kodit backend should return 501 when disabled, not 404")
}

// The Kodit repository scope is resolved from a caller-chosen session_id, so
// the caller must be able to read that session.
func TestKoditMCPBackend_AuthorizesSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	backend := &KoditMCPBackend{store: st}
	backend.SetSessionAuthorizer((&HelixAPIServer{Store: st}).authorizeUserToSession)
	session := &types.Session{ID: "ses_b", Owner: "usr_owner", ProjectID: "prj_b"}
	st.EXPECT().GetSession(gomock.Any(), "ses_b").Return(session, nil).Times(3)

	ctx := context.Background()
	_, err := backend.authorizedSession(ctx, "ses_b", &types.User{ID: "usr_stranger"})
	assert.Error(t, err)
	_, err = backend.authorizedSession(ctx, "ses_b", &types.User{ID: "usr_owner", ProjectID: "prj_a"})
	assert.Error(t, err)
	got, err := backend.authorizedSession(ctx, "ses_b", &types.User{ID: "usr_owner"})
	assert.NoError(t, err)
	assert.Equal(t, session, got)

	unwired := &KoditMCPBackend{store: st}
	_, err = unwired.authorizedSession(ctx, "ses_b", &types.User{ID: "usr_owner"})
	assert.Error(t, err, "fails closed without an authorizer")
}

// kodit.NewScopedMCPHandler treats an empty repository set as "every
// repository on the instance", so an empty scope must never reach it.
func TestScopedKoditMCPHandler_RefusesEmptyScope(t *testing.T) {
	for _, ids := range [][]int64{nil, {}} {
		handler, err := scopedKoditMCPHandler(nil, ids)
		assert.ErrorIs(t, err, errNoKoditRepositories)
		assert.Nil(t, handler)
	}
}

// A session whose scope resolves to no repositories gets 403, not an
// unscoped handler: a project with Kodit off, and a user with nothing indexed.
func TestKoditMCPBackend_EmptyScopeIsForbidden(t *testing.T) {
	cases := []struct {
		name    string
		session *types.Session
		user    *types.User
		setup   func(st *store.MockStore)
	}{
		{
			name:    "project with Kodit disabled",
			session: &types.Session{ID: "ses_p", Owner: "usr_a", ProjectID: "prj_off"},
			user:    &types.User{ID: "usr_a"},
			setup: func(st *store.MockStore) {
				st.EXPECT().GetProject(gomock.Any(), "prj_off").Return(&types.Project{ID: "prj_off", KoditEnabled: false}, nil)
			},
		},
		{
			name:    "helix-org worker whose owner has no indexed repositories",
			session: &types.Session{ID: "ses_u", Owner: "usr_b", Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff"}},
			user:    &types.User{ID: "usr_b"},
			setup: func(st *store.MockStore) {
				st.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return([]*types.GitRepository{
					{ID: "repo_unindexed", KoditIndexing: false},
				}, nil)
			},
		}, {
			name:    "plain chat outside any project",
			session: &types.Session{ID: "ses_c", Owner: "usr_c"},
			user:    &types.User{ID: "usr_c", OrganizationID: "org_c"},
			setup:   func(*store.MockStore) {},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			st.EXPECT().GetSession(gomock.Any(), tc.session.ID).Return(tc.session, nil).AnyTimes()
			tc.setup(st)
			backend := &KoditMCPBackend{store: st, enabled: true}
			backend.SetSessionAuthorizer(func(context.Context, *types.User, *types.Session, types.Action) error { return nil })
			backend.SetRepositoryAuthorizer(allowRepos("repo_unindexed"))

			req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/kodit?session_id="+tc.session.ID, nil)
			rec := httptest.NewRecorder()
			backend.ServeHTTP(rec, req, tc.user)
			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}

// mcpInitialize posts an MCP initialize request for sessionID as user.
func mcpInitialize(backend *KoditMCPBackend, sessionID string, user *types.User) *httptest.ResponseRecorder {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/kodit?session_id="+sessionID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, user)
	return rec
}

// The scope is resolved for each request and each caller: a second caller of
// the same session gets its own scope, and a change to the project (Kodit
// turned off) or to the caller's access applies to the next request.
func TestKoditMCPBackend_ScopeResolvedPerRequest(t *testing.T) {
	st := store.NewMockStore(gomock.NewController(t))
	session := &types.Session{ID: "ses_p", Owner: "usr_owner", OrganizationID: "org_1", ProjectID: "prj_1"}
	project := &types.Project{ID: "prj_1", KoditEnabled: true}
	st.EXPECT().GetSession(gomock.Any(), "ses_p").Return(session, nil).AnyTimes()
	st.EXPECT().GetProject(gomock.Any(), "prj_1").DoAndReturn(func(context.Context, string) (*types.Project, error) {
		p := *project
		return &p, nil
	}).AnyTimes()
	st.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: "prj_1"}).
		Return([]*types.GitRepository{indexedRepo("repo_1", 10)}, nil).AnyTimes()

	// The owner may read repo_1; the org owner reading the same session has
	// no access to it.
	backend := NewKoditMCPBackend(&kodit.Client{}, true, st)
	backend.SetSessionAuthorizer(func(context.Context, *types.User, *types.Session, types.Action) error { return nil })
	backend.SetRepositoryAuthorizer(func(_ context.Context, user *types.User, _ *types.GitRepository, _ types.Action) error {
		if user.ID == "usr_owner" {
			return nil
		}
		return errors.New("no access")
	})

	owner := &types.User{ID: "usr_owner"}
	other := &types.User{ID: "usr_org_owner"}

	rec := mcpInitialize(backend, "ses_p", owner)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = mcpInitialize(backend, "ses_p", other)
	assert.Equal(t, http.StatusForbidden, rec.Code, "a second caller must not reuse the first caller's scope")

	project.KoditEnabled = false
	rec = mcpInitialize(backend, "ses_p", owner)
	assert.Equal(t, http.StatusForbidden, rec.Code, "turning Kodit off must apply to the next request")
}

// With the server's repository authorizer, a key minted for a project keeps
// to that project's repositories even when the session's scope lists the
// whole organization, while an unscoped credential of the same owner sees
// both.
func TestResolveKoditRepoScope_ProjectKeyConfinedByRESTAuthorization(t *testing.T) {
	ctx := context.Background()
	session := &types.Session{ID: "ses_o", Owner: "usr_a", OrganizationID: "org_1", Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff"}}
	repos := []*types.GitRepository{indexedRepo("repo_attached", 10), indexedRepo("repo_other", 20)}
	for _, repo := range repos {
		repo.OrganizationID = "org_1"
		repo.OwnerID = "usr_a"
	}

	cases := []struct {
		name string
		user *types.User
		want []int64
	}{
		{"project-scoped key", &types.User{ID: "usr_a", ProjectID: "prj_1", SessionID: "ses_o"}, []int64{10}},
		{"unscoped credential", &types.User{ID: "usr_a"}, []int64{10, 20}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMockStore(gomock.NewController(t))
			st.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{OrganizationID: "org_1"}).Return(repos, nil)
			st.EXPECT().ListProjectRepositories(gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ context.Context, q *types.ListProjectRepositoriesQuery) ([]*types.ProjectRepository, error) {
					if q.ProjectID == "prj_1" && q.RepositoryID == "repo_attached" {
						return []*types.ProjectRepository{{ProjectID: "prj_1", RepositoryID: "repo_attached"}}, nil
					}
					return nil, nil
				}).AnyTimes()
			st.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
				Return(&types.OrganizationMembership{OrganizationID: "org_1", UserID: "usr_a", Role: types.OrganizationRoleMember}, nil).AnyTimes()

			srv := &HelixAPIServer{Store: st}
			scope, err := resolveKoditRepoScope(ctx, st, srv.authorizeUserToRepository, session, tc.user)
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.want, scope.idSlice)
		})
	}
}
