package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// RestartSessionContainerSuite pins the load-bearing contract for the
// "Restart agent session" fix: restart means RECREATE the desktop
// container (StopDesktop → StartDesktop), not continue the existing
// session. Every restart surface (in-chat /sessions/{id}/restart-agent,
// the worker-page button via the helix-org SessionRestarter port, and
// the spec-task page) funnels through restartSessionContainer, so this
// is the one place the behaviour is verified.
//
// Regression target: the worker-page button used to route through
// activate → SendMessage, which reuses the same (stuck) container and so
// never recovered a wedged worker.
type RestartSessionContainerSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestRestartSessionContainerSuite(t *testing.T) {
	suite.Run(t, new(RestartSessionContainerSuite))
}

func (s *RestartSessionContainerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)

	s.server = &HelixAPIServer{
		Store:                 s.store,
		externalAgentExecutor: s.executor,
		Cfg:                   &config.ServerConfig{},
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
	}
}

func (s *RestartSessionContainerSuite) TearDownTest() {
	s.ctrl.Finish()
}

// zedSession builds a restartable external-agent session owned by
// userID. ZedThreadID is intentionally left empty so resumeSessionInternal
// does not spawn the async open_thread goroutine (which would call into
// mocks after the test finishes).
func zedSession(id, userID, projectID string) *types.Session {
	return &types.Session{
		ID:        id,
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		ProjectID: projectID,
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
			ProjectID: projectID,
		},
	}
}

// TestRestartRecreatesContainerInOrder is the core regression gate:
// restart tears down the container (StopDesktop) and brings up a fresh
// one (StartDesktop) — in that order — then resets crashed prompts and
// kicks the queue. If a future change reverts restart to a SendMessage
// continuation, StartDesktop stops being called and this fails.
func (s *RestartSessionContainerSuite) TestRestartRecreatesContainerInOrder() {
	ctx := context.Background()
	const (
		userID    = "user_op"
		projectID = "prj_restart"
		sessionID = "ses_restart"
	)
	user := &types.User{ID: userID}
	session := zedSession(sessionID, userID, projectID)

	// resumeSessionInternal requires the project to exist and reads repos.
	s.store.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{ID: projectID, UserID: userID}, nil).AnyTimes()
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	// Refetch + metadata write after StartDesktop.
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(session, nil).AnyTimes()

	// The load-bearing ordering: StopDesktop THEN StartDesktop.
	gomock.InOrder(
		s.executor.EXPECT().StopDesktop(gomock.Any(), sessionID).Return(nil).Times(1),
		s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).Return(&types.DesktopAgentResponse{DevContainerID: "dev_new"}, nil).Times(1),
	)

	// Crashed prompts are reset and the count is returned.
	s.store.EXPECT().ResetCrashedPromptsForSession(gomock.Any(), sessionID).Return(3, nil).Times(1)

	// The queue is kicked in a goroutine — signal so we can wait for it
	// before TearDownTest runs ctrl.Finish().
	pumped := make(chan struct{})
	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), sessionID).DoAndReturn(
		func(_ context.Context, _ string) (*types.PromptHistoryEntry, error) {
			close(pumped)
			return nil, nil
		}).Times(1)

	resetCount, herr := s.server.restartSessionContainer(ctx, user, session, false)
	s.Require().Nil(herr)
	s.Equal(3, resetCount, "restart must return the count of crashed prompts it reset")

	select {
	case <-pumped:
	case <-time.After(2 * time.Second):
		s.Fail("processAnyPendingPrompt was not kicked after restart")
	}
}

