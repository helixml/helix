package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// Regression tests for project quick-create and spec-task cloning: neither
// checked that the caller was authorized on the repository, source task or
// target projects involved, so a user could write into another org.
type SpecTaskCloneAuthzSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer

	alice *types.User // member of org_a
	bob   *types.User // member of org_b
}

func TestSpecTaskCloneAuthzSuite(t *testing.T) {
	suite.Run(t, new(SpecTaskCloneAuthzSuite))
}

func (s *SpecTaskCloneAuthzSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{Store: s.store}
	s.alice = &types.User{ID: "usr_alice"}
	s.bob = &types.User{ID: "usr_bob"}

	memberships := map[string]string{"usr_alice": "org_a", "usr_bob": "org_b"}
	s.store.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
			if memberships[q.UserID] != q.OrganizationID {
				return nil, store.ErrNotFound
			}
			return &types.OrganizationMembership{OrganizationID: q.OrganizationID, UserID: q.UserID, Role: types.OrganizationRoleMember}, nil
		}).AnyTimes()

	projects := map[string]*types.Project{
		"prj_a": {ID: "prj_a", Name: "proj-a", OrganizationID: "org_a", UserID: "usr_alice"},
		"prj_b": {ID: "prj_b", Name: "proj-b", OrganizationID: "org_b", UserID: "usr_bob"},
	}
	s.store.EXPECT().GetProject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id string) (*types.Project, error) {
			if p, ok := projects[id]; ok {
				return p, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()

	repos := map[string]*types.GitRepository{
		"repo_a": {ID: "repo_a", Name: "repo-a", OrganizationID: "org_a", OwnerID: "usr_alice"},
		"repo_b": {ID: "repo_b", Name: "repo-b", OrganizationID: "org_b", OwnerID: "usr_bob"},
	}
	s.store.EXPECT().GetGitRepository(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id string) (*types.GitRepository, error) {
			if r, ok := repos[id]; ok {
				return r, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()

	tasks := map[string]*types.SpecTask{
		"spt_a": {ID: "spt_a", ProjectID: "prj_a", OrganizationID: "org_a", Name: "task-a", OriginalPrompt: "secret"},
		"spt_b": {ID: "spt_b", ProjectID: "prj_b", OrganizationID: "org_b", Name: "task-b"},
	}
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, id string) (*types.SpecTask, error) {
			if t, ok := tasks[id]; ok {
				return t, nil
			}
			return nil, store.ErrNotFound
		}).AnyTimes()

	s.store.EXPECT().GetCloneGroup(gomock.Any(), "clg_a").
		Return(&types.CloneGroup{ID: "clg_a", SourceTaskID: "spt_a", SourceProjectID: "prj_a", CreatedBy: "usr_alice"}, nil).AnyTimes()
}

// expectNoWrites fails the test if the handler persists anything.
func (s *SpecTaskCloneAuthzSuite) expectNoWrites() {
	s.store.EXPECT().CreateProject(gomock.Any(), gomock.Any()).Times(0)
	s.store.EXPECT().AttachRepositoryToProject(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	s.store.EXPECT().CreateCloneGroup(gomock.Any(), gomock.Any()).Times(0)
	s.store.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).Times(0)
}

func (s *SpecTaskCloneAuthzSuite) do(handler http.HandlerFunc, user *types.User, method, url string, vars map[string]string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		s.Require().NoError(json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, url, &buf)
	req = req.WithContext(setTestRequestUser(req.Context(), user))
	req = mux.SetURLVars(req, vars)
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func (s *SpecTaskCloneAuthzSuite) quickCreate(user *types.User, repoID string) *httptest.ResponseRecorder {
	return s.do(s.server.quickCreateProject, user, http.MethodPost, "/api/v1/projects/quick-create", nil,
		QuickCreateProjectRequest{RepoID: repoID})
}

func (s *SpecTaskCloneAuthzSuite) clone(user *types.User, taskID string, req types.CloneTaskRequest) *httptest.ResponseRecorder {
	return s.do(s.server.cloneSpecTask, user, http.MethodPost, "/api/v1/spec-tasks/"+taskID+"/clone",
		map[string]string{"taskId": taskID}, req)
}

func (s *SpecTaskCloneAuthzSuite) TestQuickCreateForeignRepoIsForbidden() {
	s.expectNoWrites()

	rr := s.quickCreate(s.bob, "repo_a")

	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *SpecTaskCloneAuthzSuite) TestQuickCreateMissingRepoIsNotFound() {
	s.expectNoWrites()

	rr := s.quickCreate(s.bob, "repo_missing")

	s.Equal(http.StatusNotFound, rr.Code)
}

func (s *SpecTaskCloneAuthzSuite) TestQuickCreateOwnRepoCreatesProjectInRepoOrg() {
	s.store.EXPECT().CreateProject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p *types.Project) (*types.Project, error) { return p, nil })
	s.store.EXPECT().AttachRepositoryToProject(gomock.Any(), gomock.Any(), "repo_a").Return(nil)

	rr := s.quickCreate(s.alice, "repo_a")

	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	var project types.Project
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &project))
	s.Equal("org_a", project.OrganizationID)
	s.Equal("usr_alice", project.UserID)
}

