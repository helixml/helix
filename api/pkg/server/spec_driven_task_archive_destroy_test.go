package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// destroy_workspace=true on archive destroys the task's desktop instead of
// stopping it, so the workspace checkout goes with it. These reuse the
// archive/stop suite's fixtures: the guards are the same ones.

// expectStoppedDesktop arranges a finished task whose desktop was already
// stopped earlier — the common case for a client that archives each task once
// it is done. Destroy must still run: the workspace outlives the stop.
func (s *SpecTaskArchiveStopSuite) expectStoppedDesktop() {
	s.store.EXPECT().
		GetSpecTaskExternalAgent(gomock.Any(), s.taskID).
		Return(&types.SpecTaskExternalAgent{SpecTaskID: s.taskID, Status: "stopped"}, nil).
		AnyTimes()
	s.store.EXPECT().
		GetSession(gomock.Any(), s.sessionID).
		Return(&types.Session{
			ID: s.sessionID,
			Metadata: types.SessionMetadata{
				AgentType:           "zed_external",
				ExternalAgentStatus: "stopped",
			},
		}, nil).
		AnyTimes()
	s.store.EXPECT().
		GetLatestInteractionsForSessions(gomock.Any(), []string{s.sessionID}).
		Return(map[string]*types.Interaction{
			s.sessionID: {State: types.InteractionStateComplete},
		}, nil).
		AnyTimes()
}

// expectInterruptReapIsLast closes archiveFinished on the goroutine's last
// store call, the interrupt reap that follows a successful teardown.
func (s *SpecTaskArchiveStopSuite) expectInterruptReapIsLast() {
	s.store.EXPECT().
		ReapWaitingInteractions(gomock.Any(), s.sessionID, types.InteractionStateInterrupted, "spec task archived").
		DoAndReturn(func(context.Context, string, types.InteractionState, string) ([]*types.Interaction, error) {
			defer close(s.archiveFinished)
			return nil, nil
		}).
		Times(1)
}

func (s *SpecTaskArchiveStopSuite) TestArchive_DestroyWorkspace_DestroysStoppedDesktop() {
	task := s.makeTask(types.TaskStatusDone)
	s.expectArchiveBookkeeping(task)
	s.expectStoppedDesktop()
	// Destroy names the task so hydra removes /data/workspaces/spec-tasks/<id>;
	// no plain stop is issued for the session.
	s.executor.EXPECT().DestroyDesktop(gomock.Any(), s.sessionID, s.taskID).Return(nil).Times(1)
	s.expectInterruptReapIsLast()

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequestBody(`{"archived":true,"stop_agent":true,"destroy_workspace":true}`))

	s.Equal(http.StatusOK, rr.Code)
	s.waitForArchiveSideEffects()
}

// The default stop_agent (omitted) also destroys a finished desktop.
func (s *SpecTaskArchiveStopSuite) TestArchive_DestroyWorkspace_DefaultStopDestroys() {
	task := s.makeTask(types.TaskStatusDone)
	s.expectArchiveBookkeeping(task)
	s.expectStoppedDesktop()
	s.executor.EXPECT().DestroyDesktop(gomock.Any(), s.sessionID, s.taskID).Return(nil).Times(1)
	s.expectInterruptReapIsLast()

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequestBody(`{"archived":true,"destroy_workspace":true}`))

	s.Equal(http.StatusOK, rr.Code)
	s.waitForArchiveSideEffects()
}

// A sandbox host that is gone for good cannot be reached; the archive still
// ends the waiting turns and leaves the leftovers to the orphan reaper.
func (s *SpecTaskArchiveStopSuite) TestArchive_DestroyWorkspace_HostGoneIsNotAnError() {
	task := s.makeTask(types.TaskStatusDone)
	s.expectArchiveBookkeeping(task)
	s.expectStoppedDesktop()
	s.executor.EXPECT().
		DestroyDesktop(gomock.Any(), s.sessionID, s.taskID).
		Return(fmt.Errorf("teardown dev container %s: %w", s.sessionID, connman.ErrNoConnection)).
		Times(1)
	s.expectInterruptReapIsLast()

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequestBody(`{"archived":true,"stop_agent":true,"destroy_workspace":true}`))

	s.Equal(http.StatusOK, rr.Code)
	s.waitForArchiveSideEffects()
}

// destroy_workspace does not widen the live-agent guard: without an explicit
// stop_agent a mid-turn agent is left alone, workspace included.
func (s *SpecTaskArchiveStopSuite) TestArchive_DestroyWorkspace_LiveAgentWithoutExplicitStopIsUntouched() {
	task := s.makeTask(types.TaskStatusImplementation)
	s.expectArchiveBookkeeping(task)
	s.expectLiveAgent()
	// No executor expectations: neither StopDesktop nor DestroyDesktop may run.

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequestBody(`{"archived":true,"destroy_workspace":true}`))

	s.Equal(http.StatusOK, rr.Code)
}