// TestRestartClearsZedThread pins the crash-recovery semantics: when the caller
// asks for a thread reset (resetThread=true), restart opens a FRESH thread rather
// than reattaching to the crashed one. A wedged agent often poisons the thread
// itself, so reattaching reproduces the wedge. We assert the session's
// ZedThreadID is cleared and persisted (UpdateSessionMetadata) before the
// container is recreated.
func (s *RestartSessionContainerSuite) TestRestartClearsZedThread() {
	ctx := context.Background()
	const (
		userID    = "user_op"
		projectID = "prj_restart"
		sessionID = "ses_restart"
	)
	user := &types.User{ID: userID}
	session := zedSession(sessionID, userID, projectID)
	session.Metadata.ZedThreadID = "poisoned-thread" // a crashed thread to escape

	s.store.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{ID: projectID, UserID: userID}, nil).AnyTimes()
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	// GetSession returns the same pointer we mutate, so resume's post-StartDesktop
	// refetch sees the cleared thread (no open_thread goroutine into mocks).
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(session, nil).AnyTimes()

	// The load-bearing assertion: the thread is cleared and persisted BEFORE the
	// container is recreated. Ordering matters — a clear after StartDesktop would
	// let the reconnect reload the stale thread from the DB, so pin it in the
	// InOrder chain (StopDesktop → clear → StartDesktop), not as a bare EXPECT.
	var clearedTo = "unset"
	clearThread := s.store.EXPECT().UpdateSessionMetadata(gomock.Any(), sessionID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, meta types.SessionMetadata) error {
			clearedTo = meta.ZedThreadID
			return nil
		}).Times(1)
	stopDesktop := s.executor.EXPECT().StopDesktop(gomock.Any(), sessionID).Return(nil).Times(1)
	startDesktop := s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).Return(&types.DesktopAgentResponse{DevContainerID: "dev_new"}, nil).Times(1)
	gomock.InOrder(stopDesktop, clearThread, startDesktop)
	s.store.EXPECT().ResetCrashedPromptsForSession(gomock.Any(), sessionID).Return(1, nil).Times(1)

	pumped := make(chan struct{})
	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), sessionID).DoAndReturn(
		func(_ context.Context, _ string) (*types.PromptHistoryEntry, error) {
			close(pumped)
			return nil, nil
		}).Times(1)

	_, herr := s.server.restartSessionContainer(ctx, user, session, true)
	s.Require().Nil(herr)
	s.Equal("", clearedTo, "restart with resetThread=true must clear ZedThreadID so Zed opens a fresh thread")

	select {
	case <-pumped:
	case <-time.After(2 * time.Second):
		s.Fail("processAnyPendingPrompt was not kicked after restart")
	}
}

// TestRestartPreservesThreadWhenNotReset is the incident regression gate
// (design/2026-07-20-restart-clears-zed-thread-context-loss.md): a restart with
// resetThread=false MUST keep the existing thread so the reconnect re-attaches
// and the conversation resumes. UpdateSessionMetadata must NOT be called to blank
// the thread — clearing a healthy thread is silent total context loss.
func (s *RestartSessionContainerSuite) TestRestartPreservesThreadWhenNotReset() {
	ctx := context.Background()
	const (
		userID    = "user_op"
		projectID = "prj_restart"
		sessionID = "ses_restart"
	)
	user := &types.User{ID: userID}
	session := zedSession(sessionID, userID, projectID)
	session.Metadata.ZedThreadID = "healthy-thread-keep-me"

	s.store.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{ID: projectID, UserID: userID}, nil).AnyTimes()
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(session, nil).AnyTimes()

	// The load-bearing assertion: the thread is NEVER blanked. gomock fails the
	// test if UpdateSessionMetadata is called with an emptied ZedThreadID.
	s.store.EXPECT().UpdateSessionMetadata(gomock.Any(), sessionID, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, meta types.SessionMetadata) error {
			s.Failf("thread cleared", "restart with resetThread=false must not blank ZedThreadID, got %q", meta.ZedThreadID)
			return nil
		}).AnyTimes()

	s.executor.EXPECT().StopDesktop(gomock.Any(), sessionID).Return(nil).Times(1)
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).Return(&types.DesktopAgentResponse{DevContainerID: "dev_new"}, nil).Times(1)
	s.store.EXPECT().ResetCrashedPromptsForSession(gomock.Any(), sessionID).Return(0, nil).Times(1)

	pumped := make(chan struct{})
	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), sessionID).DoAndReturn(
		func(_ context.Context, _ string) (*types.PromptHistoryEntry, error) {
			close(pumped)
			return nil, nil
		}).Times(1)

	_, herr := s.server.restartSessionContainer(ctx, user, session, false)
	s.Require().Nil(herr)
	s.Equal("healthy-thread-keep-me", session.Metadata.ZedThreadID, "thread pointer must be preserved across a non-reset restart")

	select {
	case <-pumped:
	case <-time.After(2 * time.Second):
		s.Fail("processAnyPendingPrompt was not kicked after restart")
	}
}

