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

	s.server.handleCommentTimeout(context.Background(), "ses-terminal", "comment-terminal")
}

// TestHandleCommentTimeout_StampsErrorWhenInteractionEmpty is the
// regression guard for the genuine no-response case: the comment was sent
// to the agent, two minutes passed, AND the interaction has no content and
// is still waiting (the agent really did go silent — desktop died, network
// dropped, whatever). In that case the user must still see the
// "try sending your comment again" message so they know to act.
func (s *CommentTimerSuite) TestHandleCommentTimeout_StampsErrorWhenInteractionEmpty() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-silent",
		ReviewID:      "review-silent",
		RequestID:     "req-silent",
		InteractionID: "int-silent",
	}
	silentInteraction := &types.Interaction{
		ID:              "int-silent",
		SessionID:       "ses-silent",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
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
	// processNextCommentInQueue is spawned in a goroutine after the
	// update — it calls IsCommentBeingProcessedForSession. Allow that
	// follow-up call to return any sensible result without coupling this
	// test to queue internals.
	s.store.EXPECT().IsCommentBeingProcessedForSession(gomock.Any(), "ses-silent").
		Return(false, nil).AnyTimes()
	// Empty queue is signalled as an error in the real store
	// (gorm.ErrRecordNotFound); a nil comment + nil err would nil-deref.
	s.store.EXPECT().GetNextQueuedCommentForSession(gomock.Any(), "ses-silent").
		Return(nil, errNotFound{}).AnyTimes()

	s.server.handleCommentTimeout(context.Background(), "ses-silent", "comment-silent")
	// Let the goroutine spawn settle so gomock's strict mode evaluates it.
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