func (s *SpecTaskArchiveStopSuite) TestArchive_DestroyWorkspace_IgnoredWithStopAgentFalse() {
	task := s.makeTask(types.TaskStatusDone)
	s.expectArchiveBookkeeping(task)

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequestBody(`{"archived":true,"stop_agent":false,"destroy_workspace":true}`))

	s.Equal(http.StatusOK, rr.Code)
}

func (s *SpecTaskArchiveStopSuite) TestArchive_DestroyWorkspace_IgnoredOnUnarchive() {
	task := s.makeTask(types.TaskStatusDone)
	task.Archived = true
	s.expectArchiveBookkeeping(task)

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequestBody(`{"archived":false,"stop_agent":true,"destroy_workspace":true}`))

	s.Equal(http.StatusOK, rr.Code)
}

// Keep Alive is the user's instruction to keep the workspace; it downgrades
// destroy to a plain stop.
func (s *SpecTaskArchiveStopSuite) TestArchive_DestroyWorkspace_KeepAliveStopsInstead() {
	task := s.makeTask(types.TaskStatusDone)
	task.KeepAlive = true
	s.expectArchiveBookkeeping(task)
	s.expectStoppedDesktop()
	s.executor.EXPECT().StopDesktop(gomock.Any(), s.sessionID).Return(nil).Times(1)
	s.expectInterruptReapIsLast()

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequestBody(`{"archived":true,"stop_agent":true,"destroy_workspace":true}`))

	s.Equal(http.StatusOK, rr.Code)
	s.waitForArchiveSideEffects()
}

// A task that never got a planning session has no desktop to destroy.
func (s *SpecTaskArchiveStopSuite) TestArchive_DestroyWorkspace_NoPlanningSessionDestroysNothing() {
	task := s.makeTask(types.TaskStatusDone)
	task.PlanningSessionID = ""
	s.expectArchiveBookkeeping(task)
	// Probed twice (liveness, then the implementation-agent stop); the second
	// probe is the goroutine's last call.
	calls := 0
	s.store.EXPECT().
		GetSpecTaskExternalAgent(gomock.Any(), s.taskID).
		DoAndReturn(func(context.Context, string) (*types.SpecTaskExternalAgent, error) {
			calls++
			if calls == 2 {
				defer close(s.archiveFinished)
			}
			return nil, fmt.Errorf("not found")
		}).
		Times(2)

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequestBody(`{"archived":true,"stop_agent":true,"destroy_workspace":true}`))

	s.Equal(http.StatusOK, rr.Code)
	s.waitForArchiveSideEffects()
}

func (s *SpecTaskArchiveStopSuite) TestArchiveRequest_DestroyWorkspaceSemantics() {
	for body, want := range map[string]bool{
		`{"archived":true,"destroy_workspace":true}`:                    true,
		`{"archived":true,"stop_agent":true,"destroy_workspace":true}`:  true,
		`{"archived":true,"stop_agent":false,"destroy_workspace":true}`: false,
		`{"archived":false,"destroy_workspace":true}`:                   false,
		`{"archived":true}`:                   false,
		`{"archived":true,"stop_agent":true}`: false,
	} {
		var req types.SpecTaskArchiveRequest
		s.Require().NoError(json.Unmarshal([]byte(body), &req), body)
		s.Equal(want, req.DestroysWorkspace(), body)
	}
}

// Another tenant cannot archive, let alone destroy, a task it does not own:
// the request is refused before any write or desktop teardown. A key scoped to
// a different project is refused the same way, even for the owner.
func (s *SpecTaskArchiveStopSuite) TestArchive_DestroyWorkspace_CrossTenantIsRefused() {
	for name, caller := range map[string]types.User{
		"other user":                 {ID: "user_other_tenant"},
		"owner, other project's key": {ID: s.userID, ProjectID: "project_other"},
	} {
		s.Run(name, func() {
			task := s.makeTask(types.TaskStatusDone)
			s.store.EXPECT().GetSpecTask(gomock.Any(), s.taskID).Return(task, nil)
			s.store.EXPECT().
				GetProject(gomock.Any(), s.projectID).
				Return(&types.Project{ID: s.projectID, UserID: s.userID}, nil).
				AnyTimes()
			// No UpdateSpecTask, StopDesktop or DestroyDesktop expectation:
			// gomock fails the test if any of them is called.

			req := s.makeArchiveRequestBody(`{"archived":true,"stop_agent":true,"destroy_workspace":true}`)
			req = req.WithContext(setRequestUser(req.Context(), caller))
			rr := httptest.NewRecorder()
			s.server.archiveSpecTask(rr, req)

			s.Contains([]int{http.StatusForbidden, http.StatusNotFound}, rr.Code)
			s.False(task.Archived, "a refused archive must not change the task")
		})
	}
}
