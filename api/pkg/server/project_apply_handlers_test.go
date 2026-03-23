package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ApplyProjectSuite struct {
	suite.Suite

	ctrl    *gomock.Controller
	store   *store.MockStore
	authCtx context.Context
	userID  string
	server  *HelixAPIServer
}

func TestApplyProjectSuite(t *testing.T) {
	suite.Run(t, new(ApplyProjectSuite))
}

func (s *ApplyProjectSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.userID = "user-apply-test"
	s.authCtx = setRequestUser(context.Background(), types.User{
		ID:    s.userID,
		Email: "apply@example.com",
	})
	s.server = &HelixAPIServer{
		Cfg:   &config.ServerConfig{},
		Store: s.store,
	}
}

// applyRequest builds a PUT /api/v1/projects/apply request with the given body.
func (s *ApplyProjectSuite) applyRequest(req types.ProjectApplyRequest) *http.Request {
	body, err := json.Marshal(req)
	s.Require().NoError(err)
	r := httptest.NewRequest(http.MethodPut, "/api/v1/projects/apply", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r.WithContext(s.authCtx)
}

// ---------------------------------------------------------------------------
// Create vs update
// ---------------------------------------------------------------------------

// TestApply_CreatesNewProject: no existing project with the same name → CreateProject is called.
func (s *ApplyProjectSuite) TestApply_CreatesNewProject() {
	req := types.ProjectApplyRequest{
		Name: "brand-new-project",
		Spec: types.ProjectSpec{
			Description: "A shiny new project",
		},
	}

	s.store.EXPECT().
		ListProjects(gomock.Any(), &store.ListProjectsQuery{UserID: s.userID}).
		Return([]*types.Project{}, nil)

	s.store.EXPECT().
		CreateProject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p *types.Project) (*types.Project, error) {
			s.Equal("brand-new-project", p.Name)
			s.Equal("A shiny new project", p.Description)
			s.Equal(s.userID, p.UserID)
			return p, nil
		})

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(httpErr)
	s.Require().NotNil(resp)
	s.True(resp.Created)
}

// TestApply_UpdatesExistingProject: project with matching name already exists → UpdateProject,
// same ID returned, Created=false.
func (s *ApplyProjectSuite) TestApply_UpdatesExistingProject() {
	existing := &types.Project{
		ID:     "proj-existing-123",
		Name:   "my-project",
		UserID: s.userID,
	}
	req := types.ProjectApplyRequest{
		Name: "my-project",
		Spec: types.ProjectSpec{
			Description: "Updated description",
			Guidelines:  "Use conventional commits",
		},
	}

	s.store.EXPECT().
		ListProjects(gomock.Any(), &store.ListProjectsQuery{UserID: s.userID}).
		Return([]*types.Project{existing}, nil)

	s.store.EXPECT().
		UpdateProject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p *types.Project) error {
			s.Equal("proj-existing-123", p.ID)
			s.Equal("Updated description", p.Description)
			s.Equal("Use conventional commits", p.Guidelines)
			return nil
		})

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(httpErr)
	s.Require().NotNil(resp)
	s.False(resp.Created)
	s.Equal("proj-existing-123", resp.ProjectID)
}

// ---------------------------------------------------------------------------
// Startup fields
// ---------------------------------------------------------------------------

// TestApply_StartsupFieldsMapped: startup block is written to the project.
func (s *ApplyProjectSuite) TestApply_StartupFieldsMapped() {
	req := types.ProjectApplyRequest{
		Name: "startup-project",
		Spec: types.ProjectSpec{
			Startup: &types.ProjectStartup{
				Install: "npm install",
				Start:   "npm start",
			},
		},
	}

	s.store.EXPECT().
		ListProjects(gomock.Any(), gomock.Any()).
		Return([]*types.Project{}, nil)

	s.store.EXPECT().
		CreateProject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p *types.Project) (*types.Project, error) {
			s.Equal("npm install", p.StartupInstall)
			s.Equal("npm start", p.StartupStart)
			return p, nil
		})

	rec := httptest.NewRecorder()
	_, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(httpErr)
}

// ---------------------------------------------------------------------------
// Kanban WIP limits
// ---------------------------------------------------------------------------

// TestApply_KanbanWIPLimitsMapped: kanban block is translated into project metadata.
func (s *ApplyProjectSuite) TestApply_KanbanWIPLimitsMapped() {
	req := types.ProjectApplyRequest{
		Name: "kanban-project",
		Spec: types.ProjectSpec{
			Kanban: &types.ProjectKanban{
				WIPLimits: &types.ProjectWIPLimits{
					Planning:       5,
					Implementation: 3,
					Review:         2,
				},
			},
		},
	}

	s.store.EXPECT().
		ListProjects(gomock.Any(), gomock.Any()).
		Return([]*types.Project{}, nil)

	s.store.EXPECT().
		CreateProject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p *types.Project) (*types.Project, error) {
			s.Require().NotNil(p.Metadata.BoardSettings)
			s.Equal(5, p.Metadata.BoardSettings.WIPLimits.Planning)
			s.Equal(3, p.Metadata.BoardSettings.WIPLimits.Implementation)
			s.Equal(2, p.Metadata.BoardSettings.WIPLimits.Review)
			return p, nil
		})

	rec := httptest.NewRecorder()
	_, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(httpErr)
}

