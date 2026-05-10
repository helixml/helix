package server

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// AutoWakeColdStartSuite covers the no-WebSocket branch added for
// helixml/helix#2397: when a stuck waiting interaction is on a session
// with no live WS, kick the dev container auto-start (bounded by
// autoWakeMaxRetries) instead of returning silently.
type AutoWakeColdStartSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestAutoWakeColdStartSuite(t *testing.T) {
	suite.Run(t, new(AutoWakeColdStartSuite))
}

func (s *AutoWakeColdStartSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)

	s.server = &HelixAPIServer{
		Store:                  s.store,
		externalAgentExecutor:  s.executor,
		externalAgentWSManager: NewExternalAgentWSManager(),
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
		streamingContexts: make(map[string]*streamingContext),
	}
}

func (s *AutoWakeColdStartSuite) TearDownTest() {
	s.ctrl.Finish()
}

// stuckInteraction returns an interaction old enough to pass the threshold gate.
func stuckInteraction(id, sessionID string, autoWakeCount int) *types.Interaction {
	return &types.Interaction{
		ID:            id,
		SessionID:     sessionID,
		State:         types.InteractionStateWaiting,
		PromptMessage: "do the thing",
		Created:       time.Now().Add(-5 * time.Minute),
		AutoWakeCount: autoWakeCount,
	}
}

// TestKicksAutoStartWhenNoWS: stuck interaction on a session with no live
// WS triggers a goroutine call to autoStartDevContainerForSession (which
// in turn calls StartDesktop) and increments AutoWakeCount via a targeted
// column update.
func (s *AutoWakeColdStartSuite) TestKicksAutoStartWhenNoWS() {
	stuck := stuckInteraction("int-1", "ses_cold", 0)

	// IncrementInteractionAutoWakeCount fires before the goroutine.
	s.store.EXPECT().IncrementInteractionAutoWakeCount(gomock.Any(), "int-1").Return(1, nil)

	// autoStartDevContainerForSession runs in a goroutine — it will call
	// GetSession (and possibly more, depending on session shape).
	session := &types.Session{
		ID:    "ses_cold",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
			ProjectID: "prj_x",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_cold").Return(session, nil).AnyTimes()
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(&types.Session{}, nil).AnyTimes()

	startCalled := make(chan struct{}, 1)
	s.executor.EXPECT().StartDesktop(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *types.DesktopAgent) (*types.DesktopAgentResponse, error) {
			startCalled <- struct{}{}
			return &types.DesktopAgentResponse{DevContainerID: "dev_1"}, nil
		},
	).Times(1)

	s.server.maybeAutoWake(context.Background(), stuck)

	select {
	case <-startCalled:
		// Good — the no-WS branch fired StartDesktop via autoStart.
	case <-time.After(2 * time.Second):
		s.FailNow("StartDesktop was not invoked — cold-start kick did not fire")
	}
}

// TestMarksAsErrorAfterMaxRetries: once AutoWakeCount has hit the cap,
// further scans must mark the interaction state=error and stop kicking.
func (s *AutoWakeColdStartSuite) TestMarksAsErrorAfterMaxRetries() {
	stuck := stuckInteraction("int-2", "ses_exhausted", autoWakeMaxRetries)

	var captured *types.Interaction
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *types.Interaction) (*types.Interaction, error) {
			captured = in
			return in, nil
		},
	).Times(1)

	// IncrementInteractionAutoWakeCount must NOT be called — we're past the cap.
	// StartDesktop must NOT be called either. gomock fails on unexpected calls.

	s.server.maybeAutoWake(context.Background(), stuck)

	s.Require().NotNil(captured)
	s.Equal(types.InteractionStateError, captured.State)
	s.Contains(captured.Error, "helixml/helix#2397")
}

// TestSkipsWhenInteractionIsYoung: stuck row younger than threshold must
// not trigger any wake-up — even with no WS — so we don't burn cap on
// genuinely-still-booting sessions.
func (s *AutoWakeColdStartSuite) TestSkipsWhenInteractionIsYoung() {
	stuck := &types.Interaction{
		ID:            "int-3",
		SessionID:     "ses_young",
		State:         types.InteractionStateWaiting,
		PromptMessage: "do the thing",
		Created:       time.Now().Add(-1 * time.Second), // way younger than threshold
		AutoWakeCount: 0,
	}

	// No mocks set — gomock fails if any store method is called.
	s.server.maybeAutoWake(context.Background(), stuck)
}
