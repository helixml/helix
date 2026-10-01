package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

func TestUpdateDesignReviewDocument(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:            "spt_1",
		ProjectID:     "prj_1",
		CreatedBy:     "usr_1",
		DesignDocPath: "2026-09-14_edit-plan_1",
		Name:          "Old title",
	}
	review := &types.SpecTaskDesignReview{
		ID:               "stdr_1",
		SpecTaskID:       task.ID,
		Status:           types.SpecTaskDesignReviewStatusInReview,
		RequirementsSpec: "# Requirements: Old title\n",
		GitCommitHash:    "old-sha",
	}
	project := &types.Project{ID: task.ProjectID, DefaultRepoID: "repo_1"}
	repo := &types.GitRepository{ID: project.DefaultRepoID}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil).Times(2)
	mockStore.EXPECT().GetSpecTaskDesignReview(gomock.Any(), review.ID).Return(review, nil).Times(2)
	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	mockStore.EXPECT().UpdateSpecTaskDesignReviewDocument(
		gomock.Any(), review.ID, task.ID, gomock.Any(), gomock.Any(),
	).DoAndReturn(func(_ context.Context, _, _ string, reviewUpdates, taskUpdates map[string]any) error {
		require.Equal(t, "new-sha", reviewUpdates["git_commit_hash"])
		require.Equal(t, "# Requirements: New title\n\nUpdated.", reviewUpdates["requirements_spec"])
		require.Equal(t, "# Requirements: New title\n\nUpdated.", taskUpdates["requirements_spec"])
		require.Equal(t, "New title", taskUpdates["name"])
		return nil
	})

	var writtenPath string
	var writtenContent string
	gitService := &fakeGitRepoService{
		getRepositoryFunc: func(_ context.Context, repoID string) (*types.GitRepository, error) {
			require.Equal(t, repo.ID, repoID)
			return repo, nil
		},
		withRepoLockFunc: func(repoID string, fn func() error) error {
			require.Equal(t, repo.ID, repoID)
			return fn()
		},
		getFileContentsFunc: func(_ context.Context, repoID, path, branch string) (string, error) {
			require.Equal(t, repo.ID, repoID)
			require.Equal(t, "helix-specs", branch)
			return review.RequirementsSpec, nil
		},
		writeFileContentsFunc: func(_ context.Context, repoID, path, branch string, content []byte, message, authorName, authorEmail string) (string, error) {
			require.Equal(t, repo.ID, repoID)
			require.Equal(t, "helix-specs", branch)
			require.Equal(t, "docs(specs): update requirements.md from review", message)
			require.Equal(t, "Reviewer", authorName)
			require.Equal(t, "reviewer@example.com", authorEmail)
			writtenPath = path
			writtenContent = string(content)
			return "new-sha", nil
		},
	}

	body, err := json.Marshal(types.SpecTaskDesignReviewDocumentUpdateRequest{
		DocumentType:    "requirements",
		Content:         "# Requirements: New title\n\nUpdated.",
		OriginalContent: func() *string { value := review.RequirementsSpec; return &value }(),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/spt_1/design-reviews/stdr_1/document", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"spec_task_id": task.ID, "review_id": review.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{
		ID:       task.CreatedBy,
		FullName: "Reviewer",
		Email:    "reviewer@example.com",
	}))
	response := httptest.NewRecorder()

	server := &HelixAPIServer{Store: mockStore, gitRepositoryService: gitService}
	server.updateDesignReviewDocument(response, req)

	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "design/tasks/2026-09-14_edit-plan_1/requirements.md", writtenPath)
	require.Equal(t, "# Requirements: New title\n\nUpdated.", writtenContent)
	require.Equal(t, writtenContent, review.RequirementsSpec)
	require.Equal(t, writtenContent, task.RequirementsSpec)
	require.Equal(t, "New title", task.Name)
	require.Equal(t, "new-sha", review.GitCommitHash)
}

