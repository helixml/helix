package server

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

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
		sessionCommentTimeout:  make(map[string]*time.Timer),
		externalAgentWSManager: NewExternalAgentWSManager(),
	}
}

// disarmCommentTimers stops any timer a re-arming branch scheduled so it cannot
// fire against a torn-down gomock controller after the test returns.
func (s *CommentTimerSuite) disarmCommentTimers() {
	s.server.sessionCommentMutex.Lock()
	defer s.server.sessionCommentMutex.Unlock()
	for id, timer := range s.server.sessionCommentTimeout {
		if timer != nil {
			timer.Stop()
		}
		delete(s.server.sessionCommentTimeout, id)
	}
}

func (s *CommentTimerSuite) TearDownTest() {
	s.disarmCommentTimers()
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

	s.server.handleCommentTimeout(context.Background(), "ses-streaming", "comment-streaming")

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
		Updated:         time.Now().Add(-3 * commentTimerInterval), // stale
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-stalled").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-stalled").
		Return(stalledInteraction, nil).AnyTimes()
	s.store.EXPECT().GetInteractionByExternalAgentRequestID(gomock.Any(), "req-stalled").
		Return(nil, errNotFound{})
	s.store.EXPECT().GetCommentByAgentRequestIDs(gomock.Any(), []string{"req-stalled"}).
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

	s.server.handleCommentTimeout(context.Background(), "ses-stalled", "comment-stalled")
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
	s.store.EXPECT().GetInteractionByExternalAgentRequestID(gomock.Any(), "req-terminal").
		Return(nil, errNotFound{})
	s.store.EXPECT().GetCommentByAgentRequestIDs(gomock.Any(), []string{"req-terminal"}).
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

	s.server.handleCommentTimeout(context.Background(), "ses-terminal", "comment-terminal")
}

// TestHandleCommentTimeout_DoesNotStampWhenAgentUnreachable is the regression
// guard for the cold-start false positive that this whole change exists to fix.
// On a reaped session the comment itself is what boots the desktop container;
// two minutes later the container is still coming up, so the interaction is
// legitimately empty and the agent's WebSocket is not connected. The timer must
// re-arm and must NOT tell the user the agent refused to answer.
func (s *CommentTimerSuite) TestHandleCommentTimeout_DoesNotStampWhenAgentUnreachable() {
	queuedAt := time.Now().Add(-2 * commentTimerInterval)
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-cold",
		ReviewID:      "review-cold",
		RequestID:     "req-cold",
		InteractionID: "int-cold",
		QueuedAt:      &queuedAt,
	}
	bootingInteraction := &types.Interaction{
		ID:              "int-cold",
		SessionID:       "ses-cold",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
		Updated:         time.Now(),
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-cold").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-cold").
		Return(bootingInteraction, nil)
	// No UpdateSpecTaskDesignReviewComment: gomock's strict mode fails the test
	// if the stamp is written. That assertion IS the point of this test.

	s.server.handleCommentTimeout(context.Background(), "ses-cold", "comment-cold")

	s.NotNil(s.server.sessionCommentTimeout["ses-cold"], "timer must be re-armed while the sandbox is still starting")
	s.Empty(comment.AgentResponse, "no message may be written while the agent is unreachable")
	s.Equal("req-cold", comment.RequestID, "the comment is still legitimately in flight")
}

// TestHandleCommentTimeout_DoesNotStampWhenCommentNotYetDispatched covers the
// window before the queue has even created an interaction for the comment —
// RequestID still holds the prompt-id placeholder. There is nothing to observe
// about the agent yet, so the timer must re-arm rather than stamp.
func (s *CommentTimerSuite) TestHandleCommentTimeout_DoesNotStampWhenCommentNotYetDispatched() {
	queuedAt := time.Now().Add(-2 * commentTimerInterval)
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-undispatched",
		ReviewID:      "review-undispatched",
		RequestID:     "prompt-undispatched",
		InteractionID: "",
		QueuedAt:      &queuedAt,
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-undispatched").
		Return(comment, nil)
	// No GetInteraction (there is none) and no update.

	s.server.handleCommentTimeout(context.Background(), "ses-undispatched", "comment-undispatched")

	s.NotNil(s.server.sessionCommentTimeout["ses-undispatched"], "timer must be re-armed before dispatch")
	s.Empty(comment.AgentResponse)
}

