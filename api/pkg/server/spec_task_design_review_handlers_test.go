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
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-streaming").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-streaming").
		Return(streamingInteraction, nil)
	// Critical: NO UpdateSpecTaskDesignReviewComment call is expected.
	// gomock's strict mode fails the test if any unexpected call is made.

	s.server.handleCommentTimeout(context.Background(), "ses-streaming", "comment-streaming")
}

// TestHandleCommentTimeout_SkipsErrorWhenInteractionIsTerminal covers the
// race where message_completed has already finalized the interaction
// (state=complete) but the comment row hasn't been written yet — between
// the interaction Save and the comment Save. The timer must not stamp the
// error in that micro-window either.
func (s *CommentTimerSuite) TestHandleCommentTimeout_SkipsErrorWhenInteractionIsTerminal() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:            "comment-terminal",
		RequestID:     "req-terminal",
		InteractionID: "int-terminal",
	}
	terminalInteraction := &types.Interaction{
		ID:        "int-terminal",
		SessionID: "ses-terminal",
		State:     types.InteractionStateComplete,
		// Even with empty body: state=complete means the run is over,
		// finalizeCommentResponse owns the comment from here.
	}

	s.store.EXPECT().GetSpecTaskDesignReviewComment(gomock.Any(), "comment-terminal").
		Return(comment, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-terminal").
		Return(terminalInteraction, nil)

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

// errNotFound is a trivial sentinel returned to short-circuit the
// finalizeCommentResponse continuation logic without dragging in the
// gorm package just for ErrRecordNotFound. The function only checks for
// non-nil err; the specific type doesn't matter.
type errNotFound struct{}

func (errNotFound) Error() string { return "not found" }
