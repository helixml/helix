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
	// ExternalAgentStatus is empty: no StartDesktop in flight, so the new
	// container-state-aware gate falls through to the kick path.
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

// TestSkipsBudgetWhileStartDesktopInFlight: when ExternalAgentStatus is
// "starting" and the interaction is still inside the cold-start grace
// period, maybeKickColdStart must skip without bumping AutoWakeCount or
// invoking StartDesktop. This is the fix for the
// spt_01kreb7sevt5ecyagxhctv3ejh failure mode (container booting normally,
// but retry budget burned before WS connect).
func (s *AutoWakeColdStartSuite) TestSkipsBudgetWhileStartDesktopInFlight() {
	// Old enough to clear the SQL stuck threshold (60s default), but
	// firmly inside the 5-minute cold-start grace window — exactly the
	// regime that used to burn the retry budget for nothing while the
	// boot was still legitimately running.
	stuck := &types.Interaction{
		ID:            "int-starting",
		SessionID:     "ses_starting",
		State:         types.InteractionStateWaiting,
		PromptMessage: "do the thing",
		Created:       time.Now().Add(-90 * time.Second),
		AutoWakeCount: 0,
	}

	session := &types.Session{
		ID:    "ses_starting",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType:           "zed_external",
			ProjectID:           "prj_x",
			ExternalAgentStatus: "starting",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_starting").Return(session, nil).Times(1)

	// IncrementInteractionAutoWakeCount and StartDesktop must NOT be called —
	// gomock fails the test on unexpected calls.

	s.server.maybeAutoWake(context.Background(), stuck)
}

// TestSkipsBudgetWhileRunningButNoWS: regression for
// spt_01ktnvz9y1grjqaaa1rq72z5tx. StartDesktop flips
// ExternalAgentStatus to "running" as soon as the container +
// desktop-bridge are reachable (~T+25s on cold boot), but Zed inside
// the container doesn't dial the external-agent WebSocket back to the
// API until GNOME + claude-agent-acp have come up (typically T+90–120s).
// The grace-period gate has to defer during that "running, no WS" gap
// or the worker burns the retry budget before Zed ever connects.
func (s *AutoWakeColdStartSuite) TestSkipsBudgetWhileRunningButNoWS() {
	// Inside the 5-minute grace, comfortably past the 60s stuck threshold —
	// the regime where the old "starting"-only gate fell through to the kick
	// because status had already flipped to "running".
	stuck := &types.Interaction{
		ID:            "int-running",
		SessionID:     "ses_running",
		State:         types.InteractionStateWaiting,
		PromptMessage: "do the thing",
		Created:       time.Now().Add(-90 * time.Second),
		AutoWakeCount: 0,
	}

	session := &types.Session{
		ID:    "ses_running",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType:           "zed_external",
			ProjectID:           "prj_x",
			ExternalAgentStatus: "running",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_running").Return(session, nil).Times(1)

	// IncrementInteractionAutoWakeCount and StartDesktop must NOT be called —
	// gomock fails the test on unexpected calls. That's the whole point: the
	// gate is supposed to defer without touching the budget.

	s.server.maybeAutoWake(context.Background(), stuck)
}

// TestKicksAfterColdStartGraceExpires: if a "starting" status persists
// past the grace period, fall through to the normal kick + budget burn
// path so a genuinely-stuck boot eventually surfaces as state=error
// instead of hanging forever.
func (s *AutoWakeColdStartSuite) TestKicksAfterColdStartGraceExpires() {
	// Force a tiny grace period via the env override so we don't have to
	// wait minutes for this test to mature.
	s.T().Setenv("HELIX_COLD_START_GRACE_SECONDS", "1")

	stuck := &types.Interaction{
		ID:            "int-grace-expired",
		SessionID:     "ses_grace_expired",
		State:         types.InteractionStateWaiting,
		PromptMessage: "do the thing",
		// Older than both stuck threshold (60s) AND the 1-second grace.
		Created:       time.Now().Add(-5 * time.Minute),
		AutoWakeCount: 0,
	}

	session := &types.Session{
		ID:    "ses_grace_expired",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType:           "zed_external",
			ProjectID:           "prj_x",
			ExternalAgentStatus: "starting", // stuck in starting past grace
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_grace_expired").Return(session, nil).AnyTimes()
	s.store.EXPECT().ListGitRepositories(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(&types.Session{}, nil).AnyTimes()

	// Now we DO expect the kick to fire and the budget to burn.
	s.store.EXPECT().IncrementInteractionAutoWakeCount(gomock.Any(), "int-grace-expired").Return(1, nil)

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
		// Good — past the grace, the kick fires.
	case <-time.After(2 * time.Second):
		s.FailNow("StartDesktop should have been invoked once grace expired")
	}
}

// TestMarksAsErrorAfterMaxRetries: once AutoWakeCount has hit the cap,
// further scans must mark the interaction state=error and stop kicking.
func (s *AutoWakeColdStartSuite) TestMarksAsErrorAfterMaxRetries() {
	stuck := stuckInteraction("int-2", "ses_exhausted", autoWakeMaxRetries)

	// GetSession is now consulted by maybeKickColdStart's container-state
	// gate. Return a session whose ExternalAgentStatus is not "starting",
	// so the gate falls through to the existing exhausted-cap path.
	session := &types.Session{
		ID: "ses_exhausted",
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_exhausted").Return(session, nil).Times(1)

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