func TestUpdateDesignReviewDocumentRejectsConcurrentChange(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{ID: "spt_1", ProjectID: "prj_1", CreatedBy: "usr_1", DesignDocPath: "task_1"}
	review := &types.SpecTaskDesignReview{ID: "stdr_1", SpecTaskID: task.ID, Status: types.SpecTaskDesignReviewStatusPending}
	project := &types.Project{ID: task.ProjectID, DefaultRepoID: "repo_1"}
	repo := &types.GitRepository{ID: project.DefaultRepoID}

	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	mockStore.EXPECT().GetSpecTaskDesignReview(gomock.Any(), review.ID).Return(review, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	gitService := &fakeGitRepoService{
		getRepositoryFunc: func(_ context.Context, _ string) (*types.GitRepository, error) { return repo, nil },
		withRepoLockFunc:  func(_ string, fn func() error) error { return fn() },
		getFileContentsFunc: func(_ context.Context, _, _, _ string) (string, error) {
			return "reviewer edit", nil
		},
	}
	original := "original document"
	body, err := json.Marshal(types.SpecTaskDesignReviewDocumentUpdateRequest{
		DocumentType:    "technical_design",
		Content:         "reviewer edit",
		OriginalContent: &original,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/spt_1/design-reviews/stdr_1/document", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"spec_task_id": task.ID, "review_id": review.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: task.CreatedBy}))
	response := httptest.NewRecorder()

	server := &HelixAPIServer{Store: mockStore, gitRepositoryService: gitService}
	server.updateDesignReviewDocument(response, req)

	require.Equal(t, http.StatusConflict, response.Code)
	require.Contains(t, response.Body.String(), "changed since editing started")
}

func TestUpdateDesignReviewDocumentRejectsEmptyContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	original := "original"
	body, err := json.Marshal(types.SpecTaskDesignReviewDocumentUpdateRequest{
		DocumentType:    "requirements",
		Content:         " \n\t",
		OriginalContent: &original,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/spt_1/design-reviews/stdr_1/document", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"spec_task_id": "spt_1", "review_id": "stdr_1"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_1"}))
	response := httptest.NewRecorder()

	server := &HelixAPIServer{Store: mockStore}
	server.updateDesignReviewDocument(response, req)

	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestUpdateDesignReviewDocumentRejectsReviewFromAnotherTask(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{ID: "spt_1", ProjectID: "prj_1", CreatedBy: "usr_1"}
	review := &types.SpecTaskDesignReview{ID: "stdr_1", SpecTaskID: "spt_other", Status: types.SpecTaskDesignReviewStatusInReview}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	mockStore.EXPECT().GetSpecTaskDesignReview(gomock.Any(), review.ID).Return(review, nil)

	original := "original"
	body, err := json.Marshal(types.SpecTaskDesignReviewDocumentUpdateRequest{DocumentType: "requirements", Content: "updated", OriginalContent: &original})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/spt_1/design-reviews/stdr_1/document", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"spec_task_id": task.ID, "review_id": review.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: task.CreatedBy}))
	response := httptest.NewRecorder()

	server := &HelixAPIServer{Store: mockStore}
	server.updateDesignReviewDocument(response, req)

	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestUpdateDesignReviewDocumentRejectsFinalizedReview(t *testing.T) {
	for _, status := range []types.SpecTaskDesignReviewStatus{
		types.SpecTaskDesignReviewStatusApproved,
		types.SpecTaskDesignReviewStatusSuperseded,
	} {
		t.Run(string(status), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			task := &types.SpecTask{ID: "spt_1", ProjectID: "prj_1", CreatedBy: "usr_1"}
			review := &types.SpecTaskDesignReview{ID: "stdr_1", SpecTaskID: task.ID, Status: status}
			mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
			mockStore.EXPECT().GetSpecTaskDesignReview(gomock.Any(), review.ID).Return(review, nil)

			original := "original"
			body, err := json.Marshal(types.SpecTaskDesignReviewDocumentUpdateRequest{DocumentType: "requirements", Content: "updated", OriginalContent: &original})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/spt_1/design-reviews/stdr_1/document", bytes.NewReader(body))
			req = mux.SetURLVars(req, map[string]string{"spec_task_id": task.ID, "review_id": review.ID})
			req = req.WithContext(setRequestUser(req.Context(), types.User{ID: task.CreatedBy}))
			response := httptest.NewRecorder()

			server := &HelixAPIServer{Store: mockStore}
			server.updateDesignReviewDocument(response, req)

			require.Equal(t, http.StatusConflict, response.Code)
		})
	}
}

func TestUpdateDesignReviewDocumentRejectsUnauthorizedProjectMember(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{ID: "spt_1", ProjectID: "prj_1", CreatedBy: "usr_creator"}
	project := &types.Project{ID: task.ProjectID, UserID: task.CreatedBy, OrganizationID: "org_1"}
	user := types.User{ID: "usr_member"}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
		OrganizationID: project.OrganizationID,
		UserID:         user.ID,
	}).Return(&types.OrganizationMembership{OrganizationID: project.OrganizationID, UserID: user.ID, Role: types.OrganizationRoleMember}, nil)
	mockStore.EXPECT().ListTeams(gomock.Any(), &store.ListTeamsQuery{OrganizationID: project.OrganizationID, UserID: user.ID}).Return([]*types.Team{}, nil)
	mockStore.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{
		OrganizationID: project.OrganizationID,
		UserID:         user.ID,
		ResourceID:     project.ID,
	}).Return([]*types.AccessGrant{}, nil)

	original := "original"
	body, err := json.Marshal(types.SpecTaskDesignReviewDocumentUpdateRequest{DocumentType: "requirements", Content: "updated", OriginalContent: &original})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/spt_1/design-reviews/stdr_1/document", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"spec_task_id": task.ID, "review_id": "stdr_1"})
	req = req.WithContext(setRequestUser(req.Context(), user))
	response := httptest.NewRecorder()

	server := &HelixAPIServer{Store: mockStore}
	server.updateDesignReviewDocument(response, req)

	require.Equal(t, http.StatusForbidden, response.Code)
}

