package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// AutoRestartCrashedAgentSuite pins the guard-rail contract for automatic
// agent-crash recovery on AUTONOMOUS surfaces (spec tasks, org workers) where
// no human is present to click the in-chat Restart button. The crash handlers
// call maybeAutoRestartCrashedAgent; this suite verifies it:
//   - actually recovers (StopDesktop → StartDesktop) and spends one unit of
//     restart budget when the session opted in and has budget left;
//   - is a NO-OP for human desktop sessions (AutoRestartOnCrash == false), which
//     keep the explicit button;
//   - STOPS once the budget is exhausted, so a boot-crash loop can't churn
//     containers forever;
//   - DEDUPES concurrent triggers (a single crash can surface as both
//     thread_load_error and chat_response_error).
type AutoRestartCrashedAgentSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer

	prevBackoff time.Duration
}

func TestAutoRestartCrashedAgentSuite(t *testing.T) {
	suite.Run(t, new(AutoRestartCrashedAgentSuite))
}

func (s *AutoRestartCrashedAgentSuite) SetupTest() {
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

	// Don't actually wait the backoff in unit tests.
	s.prevBackoff = autoRestartMinBackoff
	autoRestartMinBackoff = 0
}

func (s *AutoRestartCrashedAgentSuite) TearDownTest() {
	autoRestartMinBackoff = s.prevBackoff
	s.ctrl.Finish()
}

// autonomousSession is a restartable external-agent session that opted into
// auto-restart with `count` units of budget already spent. ZedThreadID is left
// empty so resumeSessionInternal does not spawn the async open_thread goroutine
// (which would call into mocks after the test finishes).
func autonomousSession(id, userID, projectID string, count int) *types.Session {
	return &types.Session{
		ID:        id,
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		ProjectID: projectID,
		Metadata: types.SessionMetadata{
			AgentType:          "zed_external",
			ProjectID:          projectID,
			AutoRestartOnCrash: true,
			AutoRestartCount:   count,
		},
	}
}

// TestAutonomousRestartsAndIncrements is the core gate: an opted-in session with
// budget left is recovered by recreating the container (StopDesktop →
// StartDesktop) and one unit of restart budget is persisted (AutoRestartCount
// 0 → 1) BEFORE the restart, so a restart that itself boot-crashes still counts.
func (s *AutoRestartCrashedAgentSuite) TestAutonomousRestartsAndIncrements() {
	const (
		userID    = "user_svc"
		projectID = "prj_auto"
		sessionID = "ses_auto"
	)
	session := autonomousSession(sessionID, userID, projectID, 0)

	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()
	s.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: userID}, nil).AnyTimes()
	s.store.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{ID: projectID, UserID: userID}, nil).AnyTimes()
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	// Capture every persisted session so we can assert the budget was spent.
	var persistedCounts []int
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, sess types.Session) (*types.Session, error) {
			persistedCounts = append(persistedCounts, sess.Metadata.AutoRestartCount)
			return &sess, nil
		}).AnyTimes()

	// The recovery itself: recreate the container.
	gomock.InOrder(
		s.executor.EXPECT().StopDesktop(gomock.Any(), sessionID).Return(nil).Times(1),
		s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).Return(&types.DesktopAgentResponse{DevContainerID: "dev_new"}, nil).Times(1),
	)
	s.store.EXPECT().ResetCrashedPromptsForSession(gomock.Any(), sessionID).Return(0, nil).Times(1)

	// restartSessionContainer kicks the queue in a goroutine; signal so we can
	// wait for it before TearDownTest runs ctrl.Finish().
	pumped := make(chan struct{})
	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), sessionID).DoAndReturn(
		func(_ context.Context, _ string) (*types.PromptHistoryEntry, error) {
			close(pumped)
			return nil, nil
		}).Times(1)

	s.server.maybeAutoRestartCrashedAgent(sessionID)

	select {
	case <-pumped:
	case <-time.After(2 * time.Second):
		s.Fail("auto-restart did not recreate the container / kick the queue")
	}

	s.Contains(persistedCounts, 1, "auto-restart must persist AutoRestartCount 0 → 1 before restarting")
}

// TestHumanSurfaceNoOp: a human desktop session (AutoRestartOnCrash == false)
// keeps the explicit Restart button — auto-restart must not touch the container.
func (s *AutoRestartCrashedAgentSuite) TestHumanSurfaceNoOp() {
	const sessionID = "ses_human"
	session := &types.Session{
		ID:    sessionID,
		Owner: "user_h",
		Metadata: types.SessionMetadata{
			AgentType:          "zed_external",
			AutoRestartOnCrash: false,
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).Times(1)
	// No StopDesktop/StartDesktop/UpdateSession expectations — gomock fails if hit.

	s.server.maybeAutoRestartCrashedAgent(sessionID)
}

// TestBudgetExhaustedNoOp: once AutoRestartCount has reached the cap, stop — a
// boot-crash loop must not rebuild containers forever. The crash stays terminal.
func (s *AutoRestartCrashedAgentSuite) TestBudgetExhaustedNoOp() {
	const sessionID = "ses_exhausted"
	session := autonomousSession(sessionID, "user_x", "prj_x", autoRestartMaxAttempts)
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).Times(1)
	// No GetUser/StopDesktop/StartDesktop — gomock fails if the restart runs.

	s.server.maybeAutoRestartCrashedAgent(sessionID)
}

// TestInflightDedup: a crash can surface as BOTH thread_load_error and
// chat_response_error. The second concurrent trigger must no-op without even
// reading the session, so a single crash recreates the container once.
func (s *AutoRestartCrashedAgentSuite) TestInflightDedup() {
	const sessionID = "ses_dedup"
	// Simulate an in-flight restart already holding the session.
	s.server.autoRestartInflight.Store(sessionID, struct{}{})
	// No store expectations at all — a deduped trigger must not even GetSession.

	s.server.maybeAutoRestartCrashedAgent(sessionID)
}