// ---------------------------------------------------------------------------
// Task seeding
// ---------------------------------------------------------------------------

// TestApply_SeedsNewTasksOnly: when some tasks already exist, only the missing ones are created.
func (s *ApplyProjectSuite) TestApply_SeedsNewTasksOnly() {
	existing := &types.Project{ID: "proj-tasks-1", Name: "task-project", UserID: s.userID}
	req := types.ProjectApplyRequest{
		Name: "task-project",
		Spec: types.ProjectSpec{
			Tasks: []types.ProjectTaskSpec{
				{Title: "Already exists"},
				{Title: "New task", Description: "brand new"},
			},
		},
	}

	s.store.EXPECT().
		ListProjects(gomock.Any(), gomock.Any()).
		Return([]*types.Project{existing}, nil)
	s.store.EXPECT().UpdateProject(gomock.Any(), gomock.Any()).Return(nil)
	s.store.EXPECT().
		ListSpecTasks(gomock.Any(), &types.SpecTaskFilters{ProjectID: "proj-tasks-1"}).
		Return([]*types.SpecTask{{Name: "Already exists"}}, nil)

	// Only the new task should be created — exactly once.
	s.store.EXPECT().
		CreateSpecTask(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, t *types.SpecTask) error {
			s.Equal("New task", t.Name)
			s.Equal("brand new", t.Description)
			s.Equal(types.TaskStatusBacklog, t.Status)
			s.Equal("proj-tasks-1", t.ProjectID)
			s.Equal(s.userID, t.UserID)
			return nil
		})

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(httpErr)
	s.NotNil(resp)
}

// TestApply_TaskIdempotency_NoDuplicates: all tasks already exist — CreateSpecTask must not be called.
func (s *ApplyProjectSuite) TestApply_TaskIdempotency_NoDuplicates() {
	existing := &types.Project{ID: "proj-tasks-2", Name: "task-project", UserID: s.userID}
	req := types.ProjectApplyRequest{
		Name: "task-project",
		Spec: types.ProjectSpec{
			Tasks: []types.ProjectTaskSpec{
				{Title: "Task A"},
				{Title: "Task B"},
			},
		},
	}

	s.store.EXPECT().
		ListProjects(gomock.Any(), gomock.Any()).
		Return([]*types.Project{existing}, nil)
	s.store.EXPECT().UpdateProject(gomock.Any(), gomock.Any()).Return(nil)
	s.store.EXPECT().
		ListSpecTasks(gomock.Any(), gomock.Any()).
		Return([]*types.SpecTask{{Name: "Task A"}, {Name: "Task B"}}, nil)
	// CreateSpecTask must NOT be called — gomock fails the test if it is.

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(httpErr)
	s.NotNil(resp)
}

// ---------------------------------------------------------------------------
// Repository handling
// ---------------------------------------------------------------------------

// TestApply_RepoCreatedWhenNotFound: repository URL is not in the DB → create it, then attach.
func (s *ApplyProjectSuite) TestApply_RepoCreatedWhenNotFound() {
	req := types.ProjectApplyRequest{
		Name: "repo-project",
		Spec: types.ProjectSpec{
			Repository: &types.ProjectRepositorySpec{
				URL:           "https://github.com/org/my-repo",
				DefaultBranch: "main",
			},
		},
	}

	s.store.EXPECT().
		ListProjects(gomock.Any(), gomock.Any()).
		Return([]*types.Project{}, nil)
	s.store.EXPECT().
		CreateProject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p *types.Project) (*types.Project, error) { return p, nil })

	s.store.EXPECT().
		GetGitRepositoryByExternalURL(gomock.Any(), "", "https://github.com/org/my-repo").
		Return(nil, store.ErrNotFound)

	s.store.EXPECT().
		CreateGitRepository(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, r *types.GitRepository) error {
			s.Equal("https://github.com/org/my-repo", r.ExternalURL)
			s.Equal("main", r.DefaultBranch)
			s.True(r.IsExternal)
			return nil
		})

	s.store.EXPECT().AttachRepositoryToProject(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	s.store.EXPECT().SetProjectPrimaryRepository(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(httpErr)
	s.True(resp.Created)
}

// TestApply_RepoAttachedWhenAlreadyExists: repository URL already registered → attach without creating.
func (s *ApplyProjectSuite) TestApply_RepoAttachedWhenAlreadyExists() {
	existingRepo := &types.GitRepository{
		ID:          "repo-already-123",
		ExternalURL: "https://github.com/org/my-repo",
	}
	req := types.ProjectApplyRequest{
		Name: "repo-project",
		Spec: types.ProjectSpec{
			Repository: &types.ProjectRepositorySpec{URL: "https://github.com/org/my-repo"},
		},
	}

	s.store.EXPECT().
		ListProjects(gomock.Any(), gomock.Any()).
		Return([]*types.Project{}, nil)
	s.store.EXPECT().
		CreateProject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p *types.Project) (*types.Project, error) { return p, nil })

	// Repo found — CreateGitRepository must NOT be called.
	s.store.EXPECT().
		GetGitRepositoryByExternalURL(gomock.Any(), "", "https://github.com/org/my-repo").
		Return(existingRepo, nil)

	s.store.EXPECT().
		AttachRepositoryToProject(gomock.Any(), gomock.Any(), "repo-already-123").
		Return(nil)
	s.store.EXPECT().
		SetProjectPrimaryRepository(gomock.Any(), gomock.Any(), "repo-already-123").
		Return(nil)

	rec := httptest.NewRecorder()
	_, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(httpErr)
}

// TestApply_MultiRepo_PrimarySetCorrectly: in a multi-repo spec only the primary repo triggers
// SetProjectPrimaryRepository.
func (s *ApplyProjectSuite) TestApply_MultiRepo_PrimarySetCorrectly() {
	req := types.ProjectApplyRequest{
		Name: "multi-repo-project",
		Spec: types.ProjectSpec{
			Repositories: []types.ProjectRepositorySpec{
				{URL: "https://github.com/org/frontend", Primary: true},
				{URL: "https://github.com/org/backend"},
			},
		},
	}

	s.store.EXPECT().
		ListProjects(gomock.Any(), gomock.Any()).
		Return([]*types.Project{}, nil)
	s.store.EXPECT().
		CreateProject(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p *types.Project) (*types.Project, error) { return p, nil })

	frontendRepo := &types.GitRepository{ID: "repo-frontend"}
	backendRepo := &types.GitRepository{ID: "repo-backend"}

	s.store.EXPECT().
		GetGitRepositoryByExternalURL(gomock.Any(), "", "https://github.com/org/frontend").
		Return(frontendRepo, nil)
	s.store.EXPECT().
		AttachRepositoryToProject(gomock.Any(), gomock.Any(), "repo-frontend").Return(nil)
	// Frontend is primary → SetProjectPrimaryRepository called with its ID.
	s.store.EXPECT().
		SetProjectPrimaryRepository(gomock.Any(), gomock.Any(), "repo-frontend").Return(nil)

	s.store.EXPECT().
		GetGitRepositoryByExternalURL(gomock.Any(), "", "https://github.com/org/backend").
		Return(backendRepo, nil)
	s.store.EXPECT().
		AttachRepositoryToProject(gomock.Any(), gomock.Any(), "repo-backend").Return(nil)
	// Backend is NOT primary → SetProjectPrimaryRepository must NOT be called for it.

	rec := httptest.NewRecorder()
	_, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(httpErr)
}

// ---------------------------------------------------------------------------
// Validation errors
// ---------------------------------------------------------------------------

// TestApply_MissingName: name is required.
func (s *ApplyProjectSuite) TestApply_MissingName() {
	req := types.ProjectApplyRequest{
		Name: "",
		Spec: types.ProjectSpec{Description: "nameless"},
	}

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(resp)
	s.NotNil(httpErr)
	// No store calls expected — gomock will fail if any are made.
}

// TestApply_ValidationError_BothRepositoryFields: cannot set both singular and plural forms.
func (s *ApplyProjectSuite) TestApply_ValidationError_BothRepositoryFields() {
	req := types.ProjectApplyRequest{
		Name: "bad-spec",
		Spec: types.ProjectSpec{
			Repository:   &types.ProjectRepositorySpec{URL: "https://github.com/org/a"},
			Repositories: []types.ProjectRepositorySpec{{URL: "https://github.com/org/b"}},
		},
	}

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(resp)
	s.NotNil(httpErr)
}

// TestApply_ValidationError_NoPrimaryInMultiRepo: multi-repo without a primary designator.
func (s *ApplyProjectSuite) TestApply_ValidationError_NoPrimaryInMultiRepo() {
	req := types.ProjectApplyRequest{
		Name: "bad-repos",
		Spec: types.ProjectSpec{
			Repositories: []types.ProjectRepositorySpec{
				{URL: "https://github.com/org/a"},
				{URL: "https://github.com/org/b"},
			},
		},
	}

	rec := httptest.NewRecorder()
	resp, httpErr := s.server.applyProject(rec, s.applyRequest(req))
	s.Nil(resp)
	s.NotNil(httpErr)
}