func TestUpdateDesignReviewDocumentSurfacesExternalPublicationFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{ID: "spt_1", ProjectID: "prj_1", CreatedBy: "usr_1", DesignDocPath: "task_1"}
	review := &types.SpecTaskDesignReview{ID: "stdr_1", SpecTaskID: task.ID, Status: types.SpecTaskDesignReviewStatusInReview}
	project := &types.Project{ID: task.ProjectID, DefaultRepoID: "repo_1"}
	repo := &types.GitRepository{ID: project.DefaultRepoID, IsExternal: true, ExternalURL: "https://example.invalid/repo.git"}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	mockStore.EXPECT().GetSpecTaskDesignReview(gomock.Any(), review.ID).Return(review, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)

	gitService := &fakeGitRepoService{
		getRepositoryFunc: func(_ context.Context, _ string) (*types.GitRepository, error) { return repo, nil },
		withExternalRepoWriteFunc: func(_ context.Context, actualRepo *types.GitRepository, opts services.ExternalRepoWriteOptions, _ func() error) error {
			require.Same(t, repo, actualRepo)
			require.True(t, opts.FailOnSyncError)
			require.True(t, opts.FailOnPushError)
			return errors.New("publication failed")
		},
	}
	original := "original"
	body, err := json.Marshal(types.SpecTaskDesignReviewDocumentUpdateRequest{DocumentType: "requirements", Content: "updated", OriginalContent: &original})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/spec-tasks/spt_1/design-reviews/stdr_1/document", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"spec_task_id": task.ID, "review_id": review.ID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: task.CreatedBy}))
	response := httptest.NewRecorder()

	server := &HelixAPIServer{Store: mockStore, gitRepositoryService: gitService}
	server.updateDesignReviewDocument(response, req)

	require.Equal(t, http.StatusInternalServerError, response.Code)
	require.Contains(t, response.Body.String(), "publication failed")
}

// CommentTimerSuite pins down the behaviour of the per-comment 2-minute
// response timer and the finalizeCommentResponse repair path. These exist
// because the timer used to mis-fire while the agent was actively streaming
// content into the linked interaction (the comment row's AgentResponse is
// only populated at message_completed time), and once the timer stamped the
// "agent did not respond" error string, finalizeCommentResponse refused to
// overwrite it with the real response.
type CommentTimerSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestCommentTimerSuite(t *testing.T) {
	suite.Run(t, new(CommentTimerSuite))
}

