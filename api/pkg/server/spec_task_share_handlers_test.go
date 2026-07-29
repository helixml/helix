package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// PublicShareViewerSuite covers the unauthenticated design-docs share link
// (GET /api/v1/spec-tasks/{id}/view).
//
// Regression context: the viewer used to render from the SpecTask columns
// RequirementsSpec/TechnicalDesign/ImplementationPlan, which are only ever
// written by the legacy (dead) HandleSpecGenerationComplete path. The live
// git-push pipeline writes doc content to the SpecTaskDesignReview record
// instead, so every shared link rendered three empty sections. These tests
// pin the viewer to the design-review record.
type PublicShareViewerSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestPublicShareViewerSuite(t *testing.T) {
	suite.Run(t, new(PublicShareViewerSuite))
}

func (s *PublicShareViewerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{URL: "http://localhost:0", Host: "localhost"},
		},
		Store: s.store,
	}
}

func (s *PublicShareViewerSuite) TearDownTest() {
	s.ctrl.Finish()
}

// serveView drives the handler through a router so mux vars are populated.
func (s *PublicShareViewerSuite) serveView(taskID string) *httptest.ResponseRecorder {
	router := mux.NewRouter()
	router.HandleFunc("/spec-tasks/{id}/view", s.server.viewDesignDocsPublic).Methods(http.MethodGet)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/spec-tasks/"+taskID+"/view", nil)
	router.ServeHTTP(rec, req)
	return rec
}

func publicTask(id string) *types.SpecTask {
	return &types.SpecTask{
		ID:                 id,
		Name:               "Test Task",
		ProjectID:          "prj_test",
		Status:             types.TaskStatusSpecReview,
		OriginalPrompt:     "do the thing",
		PublicDesignDocs:   true,
		UpdatedAt:          time.Now(),
		RequirementsSpec:   "", // deliberately empty — the live flow never fills these
		TechnicalDesign:    "",
		ImplementationPlan: "",
	}
}

// TestRendersContentFromDesignReview is the core regression test: the task
// columns are empty (as they always are in the real flow) but the design
// review holds the docs. The page must render the review's content.
func (s *PublicShareViewerSuite) TestRendersContentFromDesignReview() {
	task := publicTask("task-1")

	s.store.EXPECT().GetSpecTask(gomock.Any(), "task-1").Return(task, nil)
	s.store.EXPECT().ListSpecTaskDesignReviews(gomock.Any(), "task-1").Return([]types.SpecTaskDesignReview{
		{
			ID:                 "rev-1",
			SpecTaskID:         "task-1",
			Status:             types.SpecTaskDesignReviewStatusPending,
			RequirementsSpec:   "# Requirements Heading\n\nrequirements body text",
			TechnicalDesign:    "# Design Heading\n\ntechnical design body text",
			ImplementationPlan: "# Tasks Heading\n\nimplementation plan body text",
		},
	}, nil)

	rec := s.serveView("task-1")

	s.Equal(http.StatusOK, rec.Code)
	body := rec.Body.String()
	s.Contains(body, "requirements body text")
	s.Contains(body, "technical design body text")
	s.Contains(body, "implementation plan body text")
	// Markdown should be rendered to HTML, not dumped raw.
	s.Contains(body, "<h1>Requirements Heading</h1>")
	// Task-level metadata still comes from the task row.
	s.Contains(body, "Test Task")
	s.Contains(body, "do the thing")
}

// TestPrefersNonSupersededReview ensures a superseded (older revision) review
// does not shadow the current one. The store returns created_at DESC, so a
// superseded review can legitimately sort first.
func (s *PublicShareViewerSuite) TestPrefersNonSupersededReview() {
	task := publicTask("task-2")

	s.store.EXPECT().GetSpecTask(gomock.Any(), "task-2").Return(task, nil)
	s.store.EXPECT().ListSpecTaskDesignReviews(gomock.Any(), "task-2").Return([]types.SpecTaskDesignReview{
		{
			ID:               "rev-old",
			Status:           types.SpecTaskDesignReviewStatusSuperseded,
			RequirementsSpec: "STALE superseded content",
		},
		{
			ID:               "rev-current",
			Status:           types.SpecTaskDesignReviewStatusPending,
			RequirementsSpec: "CURRENT active content",
		},
	}, nil)

	rec := s.serveView("task-2")

	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), "CURRENT active content")
	s.NotContains(rec.Body.String(), "STALE superseded content")
}

// TestNoContentRendersUnavailablePage guards against silently blank sections
// when neither a review nor git has content.
func (s *PublicShareViewerSuite) TestNoContentRendersUnavailablePage() {
	task := publicTask("task-3")
	task.DesignDocsPushedAt = nil // nothing pushed, so no git backfill attempt

	s.store.EXPECT().GetSpecTask(gomock.Any(), "task-3").Return(task, nil)
	s.store.EXPECT().ListSpecTaskDesignReviews(gomock.Any(), "task-3").Return([]types.SpecTaskDesignReview{}, nil)

	rec := s.serveView("task-3")

	s.Equal(http.StatusNotFound, rec.Code)
	s.Contains(rec.Body.String(), "Design documents not available yet")
}

// TestEmptyReviewTriggersGitBackfill verifies the self-healing path: docs were
// pushed to git but no review row holds content, so the viewer looks up the
// project repo to backfill.
func (s *PublicShareViewerSuite) TestEmptyReviewTriggersGitBackfill() {
	pushedAt := time.Now()
	task := publicTask("task-4")
	task.DesignDocsPushedAt = &pushedAt

	s.store.EXPECT().GetSpecTask(gomock.Any(), "task-4").Return(task, nil)
	// First read: nothing. Then backfill runs, then we re-read.
	s.store.EXPECT().ListSpecTaskDesignReviews(gomock.Any(), "task-4").
		Return([]types.SpecTaskDesignReview{}, nil)
	// Backfill looks up the project to find the repo.
	s.store.EXPECT().GetProject(gomock.Any(), "prj_test").
		Return(&types.Project{ID: "prj_test", DefaultRepoID: ""}, nil)

	rec := s.serveView("task-4")

	// No repo configured, so backfill can't run and we fall through to the
	// unavailable page rather than rendering blank sections.
	s.Equal(http.StatusNotFound, rec.Code)
	s.Contains(rec.Body.String(), "Design documents not available yet")
}

// TestPrivateTaskRendersPrivatePage confirms the access guard is unchanged.
func (s *PublicShareViewerSuite) TestPrivateTaskRendersPrivatePage() {
	task := publicTask("task-5")
	task.PublicDesignDocs = false

	s.store.EXPECT().GetSpecTask(gomock.Any(), "task-5").Return(task, nil)
	// Must NOT touch design reviews for a private task.

	rec := s.serveView("task-5")

	s.Equal(http.StatusOK, rec.Code)
	s.Contains(rec.Body.String(), "This spec task is private")
}

// TestSpecsNotGeneratedReturns404 confirms the status guard is unchanged.
func (s *PublicShareViewerSuite) TestSpecsNotGeneratedReturns404() {
	task := publicTask("task-6")
	task.Status = types.TaskStatusBacklog

	s.store.EXPECT().GetSpecTask(gomock.Any(), "task-6").Return(task, nil)

	rec := s.serveView("task-6")

	s.Equal(http.StatusNotFound, rec.Code)
	s.Contains(rec.Body.String(), "specifications not yet generated")
}