// TestHandleCommentTimeout_StampsSandboxFailureAfterCeiling is the other half of
// the cold-start branch: waiting forever would wedge the session's comment
// queue, so past commentSandboxStartCeiling we do stamp — but with a message
// that names the real cause (the sandbox), not one that blames the agent.
func (s *CommentTimerSuite) TestHandleCommentTimeout_StampsSandboxFailureAfterCeiling() {
	queuedAt := time.Now().Add(-(commentSandboxStartCeiling + time.Minute))
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-deadbox",
		ReviewID:      "review-deadbox",
		RequestID:     "req-deadbox",
		InteractionID: "int-deadbox",
		QueuedAt:      &queuedAt,
	}
	emptyInteraction := &types.Interaction{
		ID:              "int-deadbox",
		SessionID:       "ses-deadbox",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
		Updated:         time.Now(),
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-deadbox").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-deadbox").
		Return(emptyInteraction, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal(CommentSandboxNotStartedMessage, c.AgentResponse,
				"a sandbox that never started must not be reported as the agent refusing to answer")
			s.Empty(c.RequestID, "RequestID must be cleared so the session's comment queue is not blocked")
			s.Nil(c.QueuedAt, "QueuedAt must be cleared")
			return nil
		},
	)
	// processNextCommentInQueue is spawned in a goroutine after the update.
	s.store.EXPECT().IsCommentBeingProcessedForSession(gomock.Any(), "ses-deadbox").
		Return(false, nil).AnyTimes()
	s.store.EXPECT().GetNextQueuedCommentForSession(gomock.Any(), "ses-deadbox").
		Return(nil, errNotFound{}).AnyTimes()

	s.server.handleCommentTimeout(context.Background(), "ses-deadbox", "comment-deadbox")
	// Let the goroutine spawn settle so gomock's strict mode evaluates it.
	time.Sleep(50 * time.Millisecond)
}

// markAgentConnected registers a live external-agent connection for a session
// so handleCommentTimeout takes its "agent is reachable" branch. A nil websocket
// is fine — the timer only asks whether a connection exists.
func (s *CommentTimerSuite) markAgentConnected(sessionID string) {
	s.server.externalAgentWSManager.mu.Lock()
	defer s.server.externalAgentWSManager.mu.Unlock()
	s.server.externalAgentWSManager.connections[sessionID] = &ExternalAgentWSConnection{
		SessionID:   sessionID,
		ConnectedAt: time.Now(),
	}
}

// TestHandleCommentTimeout_ReArmsWhenConnectedAgentHasNoContentYet covers the
// long-turn case on a warm sandbox: the agent is connected and thinking but has
// not emitted its first token. Absence of output is not evidence of refusal, so
// the timer re-arms instead of stamping. This is why the fix is not "raise the
// two minutes" — a 30-minute turn must survive too.
func (s *CommentTimerSuite) TestHandleCommentTimeout_ReArmsWhenConnectedAgentHasNoContentYet() {
	s.markAgentConnected("ses-thinking")
	queuedAt := time.Now().Add(-3 * commentTimerInterval)
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-thinking",
		ReviewID:      "review-thinking",
		RequestID:     "req-thinking",
		InteractionID: "int-thinking",
		QueuedAt:      &queuedAt,
	}
	thinkingInteraction := &types.Interaction{
		ID:              "int-thinking",
		SessionID:       "ses-thinking",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
		Updated:         time.Now().Add(-3 * commentTimerInterval),
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-thinking").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-thinking").
		Return(thinkingInteraction, nil)
	// No update expected — strict gomock fails the test if a stamp is written.

	s.server.handleCommentTimeout(context.Background(), "ses-thinking", "comment-thinking")

	s.NotNil(s.server.sessionCommentTimeout["ses-thinking"], "timer must re-arm while the agent is working")
	s.Empty(comment.AgentResponse)
}