func (s *CommentTimerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{URL: "http://localhost:0", Host: "localhost"},
		},
		Store:  s.store,
		pubsub: pubsub.NewNoop(),
		Controller: &controller.Controller{
			Options: controller.Options{
				Store:  s.store,
				PubSub: pubsub.NewNoop(),
			},
		},
		sessionCommentTimeout: make(map[string]*time.Timer),
	}
}

func (s *CommentTimerSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestHandleCommentTimeout_SkipsErrorWhenInteractionHasContent reproduces
// the core regression: an agent that takes longer than 2 minutes to emit
// message_completed (long answer, tool calls, thinking) is mid-stream when
// the timer fires. The interaction row has real ResponseMessage content but
// the comment row's AgentResponse is still empty (it only gets populated by
// finalizeCommentResponse). The timer MUST NOT stamp the error message in
// this case.
func (s *CommentTimerSuite) TestHandleCommentTimeout_SkipsErrorWhenInteractionHasContent() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-streaming",
		RequestID:     "req-streaming",
		InteractionID: "int-streaming",
	}
	streamingInteraction := &types.Interaction{
		ID:              "int-streaming",
		SessionID:       "ses-streaming",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "Sure — here is the plan. Step 1: ...",
		// Updated recently => the agent is actively streaming a long answer.
		// The timer must re-arm and re-check, not stamp an error or finalize.
		Updated: time.Now(),
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-streaming").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-streaming").
		Return(streamingInteraction, nil)
	// Critical: NO UpdateSpecTaskDesignReviewComment call is expected — the timer
	// re-arms (in-memory only) and defers. gomock's strict mode fails the test if
	// any unexpected store call is made.

	s.server.handleCommentTimeout(context.Background(), "ses-streaming", "comment-streaming", 0)

	// Re-arm scheduled a real 2-minute timer; stop it so it can't fire after the
	// mock controller is torn down.
	if t := s.server.sessionCommentTimeout["ses-streaming"]; t != nil {
		t.Stop()
	}
}

// TestHandleCommentTimeout_FinalizesStalledStream covers an agent that started
// streaming a response but then died mid-stream: the interaction has partial
// content but is non-terminal AND has not been updated for a full timeout
// window. Deferring forever would block the queue, so the timer finalizes the
// partial response to unblock it.
func (s *CommentTimerSuite) TestHandleCommentTimeout_FinalizesStalledStream() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-stalled",
		ReviewID:      "review-stalled",
		RequestID:     "req-stalled",
		InteractionID: "int-stalled",
	}
	stalledInteraction := &types.Interaction{
		ID:              "int-stalled",
		SessionID:       "ses-stalled",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "Partial answer that never finished...",
		Updated:         time.Now().Add(-3 * commentResponseTimeout), // stale
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-stalled").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-stalled").
		Return(stalledInteraction, nil).AnyTimes()
	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-stalled").
		Return(comment, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("Partial answer that never finished...", c.AgentResponse,
				"stalled stream must be finalized with its partial content")
			s.Empty(c.RequestID, "RequestID must be cleared so the queue is unblocked")
			return nil
		},
	)
	// Short-circuit the queue-continuation lookup.
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-stalled").
		Return(nil, errNotFound{}).AnyTimes()

	s.server.handleCommentTimeout(context.Background(), "ses-stalled", "comment-stalled", 0)
}