func (s *SpecTaskCloneAuthzSuite) TestCloneForeignSourceTaskIsForbidden() {
	s.expectNoWrites()

	rr := s.clone(s.bob, "spt_a", types.CloneTaskRequest{TargetProjectIDs: []string{"prj_b"}})

	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *SpecTaskCloneAuthzSuite) TestCloneIntoForeignProjectIsForbidden() {
	s.expectNoWrites()

	rr := s.clone(s.bob, "spt_b", types.CloneTaskRequest{TargetProjectIDs: []string{"prj_a"}})

	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *SpecTaskCloneAuthzSuite) TestCloneWithAnyForeignTargetWritesNothing() {
	s.expectNoWrites()

	rr := s.clone(s.bob, "spt_b", types.CloneTaskRequest{
		TargetProjectIDs: []string{"prj_b"},
		CreateProjects:   []types.CloneTaskCreateProjectSpec{{RepoID: "repo_a"}},
	})

	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *SpecTaskCloneAuthzSuite) TestCloneOwnTaskIntoOwnProjectSucceeds() {
	s.store.EXPECT().GetLatestDesignReview(gomock.Any(), "spt_a").Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().CreateCloneGroup(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, g *types.CloneGroup) (*types.CloneGroup, error) {
			g.ID = "clg_new"
			return g, nil
		})
	s.store.EXPECT().IncrementGlobalTaskNumber(gomock.Any()).Return(7, nil)
	s.store.EXPECT().CreateSpecTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, t *types.SpecTask) error {
			s.Equal("prj_a", t.ProjectID)
			s.Equal("org_a", t.OrganizationID)
			return nil
		})

	rr := s.clone(s.alice, "spt_a", types.CloneTaskRequest{TargetProjectIDs: []string{"prj_a"}})

	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	var resp types.CloneTaskResponse
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &resp))
	s.Equal(1, resp.TotalCloned)
	s.Equal(0, resp.TotalFailed)
}

func (s *SpecTaskCloneAuthzSuite) TestListCloneGroupsForeignTaskIsForbidden() {
	s.store.EXPECT().ListCloneGroupsForTask(gomock.Any(), gomock.Any()).Times(0)

	rr := s.do(s.server.listCloneGroups, s.bob, http.MethodGet, "/api/v1/spec-tasks/spt_a/clone-groups",
		map[string]string{"taskId": "spt_a"}, nil)

	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *SpecTaskCloneAuthzSuite) TestListCloneGroupsOwnTask() {
	s.store.EXPECT().ListCloneGroupsForTask(gomock.Any(), "spt_a").
		Return([]*types.CloneGroup{{ID: "clg_a", SourceTaskID: "spt_a"}}, nil)

	rr := s.do(s.server.listCloneGroups, s.alice, http.MethodGet, "/api/v1/spec-tasks/spt_a/clone-groups",
		map[string]string{"taskId": "spt_a"}, nil)

	s.Equal(http.StatusOK, rr.Code)
}

func (s *SpecTaskCloneAuthzSuite) TestCloneGroupProgressForeignGroupIsForbidden() {
	s.store.EXPECT().GetCloneGroupProgress(gomock.Any(), gomock.Any()).Times(0)

	rr := s.do(s.server.getCloneGroupProgress, s.bob, http.MethodGet, "/api/v1/clone-groups/clg_a/progress",
		map[string]string{"groupId": "clg_a"}, nil)

	s.Equal(http.StatusForbidden, rr.Code)
}

func (s *SpecTaskCloneAuthzSuite) TestCloneGroupProgressHidesUnreadableTargets() {
	readable := types.SpecTask{ID: "spt_a2", ProjectID: "prj_a", Status: types.TaskStatusDone}
	hidden := types.SpecTask{ID: "spt_b2", ProjectID: "prj_b", Status: types.TaskStatusBacklog}
	s.store.EXPECT().GetCloneGroupProgress(gomock.Any(), "clg_a").Return(&types.CloneGroupProgress{
		CloneGroupID: "clg_a",
		Tasks: []types.CloneGroupTaskProgress{
			{TaskID: readable.ID, ProjectID: readable.ProjectID, Status: readable.Status.String()},
			{TaskID: hidden.ID, ProjectID: hidden.ProjectID, Status: hidden.Status.String()},
		},
		FullTasks:       []types.SpecTaskWithProject{{SpecTask: readable}, {SpecTask: hidden}},
		TotalTasks:      2,
		CompletedTasks:  1,
		ProgressPct:     50,
		StatusBreakdown: map[string]int{readable.Status.String(): 1, hidden.Status.String(): 1},
	}, nil)

	rr := s.do(s.server.getCloneGroupProgress, s.alice, http.MethodGet, "/api/v1/clone-groups/clg_a/progress",
		map[string]string{"groupId": "clg_a"}, nil)

	s.Require().Equal(http.StatusOK, rr.Code, rr.Body.String())
	var progress types.CloneGroupProgress
	s.Require().NoError(json.Unmarshal(rr.Body.Bytes(), &progress))
	s.Require().Len(progress.Tasks, 1)
	s.Equal("spt_a2", progress.Tasks[0].TaskID)
	s.Require().Len(progress.FullTasks, 1)
	s.Equal("spt_a2", progress.FullTasks[0].ID)
	s.Equal(1, progress.TotalTasks)
	s.Equal(100, progress.ProgressPct)
	s.Equal(map[string]int{readable.Status.String(): 1}, progress.StatusBreakdown)
}