// TestHandleCommentTimeout_StampsNoResponseAfterSilentCeiling is the one path
// on which CommentTimerNoResponseMessage is true: the agent is demonstrably
// reachable, the interaction is non-terminal, it holds zero bytes, and nothing
// has moved for the whole silent-agent ceiling.
func (s *CommentTimerSuite) TestHandleCommentTimeout_StampsNoResponseAfterSilentCeiling() {
	s.markAgentConnected("ses-mute")
	queuedAt := time.Now().Add(-(commentSilentAgentCeiling + time.Minute))
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-mute",
		ReviewID:      "review-mute",
		RequestID:     "req-mute",
		InteractionID: "int-mute",
		QueuedAt:      &queuedAt,
	}
	muteInteraction := &types.Interaction{
		ID:              "int-mute",
		SessionID:       "ses-mute",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
		Updated:         time.Now().Add(-(commentSilentAgentCeiling + time.Minute)),
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-mute").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-mute").
		Return(muteInteraction, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal(CommentTimerNoResponseMessage, c.AgentResponse)
			s.Empty(c.RequestID, "RequestID must be cleared so the queue is not blocked")
			s.Nil(c.QueuedAt)
			return nil
		},
	)
	s.store.EXPECT().IsCommentBeingProcessedForSession(gomock.Any(), "ses-mute").
		Return(false, nil).AnyTimes()
	s.store.EXPECT().GetNextQueuedCommentForSession(gomock.Any(), "ses-mute").
		Return(nil, errNotFound{}).AnyTimes()

	s.server.handleCommentTimeout(context.Background(), "ses-mute", "comment-mute")
	time.Sleep(50 * time.Millisecond)
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

	s.server.handleCommentTimeout(context.Background(), "ses-resolved", "comment-resolved")
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

	s.store.EXPECT().GetInteractionByExternalAgentRequestID(gomock.Any(), "req-repair").
		Return(nil, errNotFound{})
	s.store.EXPECT().GetCommentByAgentRequestIDs(gomock.Any(), []string{"req-repair"}).
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

	s.store.EXPECT().GetInteractionByExternalAgentRequestID(gomock.Any(), "req-happy").
		Return(nil, errNotFound{})
	s.store.EXPECT().GetCommentByAgentRequestIDs(gomock.Any(), []string{"req-happy"}).
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

// TestFinalizeCommentResponse_RepairsAfterTimerStampClearedRequestID is the
// direct regression guard for the data loss. handleCommentTimeout clears
// request_id when it stamps, so a late message_completed cannot find the comment
// by request_id at all. The resolver must still find it via interaction_id and
// overwrite the stamp with the real answer.
func (s *CommentTimerSuite) TestFinalizeCommentResponse_RepairsAfterTimerStampClearedRequestID() {
	stamped := &types.SpecTaskDesignReviewComment{
		ID:            "comment-stranded",
		ReviewID:      "review-stranded",
		RequestID:     "", // blanked by the timer when it stamped
		InteractionID: "int-stranded",
		AgentResponse: CommentTimerNoResponseMessage,
	}
	completed := &types.Interaction{
		ID:              "int-stranded",
		State:           types.InteractionStateComplete,
		ResponseMessage: "The answer the user was told did not exist.",
	}

	s.store.EXPECT().GetInteractionByExternalAgentRequestID(gomock.Any(), "int-stranded").
		Return(completed, nil)
	s.store.EXPECT().GetCommentByAgentRequestIDs(gomock.Any(), []string{"int-stranded"}).
		Return(stamped, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-stranded").
		Return(completed, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("The answer the user was told did not exist.", c.AgentResponse,
				"a late completion must repair a comment whose request_id the timer already cleared")
			return nil
		},
	)
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-stranded").
		Return(nil, errNotFound{}).AnyTimes()

	err := s.server.finalizeCommentResponse(context.Background(), "int-stranded")
	s.NoError(err)
}