// TestButtonPreservesHealthyThreadResetsWedged pins the human-button decision:
// restartCrashedAgentThread inspects the session's last interaction and only
// resets the thread when that turn did not finish cleanly. A completed last turn
// → preserve (no metadata clear); a mid-turn/waiting last turn → reset.
func (s *RestartSessionContainerSuite) TestButtonPreservesHealthyThreadResetsWedged() {
	const (
		userID    = "user_op"
		projectID = "prj_restart"
		sessionID = "ses_restart"
	)

	cases := []struct {
		name      string
		lastState types.InteractionState
		wantReset bool
	}{
		{"complete_last_turn_preserves", types.InteractionStateComplete, false},
		{"interrupted_last_turn_preserves", types.InteractionStateInterrupted, false},
		{"waiting_last_turn_resets", types.InteractionStateWaiting, true},
		{"error_last_turn_resets", types.InteractionStateError, true},
	}

	for _, tc := range cases {
		s.Run(tc.name, func() {
			// Fresh controller per subtest so expectations don't leak.
			s.SetupTest()
			defer s.ctrl.Finish()

			session := zedSession(sessionID, userID, projectID)
			session.Metadata.ZedThreadID = "existing-thread"

			s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()
			s.store.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{ID: projectID, UserID: userID}, nil).AnyTimes()
			s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
			s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(session, nil).AnyTimes()

			// The health probe: newest interaction carries tc.lastState.
			s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
				[]*types.Interaction{{ID: "int_1", State: tc.lastState}}, int64(1), nil).AnyTimes()

			cleared := false
			s.store.EXPECT().UpdateSessionMetadata(gomock.Any(), sessionID, gomock.Any()).DoAndReturn(
				func(_ context.Context, _ string, meta types.SessionMetadata) error {
					if meta.ZedThreadID == "" {
						cleared = true
					}
					return nil
				}).AnyTimes()

			s.executor.EXPECT().StopDesktop(gomock.Any(), sessionID).Return(nil).Times(1)
			s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).Return(&types.DesktopAgentResponse{DevContainerID: "dev_new"}, nil).Times(1)
			s.store.EXPECT().ResetCrashedPromptsForSession(gomock.Any(), sessionID).Return(0, nil).Times(1)
			s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), sessionID).Return(nil, nil).AnyTimes()

			req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/restart-agent", nil)
			req = mux.SetURLVars(req, map[string]string{"id": sessionID})
			req = req.WithContext(setRequestUser(req.Context(), types.User{ID: userID}))

			resp, herr := s.server.restartCrashedAgentThread(nil, req)
			s.Require().Nil(herr)
			s.Equal(tc.wantReset, resp["thread_reset"], "thread_reset decision for last state %q", tc.lastState)
			s.Equal(tc.wantReset, cleared, "metadata clear must match reset decision for last state %q", tc.lastState)

			// Let the queue-kick goroutine settle before ctrl.Finish().
			time.Sleep(50 * time.Millisecond)
		})
	}
}

// TestRestartCrashedAgentThread_403WhenNotOwner pins authorization on
// the in-chat endpoint: a user who doesn't own the (orgless) session
// gets 403 and no container is touched.
func (s *RestartSessionContainerSuite) TestRestartCrashedAgentThread_403WhenNotOwner() {
	const sessionID = "ses_other"
	session := zedSession(sessionID, "owner_user", "prj_x")
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).Times(1)
	// No StopDesktop/StartDesktop expected — gomock fails if they run.

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/restart-agent", nil)
	req = mux.SetURLVars(req, map[string]string{"id": sessionID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "intruder"}))

	_, herr := s.server.restartCrashedAgentThread(nil, req)
	s.Require().NotNil(herr)
	s.Equal(http.StatusForbidden, herr.StatusCode)
}

// TestRestart400WhenNotZedExternal pins the guard: restart only applies
// to external Zed agent sessions. A non-zed session is rejected before
// any container teardown.
func (s *RestartSessionContainerSuite) TestRestart400WhenNotZedExternal() {
	ctx := context.Background()
	const userID = "user_op"
	user := &types.User{ID: userID}
	session := &types.Session{
		ID:    "ses_plain",
		Owner: userID,
		Metadata: types.SessionMetadata{
			AgentType: "", // not zed_external
		},
	}
	// No executor / store calls expected.
	_, herr := s.server.restartSessionContainer(ctx, user, session, false)
	s.Require().NotNil(herr)
	s.Equal(http.StatusBadRequest, herr.StatusCode)
}