// TestHandleCommentTimeout_FinalizesWhenInteractionIsTerminal covers the core
// deadlock fix: the agent finished (interaction state=complete with content) but
// finalizeCommentResponse never ran because the message_completed event never
// mapped back to this comment (coalesced re-sends, missed/duplicate completion,
// restart). The backstop timer MUST finalize the comment itself — copy the
// response and clear request_id — otherwise the comment stays in-flight forever
// and blocks every later comment for the session.
func (s *CommentTimerSuite) TestHandleCommentTimeout_FinalizesWhenInteractionIsTerminal() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-terminal",
		ReviewID:      "review-terminal",
		RequestID:     "req-terminal",
		InteractionID: "int-terminal",
	}
	terminalInteraction := &types.Interaction{
		ID:              "int-terminal",
		SessionID:       "ses-terminal",
		State:           types.InteractionStateComplete,
		ResponseMessage: "The agent's completed response.",
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-terminal").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-terminal").
		Return(terminalInteraction, nil).AnyTimes()
	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-terminal").
		Return(comment, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("The agent's completed response.", c.AgentResponse,
				"terminal interaction must be finalized onto the comment")
			s.Empty(c.RequestID, "RequestID must be cleared so the queue is unblocked")
			s.Nil(c.QueuedAt)
			return nil
		},
	)
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-terminal").
		Return(nil, errNotFound{}).AnyTimes()

	s.server.handleCommentTimeout(context.Background(), "ses-terminal", "comment-terminal", 0)
}

// stopRearmedTimer asserts the handler re-armed the session timer and stops it
// so it can't fire after the mock controller is torn down.
func (s *CommentTimerSuite) stopRearmedTimer(sessionID string) {
	t := s.server.sessionCommentTimeout[sessionID]
	s.Require().NotNil(t, "timer must be re-armed")
	t.Stop()
}

// expectQueueAdvance allows the processNextCommentInQueue goroutine spawned
// after a stamp to run against an empty queue.
func (s *CommentTimerSuite) expectQueueAdvance(sessionID string) {
	s.store.EXPECT().IsCommentBeingProcessedForSession(gomock.Any(), sessionID).
		Return(false, nil).MinTimes(1)
	// Empty queue is signalled as an error in the real store
	// (gorm.ErrRecordNotFound); a nil comment + nil err would nil-deref.
	s.store.EXPECT().GetNextQueuedCommentForSession(gomock.Any(), sessionID).
		Return(nil, errNotFound{}).AnyTimes()
}

// TestHandleCommentTimeout_RearmsWhenInteractionQueuedWithNoOutput reproduces
// the meta incident: the comment's interaction sat `waiting` with zero bytes
// for ~18 minutes behind four earlier prompts on the same session. "The agent
// hasn't started yet" must not be stamped as "the agent did not respond".
func (s *CommentTimerSuite) TestHandleCommentTimeout_RearmsWhenInteractionQueuedWithNoOutput() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-queued",
		RequestID:     "req-queued",
		InteractionID: "int-queued",
	}
	queuedInteraction := &types.Interaction{
		ID:      "int-queued",
		State:   types.InteractionStateWaiting,
		Updated: time.Now().Add(-10 * commentResponseTimeout), // untouched while queued
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-queued").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-queued").
		Return(queuedInteraction, nil)
	// NO UpdateSpecTaskDesignReviewComment: gomock strict mode fails on a stamp.

	s.server.handleCommentTimeout(context.Background(), "ses-queued", "comment-queued", 3)

	s.stopRearmedTimer("ses-queued")
}

// TestHandleCommentTimeout_RearmsWhilePromptAwaitingDispatch covers the comment
// whose prompt is still in the session prompt queue: RequestID holds the
// prompt-id placeholder and no interaction exists yet.
func (s *CommentTimerSuite) TestHandleCommentTimeout_RearmsWhilePromptAwaitingDispatch() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:        "comment-pending",
		RequestID: "prompt-pending",
		PromptID:  "prompt-pending",
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-pending").
		Return(comment, nil)
	s.store.EXPECT().GetPromptHistoryEntry(gomock.Any(), "prompt-pending").
		Return(&types.PromptHistoryEntry{ID: "prompt-pending", Status: "pending"}, nil)

	s.server.handleCommentTimeout(context.Background(), "ses-pending", "comment-pending", 0)

	s.stopRearmedTimer("ses-pending")
}