// TestFinalizeCommentResponse_ResolvesWhenAgentRebindsRequestID covers the
// second, independent break found by the cold-start reproduction: the agent
// rebinds the interaction's ExternalAgentRequestID mid-turn, so the completion
// arrives under a `req_…` id the comment never stored. Resolution must normalise
// that id back to the interaction before looking the comment up.
func (s *CommentTimerSuite) TestFinalizeCommentResponse_ResolvesWhenAgentRebindsRequestID() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-rebound",
		ReviewID:      "review-rebound",
		RequestID:     "int-rebound", // stored at dispatch, before the rebind
		InteractionID: "int-rebound",
	}
	completed := &types.Interaction{
		ID:              "int-rebound",
		State:           types.InteractionStateComplete,
		ResponseMessage: "Answer delivered under a rebound request id.",
	}

	// message_completed arrives keyed by the agent's own request id.
	s.store.EXPECT().GetInteractionByExternalAgentRequestID(gomock.Any(), "req-rebound").
		Return(completed, nil)
	s.store.EXPECT().GetCommentByAgentRequestIDs(gomock.Any(), []string{"req-rebound", "int-rebound"}).
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-rebound").
		Return(completed, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("Answer delivered under a rebound request id.", c.AgentResponse)
			return nil
		},
	)
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-rebound").
		Return(nil, errNotFound{}).AnyTimes()

	err := s.server.finalizeCommentResponse(context.Background(), "req-rebound")
	s.NoError(err)
}

// TestFinalizeCommentResponse_SurfacesInteractionErrorWhenNoOutput covers a
// turn that failed before producing anything. The interaction knows exactly why
// (observed live: "Agent never connected after auto-wake cold-start retries"),
// so the comment must carry that reason rather than an empty response box.
func (s *CommentTimerSuite) TestFinalizeCommentResponse_SurfacesInteractionErrorWhenNoOutput() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-errored",
		ReviewID:      "review-errored",
		RequestID:     "int-errored",
		InteractionID: "int-errored",
	}
	failed := &types.Interaction{
		ID:              "int-errored",
		State:           types.InteractionStateError,
		ResponseMessage: "",
		Error:           "Agent never connected after auto-wake cold-start retries",
	}

	s.store.EXPECT().GetInteractionByExternalAgentRequestID(gomock.Any(), "int-errored").
		Return(failed, nil)
	s.store.EXPECT().GetCommentByAgentRequestIDs(gomock.Any(), []string{"int-errored"}).
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-errored").
		Return(failed, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("[Agent turn failed: Agent never connected after auto-wake cold-start retries]", c.AgentResponse,
				"a failed turn must explain itself instead of leaving the comment blank")
			return nil
		},
	)
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-errored").
		Return(nil, errNotFound{}).AnyTimes()

	err := s.server.finalizeCommentResponse(context.Background(), "int-errored")
	s.NoError(err)
}

