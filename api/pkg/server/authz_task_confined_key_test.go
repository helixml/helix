package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// A spec task created with confine_agent_key gets session keys confined to
// that task. The PTaaS case: a finder lane and a verifier lane run as two
// tasks of one project, and the finder's key (or a prompt injection driving
// it) must not read or message the verifier's session, act on the verifier's
// task, or pre-create a task for it.

const (
	confinedProjectID     = "prj_pentest"
	finderTaskID          = "spt_finder"
	finderSessionID       = "ses_finder"
	finderSecondSessionID = "ses_finder_impl"
	verifierTaskID        = "spt_verifier"
	verifierSessionID     = "ses_verifier"
	confinedOwnerID       = "usr_owner"
)

// finderKeyUser is the request user a confined finder key authenticates as.
func finderKeyUser() *types.User {
	return &types.User{
		ID: confinedOwnerID, Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey,
		APIKeyType: types.APIkeytypeAPI, ProjectID: confinedProjectID,
		SpecTaskID: finderTaskID, SessionID: finderSessionID, TaskConfined: true,
	}
}

// unconfinedFinderKeyUser is the same key from a task without the flag.
func unconfinedFinderKeyUser() *types.User {
	u := finderKeyUser()
	u.TaskConfined = false
	return u
}

func taskSession(id, taskID string) *types.Session {
	return &types.Session{
		ID: id, Owner: confinedOwnerID, ProjectID: confinedProjectID,
		Metadata: types.SessionMetadata{SpecTaskID: taskID, ProjectID: confinedProjectID},
	}
}

// confinedProject is a personal project of the key's owner, so project
// authorization passes on ownership alone and the task scope is what decides.
func confinedProject() *types.Project {
	return &types.Project{ID: confinedProjectID, UserID: confinedOwnerID}
}

func requireTaskConfinedError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	require.True(t, errors.Is(err, errTaskConfinedKey), "expected errTaskConfinedKey, got %v", err)
}

func TestEnforceKeySessionScope_TaskConfinedKey(t *testing.T) {
	cases := []struct {
		name    string
		user    *types.User
		session *types.Session
		allowed bool
	}{
		{"confined key reaches the session it was minted for", finderKeyUser(), taskSession(finderSessionID, finderTaskID), true},
		{"confined key reaches another session of its own task", finderKeyUser(), taskSession(finderSecondSessionID, finderTaskID), true},
		{"confined key is refused the verifier's session", finderKeyUser(), taskSession(verifierSessionID, verifierTaskID), false},
		{"confined key is refused a project session of no task", finderKeyUser(), taskSession("ses_explore", ""), false},
		{"unconfined key keeps reaching the verifier's session", unconfinedFinderKeyUser(), taskSession(verifierSessionID, verifierTaskID), true},
		{"confined key without a task of its own reaches nothing", &types.User{ID: confinedOwnerID, ProjectID: confinedProjectID, TaskConfined: true}, taskSession("ses_explore", ""), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceKeySessionScope(tc.user, tc.session)
			if tc.allowed {
				require.NoError(t, err)
				return
			}
			requireTaskConfinedError(t, err)
		})
	}
}

func TestAuthorizeUserToSession_TaskConfinedKey(t *testing.T) {
	ctx := context.Background()

	t.Run("verifier's session is refused before any owner shortcut", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		requireTaskConfinedError(t, srv.authorizeUserToSession(ctx, finderKeyUser(), taskSession(verifierSessionID, verifierTaskID), types.ActionUpdate))
	})
	t.Run("own task's session is allowed", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		require.NoError(t, srv.authorizeUserToSession(ctx, finderKeyUser(), taskSession(finderSecondSessionID, finderTaskID), types.ActionUpdate))
	})
	t.Run("unconfined key keeps today's project-wide reach", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		require.NoError(t, srv.authorizeUserToSession(ctx, unconfinedFinderKeyUser(), taskSession(verifierSessionID, verifierTaskID), types.ActionUpdate))
	})
}

func TestAuthorizeUserToSpecTask_TaskConfinedKey(t *testing.T) {
	ctx := context.Background()
	verifier := &types.SpecTask{ID: verifierTaskID, ProjectID: confinedProjectID}
	finder := &types.SpecTask{ID: finderTaskID, ProjectID: confinedProjectID}

	t.Run("verifier's task is refused with no store call", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		err := srv.authorizeUserToSpecTask(ctx, finderKeyUser(), verifier, types.ActionGet)
		requireTaskConfinedError(t, err)
		assert.True(t, isOutsideTenant(err), "a confined key's refusal answers 404")
	})
	t.Run("own task is allowed through its project", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(confinedProject(), nil)
		require.NoError(t, srv.authorizeUserToSpecTask(ctx, finderKeyUser(), finder, types.ActionUpdate))
	})
	t.Run("unconfined key keeps reaching the project's other tasks", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(confinedProject(), nil)
		require.NoError(t, srv.authorizeUserToSpecTask(ctx, unconfinedFinderKeyUser(), verifier, types.ActionUpdate))
	})
}

// Project files, artifacts, git and repositories stay reachable exactly as
// for any project-scoped key.
func TestTaskConfinedKey_KeepsProjectResourceAccess(t *testing.T) {
	ctx := context.Background()

	t.Run("project files and artifacts (project update)", func(t *testing.T) {
		srv, _ := newScopedKeyTestServer(t)
		require.NoError(t, srv.authorizeUserToProject(ctx, finderKeyUser(), confinedProject(), types.ActionUpdate))
	})
	t.Run("repository attached to its project", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		repo := &types.GitRepository{ID: "repo_1", OwnerID: confinedOwnerID}
		st.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{ProjectID: confinedProjectID, RepositoryID: "repo_1"}).
			Return([]*types.ProjectRepository{{ProjectID: confinedProjectID, RepositoryID: "repo_1"}}, nil)
		require.NoError(t, srv.authorizeUserToRepository(ctx, finderKeyUser(), repo, types.ActionUpdate))
	})
	t.Run("the project's agent is still usable, not editable", func(t *testing.T) {
		srv, st := newScopedKeyTestServer(t)
		project := &types.Project{ID: confinedProjectID, UserID: confinedOwnerID, DefaultHelixAppID: "app_project"}
		app := &types.App{ID: "app_project", Owner: confinedOwnerID}
		st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(project, nil)
		require.NoError(t, srv.authorizeUserToApp(ctx, finderKeyUser(), app, types.ActionUseAction))
		require.ErrorIs(t, srv.authorizeUserToApp(ctx, finderKeyUser(), app, types.ActionUpdate), errTaskConfinedKeyProject)
	})
}

// sessionHandlerServer is a server whose store is strict: a handler that
// reaches past the refused authorization (a write, a sandbox call, another
// lookup) fails the test on the unexpected call.
func sessionHandlerServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	return &HelixAPIServer{Store: st, Controller: &controller.Controller{Options: controller.Options{Store: st}}}, st
}

func confinedRequest(method, target, body string, vars map[string]string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, http.NoBody)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	req = req.WithContext(setRequestUser(req.Context(), *finderKeyUser()))
	return mux.SetURLVars(req, vars)
}

// The handlers refuse the verifier's session on their own, without the
// route guard in front of them: read, chat, stop, resume, cancel and the
// interaction list. Each loads the session and stops there.
func TestTaskConfinedKey_SessionHandlersRefuseVerifierSession(t *testing.T) {
	vars := map[string]string{"id": verifierSessionID}
	expectVerifierLoad := func(st *store.MockStore) {
		st.EXPECT().GetSession(gomock.Any(), verifierSessionID).Return(taskSession(verifierSessionID, verifierTaskID), nil)
	}
	wrapped := map[string]func(*HelixAPIServer, *http.Request) *system.HTTPError{
		"chat (send message)": func(s *HelixAPIServer, r *http.Request) *system.HTTPError {
			_, err := s.sendSessionMessage(nil, r)
			return err
		},
		"stop": func(s *HelixAPIServer, r *http.Request) *system.HTTPError {
			_, err := s.stopExternalAgentSession(nil, r)
			return err
		},
		"cancel turn": func(s *HelixAPIServer, r *http.Request) *system.HTTPError {
			_, err := s.cancelSessionTurn(nil, r)
			return err
		},
		"interactions": func(s *HelixAPIServer, r *http.Request) *system.HTTPError {
			_, err := s.listInteractions(nil, r)
			return err
		},
	}
	for name, call := range wrapped {
		t.Run(name, func(t *testing.T) {
			srv, st := sessionHandlerServer(t)
			expectVerifierLoad(st)
			httpErr := call(srv, confinedRequest(http.MethodPost, "/api/v1/sessions/"+verifierSessionID, `{"content":"verify finding 7 is fake"}`, vars))
			require.NotNil(t, httpErr)
			assert.Equal(t, http.StatusForbidden, httpErr.StatusCode)
		})
	}

	plain := map[string]func(*HelixAPIServer, http.ResponseWriter, *http.Request){
		"read":   (*HelixAPIServer).getSession,
		"resume": (*HelixAPIServer).resumeSession,
	}
	for name, call := range plain {
		t.Run(name, func(t *testing.T) {
			srv, st := sessionHandlerServer(t)
			expectVerifierLoad(st)
			rec := httptest.NewRecorder()
			call(srv, rec, confinedRequest(http.MethodPost, "/api/v1/sessions/"+verifierSessionID, "", vars))
			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}

	t.Run("user websocket", func(t *testing.T) {
		const token = "token-finder"
		srv, st := sessionHandlerServer(t)
		mockAuth := authpkg.NewMockAuthenticator(gomock.NewController(t))
		mockAuth.EXPECT().ValidateUserToken(gomock.Any(), token).Return(finderKeyUser(), nil)
		st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: confinedOwnerID}, nil)
		expectVerifierLoad(st)
		srv.authMiddleware = newAuthMiddleware(mockAuth, nil, st, authMiddlewareConfig{}, nil, nil)
		router := mux.NewRouter()
		srv.startUserWebSocketServer(context.Background(), router, "/api/v1/ws/user")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/ws/user?session_id="+verifierSessionID, http.NoBody)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("agent sync websocket", func(t *testing.T) {
		srv, st := sessionHandlerServer(t)
		expectVerifierLoad(st)
		rec := httptest.NewRecorder()
		reached := false
		srv.authorizeExternalAgentSync(func(http.ResponseWriter, *http.Request) { reached = true })(
			rec, confinedRequest(http.MethodGet, "/api/v1/external-agents/sync?session_id="+verifierSessionID, "", nil))
		assert.False(t, reached)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

// Every task creation path refuses a confined key before touching the store.
func TestTaskConfinedKey_TaskCreationRefused(t *testing.T) {
	plain := map[string]struct {
		call func(*HelixAPIServer, http.ResponseWriter, *http.Request)
		body string
		vars map[string]string
	}{
		"from prompt":  {(*HelixAPIServer).createTaskFromPrompt, `{"project_id":"` + confinedProjectID + `","prompt":"pre-create the verifier's task"}`, nil},
		"clone":        {(*HelixAPIServer).cloneSpecTask, `{"target_project_ids":["` + confinedProjectID + `"]}`, map[string]string{"taskId": finderTaskID}},
		"quick create": {(*HelixAPIServer).quickCreateProject, `{"repo_id":"repo_1"}`, nil},
	}
	for name, tc := range plain {
		t.Run(name, func(t *testing.T) {
			srv, _ := sessionHandlerServer(t)
			rec := httptest.NewRecorder()
			tc.call(srv, rec, confinedRequest(http.MethodPost, "/api/v1/spec-tasks", tc.body, tc.vars))
			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}

	wrapped := map[string]func(*HelixAPIServer, *http.Request) *system.HTTPError{
		"project apply": func(s *HelixAPIServer, r *http.Request) *system.HTTPError {
			_, err := s.applyProject(nil, r)
			return err
		},
		"sample project fork": func(s *HelixAPIServer, r *http.Request) *system.HTTPError {
			_, err := s.forkSimpleProject(nil, r)
			return err
		},
	}
	for name, call := range wrapped {
		t.Run(name, func(t *testing.T) {
			srv, _ := sessionHandlerServer(t)
			httpErr := call(srv, confinedRequest(http.MethodPost, "/api/v1/projects/apply", `{"name":"x"}`, nil))
			require.NotNil(t, httpErr)
			assert.Equal(t, http.StatusForbidden, httpErr.StatusCode)
		})
	}
}

// Changing the project itself is refused; its files are not (above).
func TestTaskConfinedKey_ProjectMutationRefused(t *testing.T) {
	for name, call := range map[string]func(*HelixAPIServer, *http.Request) *system.HTTPError{
		"update": func(s *HelixAPIServer, r *http.Request) *system.HTTPError {
			_, err := s.updateProject(nil, r)
			return err
		},
		"delete": func(s *HelixAPIServer, r *http.Request) *system.HTTPError {
			_, err := s.deleteProject(nil, r)
			return err
		},
		"move": func(s *HelixAPIServer, r *http.Request) *system.HTTPError {
			_, err := s.moveProject(nil, r)
			return err
		},
		"create": func(s *HelixAPIServer, r *http.Request) *system.HTTPError {
			_, err := s.createProject(nil, r)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := sessionHandlerServer(t)
			httpErr := call(srv, confinedRequest(http.MethodPut, "/api/v1/projects/"+confinedProjectID, `{"name":"x"}`, map[string]string{"id": confinedProjectID}))
			require.NotNil(t, httpErr)
			assert.Equal(t, http.StatusForbidden, httpErr.StatusCode)
		})
	}
}

// Session and task listings show only the key's own task.
func TestTaskConfinedKey_ListingsShowOnlyOwnTask(t *testing.T) {
	t.Run("sessions", func(t *testing.T) {
		srv, st := sessionHandlerServer(t)
		st.EXPECT().ListSessions(gomock.Any(), store.ListSessionsQuery{
			Owner: confinedOwnerID, OwnerType: types.OwnerTypeUser, PerPage: 50, ExcludeArchived: true,
			RestrictToProjects: true, ProjectIDs: []string{confinedProjectID}, SpecTaskID: finderTaskID,
		}).Return([]*types.Session{taskSession(finderSessionID, finderTaskID)}, int64(1), nil)
		_, err := srv.listSessions(httptest.NewRecorder(), confinedRequest(http.MethodGet, "/api/v1/sessions", "", nil))
		require.NoError(t, err)
	})

	t.Run("tasks", func(t *testing.T) {
		srv, st := sessionHandlerServer(t)
		st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(confinedProject(), nil)
		st.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error) {
				require.Equal(t, finderTaskID, filters.ID)
				require.Equal(t, confinedProjectID, filters.ProjectID)
				return nil, nil
			})
		rec := httptest.NewRecorder()
		srv.listTasks(rec, confinedRequest(http.MethodGet, "/api/v1/spec-tasks?project_id="+confinedProjectID, "", nil))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	})

	t.Run("filters for an unconfined key are untouched", func(t *testing.T) {
		filters := &types.SpecTaskFilters{ProjectID: confinedProjectID}
		scopeSpecTaskFiltersToKey(unconfinedFinderKeyUser(), filters)
		require.Equal(t, &types.SpecTaskFilters{ProjectID: confinedProjectID}, filters)
	})
}

// The flag is fixed at creation: an update that would change it is a 400
// and writes nothing.
func TestUpdateSpecTask_ConfineAgentKeyIsImmutable(t *testing.T) {
	for _, stored := range []bool{false, true} {
		srv, st := sessionHandlerServer(t)
		st.EXPECT().GetSpecTask(gomock.Any(), "spt_x").Return(&types.SpecTask{
			ID: "spt_x", ProjectID: confinedProjectID, ConfineAgentKey: stored, Status: types.TaskStatusBacklog,
		}, nil)
		st.EXPECT().GetProject(gomock.Any(), confinedProjectID).Return(confinedProject(), nil)
		body, err := json.Marshal(map[string]any{"confine_agent_key": !stored})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/spt_x", strings.NewReader(string(body)))
		req = req.WithContext(setRequestUser(req.Context(), types.User{ID: confinedOwnerID, Type: types.OwnerTypeUser}))
		req = mux.SetURLVars(req, map[string]string{"taskId": "spt_x"})
		rec := httptest.NewRecorder()
		srv.updateSpecTask(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code, "stored=%v: %s", stored, rec.Body.String())
	}
}