// TestHandleCommentTimeout_StampsWhenNoInteractionAndPromptGone: the prompt was
// removed from the queue (or never existed) and no interaction was ever created
// — the agent will never see this comment.
func (s *CommentTimerSuite) TestHandleCommentTimeout_StampsWhenNoInteractionAndPromptGone() {
	removed := time.Now()
	comment := &types.SpecTaskDesignReviewComment{
		ID:        "comment-orphan",
		RequestID: "prompt-orphan",
		PromptID:  "prompt-orphan",
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-orphan").
		Return(comment, nil)
	s.store.EXPECT().GetPromptHistoryEntry(gomock.Any(), "prompt-orphan").
		Return(&types.PromptHistoryEntry{ID: "prompt-orphan", Status: "pending", DeletedAt: &removed}, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal(CommentTimerNoResponseMessage, c.AgentResponse)
			s.Empty(c.RequestID)
			return nil
		},
	)
	s.expectQueueAdvance("ses-orphan")

	s.server.handleCommentTimeout(context.Background(), "ses-orphan", "comment-orphan", 0)
	time.Sleep(50 * time.Millisecond)
}

// TestHandleCommentTimeout_StampsWhenTerminalWithNoText is the genuine
// no-response case: the agent finished its turn without producing anything.
// Stamp the error so the user knows to retry, and advance the queue.
func (s *CommentTimerSuite) TestHandleCommentTimeout_StampsWhenTerminalWithNoText() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-silent",
		ReviewID:      "review-silent",
		RequestID:     "req-silent",
		InteractionID: "int-silent",
	}
	silentInteraction := &types.Interaction{
		ID:    "int-silent",
		State: types.InteractionStateError,
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-silent").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-silent").
		Return(silentInteraction, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal(CommentTimerNoResponseMessage, c.AgentResponse, "error message must be stamped on the comment")
			s.Empty(c.RequestID, "RequestID must be cleared so the comment is no longer 'processing'")
			s.Nil(c.QueuedAt, "QueuedAt must be cleared")
			return nil
		},
	)
	s.expectQueueAdvance("ses-silent")

	s.server.handleCommentTimeout(context.Background(), "ses-silent", "comment-silent", 0)
	// Let the goroutine spawn settle so gomock's strict mode evaluates it.
	time.Sleep(50 * time.Millisecond)
}

// TestHandleCommentTimeout_StampsAfterRearmCap bounds the wait: an interaction
// that stays non-terminal with no output for maxCommentTimerRearms windows is
// treated as dead, so it can't block the session's comment queue forever.
func (s *CommentTimerSuite) TestHandleCommentTimeout_StampsAfterRearmCap() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-dead",
		RequestID:     "req-dead",
		InteractionID: "int-dead",
	}
	deadInteraction := &types.Interaction{
		ID:    "int-dead",
		State: types.InteractionStateWaiting,
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-dead").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-dead").
		Return(deadInteraction, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal(CommentTimerNoResponseMessage, c.AgentResponse)
			s.Empty(c.RequestID)
			return nil
		},
	)
	s.expectQueueAdvance("ses-dead")

	s.server.handleCommentTimeout(context.Background(), "ses-dead", "comment-dead", maxCommentTimerRearms)
	time.Sleep(50 * time.Millisecond)
	s.Nil(s.server.sessionCommentTimeout["ses-dead"], "timer must not be re-armed past the cap")
}

// TestHandleCommentTimeout_NoopWhenAlreadyResolved verifies the early-exit
// when finalizeCommentResponse already ran (RequestID cleared) before the
// timer fires. The timer must do nothing — no error stamp, no follow-up
// queue processing.
func (s *CommentTimerSuite) TestHandleCommentTimeout_NoopWhenAlreadyResolved() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-resolved",
		RequestID:     "", // already finalized
		AgentResponse: "All done.",
		InteractionID: "int-resolved",
	}
	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-resolved").
		Return(comment, nil)
	// NO further calls expected.

	s.server.handleCommentTimeout(context.Background(), "ses-resolved", "comment-resolved", 0)
}

// TestFinalizeCommentResponse_OverwritesStaleTimerError pins the repair
// path. Suppose the timer fired prematurely and stamped the error string
// onto a comment whose linked interaction subsequently completed with a
// real response. When message_completed arrives and finalizeCommentResponse
// runs, the comment's AgentResponse must end up as the real interaction
// text, not the leftover error.
func (s *CommentTimerSuite) TestFinalizeCommentResponse_OverwritesStaleTimerError() {
	staleComment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-repair",
		ReviewID:      "review-repair",
		RequestID:     "req-repair",
		InteractionID: "int-repair",
		// The 2-min timer ran first and stamped the error.
		AgentResponse: CommentTimerNoResponseMessage,
	}
	realInteraction := &types.Interaction{
		ID:              "int-repair",
		State:           types.InteractionStateComplete,
		ResponseMessage: "The real, useful response the agent produced.",
	}

	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-repair").
		Return(staleComment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-repair").
		Return(realInteraction, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("The real, useful response the agent produced.", c.AgentResponse,
				"finalize must overwrite the stale timer-stamped error with the real response")
			s.Empty(c.RequestID)
			s.Nil(c.QueuedAt)
			return nil
		},
	)
	// finalizeCommentResponse looks up the review to find the planning
	// session for queue continuation. Returning an error short-circuits
	// the rest cleanly.
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-repair").
		Return(nil, errNotFound{}).AnyTimes()

	err := s.server.finalizeCommentResponse(context.Background(), "req-repair")
	s.NoError(err)
}

// TestFinalizeCommentResponseForInteraction_HealsStampedComment is the meta
// incident's second half: the timer stamped the comment and cleared its
// request_id, then the real answer arrived. The request_id lookup misses, so
// the completing interaction's id must find the comment and overwrite the stamp
// with the real response AND its entries.
func (s *CommentTimerSuite) TestFinalizeCommentResponseForInteraction_HealsStampedComment() {
	stamped := &types.SpecTaskDesignReviewComment{
		ID:            "comment-stamped",
		ReviewID:      "review-stamped",
		RequestID:     "",
		InteractionID: "int-late",
		AgentResponse: CommentTimerNoResponseMessage,
	}
	entries := []byte(`[{"type":"text","content":"89KB of real answer"}]`)
	lateInteraction := &types.Interaction{
		ID:              "int-late",
		State:           types.InteractionStateComplete,
		ResponseMessage: "89KB of real answer",
		ResponseEntries: entries,
	}

	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "int-late").
		Return(nil, errNotFound{})
	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-late").
		Return(stamped, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-late").
		Return(lateInteraction, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("comment-stamped", c.ID)
			s.Equal("89KB of real answer", c.AgentResponse, "stale stamp must be overwritten")
			s.JSONEq(string(entries), string(c.AgentResponseEntries), "entries must be copied so tool calls render")
			s.NotNil(c.AgentResponseAt)
			return nil
		},
	)
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-stamped").
		Return(nil, errNotFound{}).AnyTimes()

	s.NoError(s.server.finalizeCommentResponseForInteraction(context.Background(), "int-late", "int-late"))
}

// TestFinalizeCommentResponseForInteraction_NoRequestIDBranch covers the
// message_completed fallback for agents that don't echo request_id.
func (s *CommentTimerSuite) TestFinalizeCommentResponseForInteraction_NoRequestIDBranch() {
	stamped := &types.SpecTaskDesignReviewComment{
		ID:            "comment-noreq",
		ReviewID:      "review-noreq",
		InteractionID: "int-noreq",
		AgentResponse: CommentTimerNoResponseMessage,
	}
	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-noreq").
		Return(stamped, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-noreq").
		Return(&types.Interaction{ID: "int-noreq", State: types.InteractionStateComplete, ResponseMessage: "answer"}, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("answer", c.AgentResponse)
			return nil
		},
	)
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-noreq").
		Return(nil, errNotFound{}).AnyTimes()

	s.NoError(s.server.finalizeCommentResponseForInteraction(context.Background(), "", "int-noreq"))
}