// TestFinalizeCommentResponse_ReturnsSentinelWhenNoComment pins the distinction
// Defect 3 depends on: a non-comment interaction must be identifiable as such,
// so the caller can log it at DEBUG while treating every other failure as a lost
// answer.
func (s *CommentTimerSuite) TestFinalizeCommentResponse_ReturnsSentinelWhenNoComment() {
	s.store.EXPECT().GetInteractionByExternalAgentRequestID(gomock.Any(), "req-orphan").
		Return(nil, errNotFound{})
	s.store.EXPECT().GetCommentByAgentRequestIDs(gomock.Any(), []string{"req-orphan"}).
		Return(nil, errNotFound{})

	err := s.server.finalizeCommentResponse(context.Background(), "req-orphan")
	s.Error(err)
	s.ErrorIs(err, ErrNoCommentForAgentRequest)
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
	s.store.EXPECT().GetInteractionByExternalAgentRequestID(gomock.Any(), "req-zombie").
		Return(nil, errNotFound{})
	s.store.EXPECT().GetCommentByAgentRequestIDs(gomock.Any(), []string{"req-zombie"}).
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

// TestReconcileTimerStampedComments_RepairsMatchingRows covers the recovery of
// the historical backlog: a comment carrying a placeholder stamp whose linked
// interaction finished with real content gets the answer copied across, with
// agent_response_at taken from the interaction rather than "now" (the answer is
// historical).
func (s *CommentTimerSuite) TestReconcileTimerStampedComments_RepairsMatchingRows() {
	answeredAt := time.Date(2026, 3, 9, 11, 40, 8, 0, time.UTC)
	stamps := []string{CommentTimerNoResponseMessage, CommentSandboxNotStartedMessage}

	first := s.store.EXPECT().ListTimerStampedCommentsWithResponses(gomock.Any(), stamps, timerStampedRepairBatchSize).
		Return([]store.TimerStampedCommentRepair{{
			CommentID:          "comment-stranded",
			InteractionID:      "int-stranded",
			InteractionState:   string(types.InteractionStateComplete),
			ResponseMessage:    "Recovered answer.",
			InteractionUpdated: answeredAt,
		}}, nil)
	// Second pass sees nothing: the predicate stops matching once repaired.
	s.store.EXPECT().ListTimerStampedCommentsWithResponses(gomock.Any(), stamps, timerStampedRepairBatchSize).
		Return(nil, nil).After(first)

	s.store.EXPECT().RepairTimerStampedComment(gomock.Any(), "comment-stranded", "Recovered answer.", gomock.Any(), answeredAt).
		Return(nil)

	s.server.reconcileTimerStampedComments(context.Background())
}

// TestReconcileTimerStampedComments_NoopWhenNothingStranded is the idempotency
// guard: a second run over already-repaired data must write nothing.
func (s *CommentTimerSuite) TestReconcileTimerStampedComments_NoopWhenNothingStranded() {
	s.store.EXPECT().ListTimerStampedCommentsWithResponses(gomock.Any(), gomock.Any(), timerStampedRepairBatchSize).
		Return(nil, nil)
	// No RepairTimerStampedComment expected.

	s.server.reconcileTimerStampedComments(context.Background())
}

// TestReconcileTimerStampedComments_LeavesStampWhenInteractionYieldsNoText
// guards against blanking a stamp we cannot replace: if the interaction has a
// response_message but TextFromInteraction resolves to nothing (entries take
// precedence and are empty), the row is left alone rather than wiped.
func (s *CommentTimerSuite) TestReconcileTimerStampedComments_LeavesStampWhenInteractionYieldsNoText() {
	s.store.EXPECT().ListTimerStampedCommentsWithResponses(gomock.Any(), gomock.Any(), timerStampedRepairBatchSize).
		Return([]store.TimerStampedCommentRepair{{
			CommentID:       "comment-empty",
			InteractionID:   "int-empty",
			ResponseMessage: "",
			ResponseEntries: datatypes.JSON([]byte(`[]`)),
		}}, nil)
	// No RepairTimerStampedComment, and no second list pass — a batch that
	// repairs nothing must stop rather than loop on the same rows forever.

	s.server.reconcileTimerStampedComments(context.Background())
}

// errNotFound is a trivial sentinel returned to short-circuit the
// finalizeCommentResponse continuation logic without dragging in the
// gorm package just for ErrRecordNotFound. The function only checks for
// non-nil err; the specific type doesn't matter.
type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }
