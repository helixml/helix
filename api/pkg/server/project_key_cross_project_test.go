package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// A project key for P1 held by a user who owns the organization, and so can
// reach P2 with a browser session, runs the real handlers against P2's
// objects. The middleware allowlist admits these routes; the handlers'
// authorization must still refuse them. The mock store has no write
// expectations, so any write fails the test.

const (
	xpOrg      = "org_xp"
	xpOwner    = "usr_xp_owner"
	xpP1       = "prj_xp_1"
	xpP2       = "prj_xp_2"
	xpTaskP1   = "spt_xp_1"
	xpTaskP2   = "spt_xp_2"
	xpSesP1    = "ses_xp_1"
	xpSesP2    = "ses_xp_2"
	xpSesChat  = "ses_xp_chat"
	xpRepoP1   = "repo_xp_1"
	xpRepoP2   = "repo_xp_2"
	xpRepoBare = "repo_xp_org"
)

type ProjectKeyCrossProjectSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
	key    *types.User
}

func TestProjectKeyCrossProjectSuite(t *testing.T) { suite.Run(t, new(ProjectKeyCrossProjectSuite)) }

func (s *ProjectKeyCrossProjectSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	repoBase := s.T().TempDir()
	for _, id := range []string{xpRepoP1, xpRepoP2, xpRepoBare} {
		s.Require().NoError(exec.Command("git", "init", "-q", "--bare", "--initial-branch=main", filepath.Join(repoBase, "git-repositories", id)).Run())
	}
	s.server = &HelixAPIServer{
		Store:                s.store,
		gitRepositoryService: services.NewGitRepositoryService(s.store, repoBase, "http://localhost:8080", "test", "test@example.com"),
	}
	s.key = &types.User{
		ID: xpOwner, Type: types.OwnerTypeUser, TokenType: types.TokenTypeAPIKey, Token: "hl-p1",
		APIKeyType: types.APIkeytypeAPI, OrganizationID: xpOrg, ProjectID: xpP1,
	}

	projects := map[string]*types.Project{
		xpP1: {ID: xpP1, OrganizationID: xpOrg, UserID: xpOwner},
		xpP2: {ID: xpP2, OrganizationID: xpOrg, UserID: xpOwner},
	}
	tasks := map[string]*types.SpecTask{
		xpTaskP1: {ID: xpTaskP1, ProjectID: xpP1, CreatedBy: xpOwner, Status: types.TaskStatusBacklog},
		xpTaskP2: {ID: xpTaskP2, ProjectID: xpP2, CreatedBy: xpOwner, Status: types.TaskStatusBacklog},
	}
	sessions := map[string]*types.Session{
		xpSesP1:   {ID: xpSesP1, Owner: xpOwner, OrganizationID: xpOrg, ProjectID: xpP1},
		xpSesP2:   {ID: xpSesP2, Owner: xpOwner, OrganizationID: xpOrg, ProjectID: xpP2},
		xpSesChat: {ID: xpSesChat, Owner: xpOwner, OrganizationID: xpOrg},
	}
	repos := map[string]*types.GitRepository{
		xpRepoP1:   {ID: xpRepoP1, OrganizationID: xpOrg, OwnerID: xpOwner, DefaultBranch: "main"},
		xpRepoP2:   {ID: xpRepoP2, OrganizationID: xpOrg, OwnerID: xpOwner, DefaultBranch: "main"},
		xpRepoBare: {ID: xpRepoBare, OrganizationID: xpOrg, OwnerID: xpOwner, DefaultBranch: "main"},
	}
	attached := map[string]string{xpRepoP1: xpP1, xpRepoP2: xpP2}

	s.store.EXPECT().GetProject(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.Project, error) {
			if p, ok := projects[id]; ok {
				c := *p
				return &c, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.SpecTask, error) {
			if t, ok := tasks[id]; ok {
				c := *t
				return &c, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	s.store.EXPECT().GetSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.Session, error) {
			if v, ok := sessions[id]; ok {
				c := *v
				return &c, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	s.store.EXPECT().GetGitRepository(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, id string) (*types.GitRepository, error) {
			if v, ok := repos[id]; ok {
				c := *v
				return &c, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()
	s.store.EXPECT().ListProjectRepositories(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, q *types.ListProjectRepositoriesQuery) ([]*types.ProjectRepository, error) {
			if attached[q.RepositoryID] == q.ProjectID {
				return []*types.ProjectRepository{{ProjectID: q.ProjectID, RepositoryID: q.RepositoryID}}, nil
			}
			return nil, nil
		}).AnyTimes()
	// The key's owner owns the organization: every owner shortcut is open.
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(
		&types.OrganizationMembership{OrganizationID: xpOrg, UserID: xpOwner, Role: types.OrganizationRoleOwner}, nil).AnyTimes()
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil).AnyTimes()
	s.store.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.store.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetSessionClaudeSubscription(gomock.Any(), gomock.Any()).Return(
		&types.ClaudeSubscription{ID: "csub", OwnerID: xpOwner, OwnerType: types.OwnerTypeUser}, nil).AnyTimes()
}

func (s *ProjectKeyCrossProjectSuite) call(handler http.HandlerFunc, method, target string, vars map[string]string, body string) int {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req = mux.SetURLVars(req, vars)
	req = req.WithContext(setTestRequestUser(req.Context(), s.key))
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr.Code
}

func (s *ProjectKeyCrossProjectSuite) requireRefused(code int) {
	s.T().Helper()
	s.True(code == http.StatusForbidden || code == http.StatusNotFound, "expected 403 or 404, got %d", code)
}

// The fixture does not refuse everything: the same key reaches its own
// project and objects.
func (s *ProjectKeyCrossProjectSuite) TestOwnProjectIsReachable() {
	s.Equal(http.StatusOK, s.call(s.server.getGitRepository, http.MethodGet, "/api/v1/git/repositories/"+xpRepoP1, map[string]string{"id": xpRepoP1}, ""))
	s.NoError(s.server.authorizeUserToProjectByID(context.Background(), s.key, xpP1, types.ActionGet))
	s.NoError(s.server.authorizeUserToSession(context.Background(), s.key, &types.Session{ID: xpSesP1, Owner: xpOwner, OrganizationID: xpOrg, ProjectID: xpP1}, types.ActionGet))
	repo, _ := s.store.GetGitRepository(context.Background(), xpRepoP1)
	s.NoError(s.server.authorizeUserToRepository(context.Background(), s.key, repo, types.ActionGet))
}

func (s *ProjectKeyCrossProjectSuite) TestOtherProjectByPath() {
	s.requireRefused(s.call(system.Wrapper(s.server.getProject), http.MethodGet, "/api/v1/projects/"+xpP2, map[string]string{"id": xpP2}, ""))
}

func (s *ProjectKeyCrossProjectSuite) TestOtherProjectInQueryOrBody() {
	s.requireRefused(s.call(s.server.listTasks, http.MethodGet, "/api/v1/spec-tasks?project_id="+xpP2, nil, ""))
	s.requireRefused(s.call(s.server.createTaskFromPrompt, http.MethodPost, "/api/v1/spec-tasks/from-prompt", nil,
		`{"project_id":"`+xpP2+`","prompt":"do it"}`))
	// Clone from its own project into P2, or into a new project.
	s.requireRefused(s.call(s.server.cloneSpecTask, http.MethodPost, "/api/v1/spec-tasks/"+xpTaskP1+"/clone",
		map[string]string{"taskId": xpTaskP1}, `{"target_project_ids":["`+xpP2+`"]}`))
	s.requireRefused(s.call(s.server.cloneSpecTask, http.MethodPost, "/api/v1/spec-tasks/"+xpTaskP1+"/clone",
		map[string]string{"taskId": xpTaskP1}, `{"create_projects":[{"repo_id":"`+xpRepoP1+`"}]}`))
}

func (s *ProjectKeyCrossProjectSuite) TestOtherProjectsSpecTaskByID() {
	s.requireRefused(s.call(s.server.getTask, http.MethodGet, "/api/v1/spec-tasks/"+xpTaskP2, map[string]string{"taskId": xpTaskP2}, ""))
	s.requireRefused(s.call(s.server.cloneSpecTask, http.MethodPost, "/api/v1/spec-tasks/"+xpTaskP2+"/clone",
		map[string]string{"taskId": xpTaskP2}, `{"target_project_ids":["`+xpP1+`"]}`))
}

func (s *ProjectKeyCrossProjectSuite) TestOtherProjectsSessionByID() {
	for _, id := range []string{xpSesP2, xpSesChat} {
		s.requireRefused(s.call(s.server.getSession, http.MethodGet, "/api/v1/sessions/"+id, map[string]string{"id": id}, ""))
		s.requireRefused(s.call(system.Wrapper(s.server.getSessionClaudeCredentials), http.MethodGet,
			"/api/v1/sessions/"+id+"/claude-credentials", map[string]string{"id": id}, ""))
	}
	// Agent subscription tokens belong to the session's sandbox, not to a
	// project key, even in its own project.
	s.requireRefused(s.call(system.Wrapper(s.server.getSessionClaudeCredentials), http.MethodGet,
		"/api/v1/sessions/"+xpSesP1+"/claude-credentials", map[string]string{"id": xpSesP1}, ""))
}

func (s *ProjectKeyCrossProjectSuite) TestOtherProjectsRepositoryByID() {
	for _, id := range []string{xpRepoP2, xpRepoBare} {
		s.requireRefused(s.call(s.server.getGitRepository, http.MethodGet, "/api/v1/git/repositories/"+id, map[string]string{"id": id}, ""))
	}
}