// TestFinalizeCommentResponseForInteraction_DoesNotClobberResentComment: the
// user re-sent a stamped comment; the re-send is a new row on a new interaction
// and has been answered. A late completion for the OLD interaction must only
// ever touch the old row, and must never overwrite a real answer.
func (s *CommentTimerSuite) TestFinalizeCommentResponseForInteraction_DoesNotClobberResentComment() {
	resent := &types.SpecTaskDesignReviewComment{
		ID:            "comment-resent",
		InteractionID: "int-new",
		AgentResponse: "Answer to the re-sent comment.",
	}
	// A store that (wrongly) resolved the old interaction to the re-sent row
	// must still be rejected by the exact-id guard.
	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-old").
		Return(resent, nil)
	// A comment already holding a real answer is left alone too.
	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-new").
		Return(resent, nil)
	// NO UpdateSpecTaskDesignReviewComment expected.

	s.Error(s.server.finalizeCommentResponseForInteraction(context.Background(), "", "int-old"))
	s.Error(s.server.finalizeCommentResponseForInteraction(context.Background(), "", "int-new"))
	s.Equal("Answer to the re-sent comment.", resent.AgentResponse)
}

// TestFinalizeCommentResponse_PopulatesEmptyComment is the standard happy
// path: comment has no AgentResponse yet, interaction has content, finalize
// copies it across.
func (s *CommentTimerSuite) TestFinalizeCommentResponse_PopulatesEmptyComment() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-happy",
		ReviewID:      "review-happy",
		RequestID:     "req-happy",
		InteractionID: "int-happy",
	}
	interaction := &types.Interaction{
		ID:              "int-happy",
		State:           types.InteractionStateComplete,
		ResponseMessage: "Standard agent reply.",
	}

	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-happy").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-happy").
		Return(interaction, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("Standard agent reply.", c.AgentResponse)
			s.Empty(c.RequestID)
			return nil
		},
	)
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-happy").
		Return(nil, errNotFound{}).AnyTimes()

	err := s.server.finalizeCommentResponse(context.Background(), "req-happy")
	s.NoError(err)
}

// TestReconcileStuckInFlightComment_FinalizesTerminal verifies that a comment
// stuck in-flight (request_id set, no response) whose interaction has already
// completed is finalized — clearing the marker so the session's queue can drain.
func (s *CommentTimerSuite) TestReconcileStuckInFlightComment_FinalizesTerminal() {
	stuck := &types.SpecTaskDesignReviewComment{
		ID:            "comment-zombie",
		ReviewID:      "review-zombie",
		RequestID:     "req-zombie",
		InteractionID: "int-zombie",
	}
	terminalInteraction := &types.Interaction{
		ID:              "int-zombie",
		State:           types.InteractionStateComplete,
		ResponseMessage: "Already answered long ago.",
	}

	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses-zombie").
		Return(stuck, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-zombie").
		Return(terminalInteraction, nil).AnyTimes()
	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-zombie").
		Return(stuck, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("Already answered long ago.", c.AgentResponse)
			s.Empty(c.RequestID)
			return nil
		},
	)
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-zombie").
		Return(nil, errNotFound{}).AnyTimes()

	reconciled := s.server.reconcileStuckInFlightComment(context.Background(), "ses-zombie")
	s.True(reconciled, "a terminal stuck comment must be reconciled")
}

// TestReconcileStuckInFlightComment_SkipsActive verifies that a comment whose
// interaction is still waiting/streaming is left untouched — the agent is
// genuinely working and we must not steal the in-flight slot.
func (s *CommentTimerSuite) TestReconcileStuckInFlightComment_SkipsActive() {
	active := &types.SpecTaskDesignReviewComment{
		ID:            "comment-active",
		RequestID:     "req-active",
		InteractionID: "int-active",
	}
	activeInteraction := &types.Interaction{
		ID:    "int-active",
		State: types.InteractionStateWaiting,
	}

	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses-active").
		Return(active, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-active").
		Return(activeInteraction, nil)
	// NO finalize calls expected.

	reconciled := s.server.reconcileStuckInFlightComment(context.Background(), "ses-active")
	s.False(reconciled, "an actively-processing comment must not be reconciled")
}

// errNotFound is a trivial sentinel returned to short-circuit the
// finalizeCommentResponse continuation logic without dragging in the
// gorm package just for ErrRecordNotFound. The function only checks for
// non-nil err; the specific type doesn't matter.
type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }
