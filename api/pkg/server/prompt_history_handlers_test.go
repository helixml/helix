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

// PromptHistoryHandlersSuite tests processPendingPromptsForIdleSessions and related helpers.
type PromptHistoryHandlersSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestPromptHistoryHandlersSuite(t *testing.T) {
	suite.Run(t, new(PromptHistoryHandlersSuite))
}

func (s *PromptHistoryHandlersSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)

	s.server = &HelixAPIServer{
		Cfg: &config.ServerConfig{
			WebServer: config.WebServer{
				URL:         "http://localhost:0",
				Host:        "localhost",
				Port:        0,
				RunnerToken: "test-runner-token",
			},
		},
		Store:  s.store,
		pubsub: pubsub.NewNoop(),
		Controller: &controller.Controller{
			Options: controller.Options{
				Store:  s.store,
				PubSub: pubsub.NewNoop(),
			},
		},
		externalAgentWSManager:      NewExternalAgentWSManager(),
		externalAgentRunnerManager:  NewExternalAgentRunnerManager(),
		contextMappings:             make(map[string]string),
		requestToSessionMapping:     make(map[string]string),
		requestToInteractionMapping: make(map[string]string),
		pendingCancelChannels:       make(map[string]chan string),
		externalAgentSessionMapping: make(map[string]string),
		externalAgentUserMapping:    make(map[string]string),
		sessionCommentTimeout:       make(map[string]*time.Timer),
		requestToCommenterMapping:   make(map[string]string),
		streamingContexts:           make(map[string]*streamingContext),
		streamingRateLimiter:        make(map[string]time.Time),
	}
}

func (s *PromptHistoryHandlersSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestProcessPendingPromptsForIdleSessions_IdleSession_QueueOnly verifies Bug 2 fix:
// When a session is idle and only queue-mode (interrupt=false) prompts are pending,
// processPendingPromptsForIdleSessions must dispatch via processPromptQueue
// (which calls GetNextPendingPrompt), NOT via processAnyPendingPrompt
// (which calls GetAnyPendingPrompt).
func (s *PromptHistoryHandlersSuite) TestProcessPendingPromptsForIdleSessions_IdleSession_QueueOnly() {
	sessionID := "ses_queue_only"

	// One pending queue-mode prompt for the session
	pendingEntry := &types.PromptHistoryEntry{
		ID:        "prompt-q1",
		SessionID: sessionID,
		Status:    "pending",
		Interrupt: false, // queue-mode
		Content:   "queue message",
	}

	// ListPromptHistoryBySpecTask returns the pending entry
	s.store.EXPECT().
		ListPromptHistoryBySpecTask(gomock.Any(), "task-123").
		Return([]*types.PromptHistoryEntry{pendingEntry}, nil)

	// GetSpecTask is called to determine the canonical planning session (fix #10b)
	s.store.EXPECT().
		GetSpecTask(gomock.Any(), "task-123").
		Return(&types.SpecTask{ID: "task-123", PlanningSessionID: sessionID}, nil)

	// GetSession for the session (used to load session + check interactions)
	session := &types.Session{
		ID:           sessionID,
		Owner:        "user-1",
		GenerationID: 0,
	}
	// GetSession + ListInteractions called by both processPendingPromptsForIdleSessions
	// (to check if session is idle) AND processPromptQueue (to check if session is busy)
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil).AnyTimes()
	s.store.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return([]*types.Interaction{}, int64(0), nil).AnyTimes()

	// processPromptQueue calls GetNextPendingPrompt — return nil to stop further processing
	s.store.EXPECT().
		GetNextPendingPrompt(gomock.Any(), sessionID).
		Return(nil, nil)

	// GetAnyPendingPrompt must NOT be called (gomock enforces this: any unexpected call fails)

	s.server.processPendingPromptsForIdleSessions(context.Background(), "task-123")
}

// TestProcessPendingPromptsForIdleSessions_IdleSession_InterruptOnly verifies that
// interrupt-mode messages are dispatched via processInterruptPrompt when idle.
func (s *PromptHistoryHandlersSuite) TestProcessPendingPromptsForIdleSessions_IdleSession_InterruptOnly() {
	sessionID := "ses_interrupt_only"

	pendingEntry := &types.PromptHistoryEntry{
		ID:        "prompt-i1",
		SessionID: sessionID,
		Status:    "pending",
		Interrupt: true, // interrupt-mode
		Content:   "interrupt message",
	}

	s.store.EXPECT().
		ListPromptHistoryBySpecTask(gomock.Any(), "task-456").
		Return([]*types.PromptHistoryEntry{pendingEntry}, nil)

	// GetSpecTask is called to determine the canonical planning session (fix #10b)
	s.store.EXPECT().
		GetSpecTask(gomock.Any(), "task-456").
		Return(&types.SpecTask{ID: "task-456", PlanningSessionID: sessionID}, nil)

	session := &types.Session{
		ID:           sessionID,
		Owner:        "user-1",
		GenerationID: 0,
	}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)

	// ListInteractions returns empty → session is idle
	s.store.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return([]*types.Interaction{}, int64(0), nil)

	// processInterruptPrompt calls GetNextInterruptPrompt — return nil to stop
	s.store.EXPECT().
		GetNextInterruptPrompt(gomock.Any(), sessionID).
		Return(nil, nil)

	s.server.processPendingPromptsForIdleSessions(context.Background(), "task-456")
}

// TestProcessPendingPromptsForIdleSessions_BusySession_QueueOnly verifies that
// queue-mode messages are NOT dispatched when the session is busy (has a Waiting interaction).
func (s *PromptHistoryHandlersSuite) TestProcessPendingPromptsForIdleSessions_BusySession_QueueOnly() {
	sessionID := "ses_busy_queue"

	pendingEntry := &types.PromptHistoryEntry{
		ID:        "prompt-bq1",
		SessionID: sessionID,
		Status:    "pending",
		Interrupt: false,
		Content:   "queued while busy",
	}

	s.store.EXPECT().
		ListPromptHistoryBySpecTask(gomock.Any(), "task-789").
		Return([]*types.PromptHistoryEntry{pendingEntry}, nil)

	// GetSpecTask is called to determine the canonical planning session (fix #10b)
	s.store.EXPECT().
		GetSpecTask(gomock.Any(), "task-789").
		Return(&types.SpecTask{ID: "task-789", PlanningSessionID: sessionID}, nil)

	session := &types.Session{
		ID:           sessionID,
		Owner:        "user-1",
		GenerationID: 0,
	}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)

	// ListInteractions returns a Waiting interaction → session is busy
	waitingInteraction := &types.Interaction{
		ID:        "int-busy",
		SessionID: sessionID,
		State:     types.InteractionStateWaiting,
	}
	s.store.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return([]*types.Interaction{waitingInteraction}, int64(1), nil)

	// Neither GetNextPendingPrompt nor GetAnyPendingPrompt should be called
	// (gomock enforces this automatically — any unexpected call fails the test)

	s.server.processPendingPromptsForIdleSessions(context.Background(), "task-789")
}

// TestMarkCanonicalSessionStartingForSync_NoWS_MarksStarting verifies that
// when a chat is sent to a session whose desktop has no live WebSocket,
// syncPromptHistory's helper flips external_agent_status to "starting"
// synchronously before the wake goroutine fires — so the frontend's first
// refetch returns a row that agrees with the optimistic cache write.
// Spec: design/tasks/002047_yet-again-sending-a/.
func (s *PromptHistoryHandlersSuite) TestMarkCanonicalSessionStartingForSync_NoWS_MarksStarting() {
	sessionID := "ses_no_ws"
	specTaskID := "spt_idle"

	s.store.EXPECT().
		GetSpecTask(gomock.Any(), specTaskID).
		Return(&types.SpecTask{ID: specTaskID, PlanningSessionID: sessionID}, nil)

	// No WS registered for this session — manager returns (nil, false).
	// MarkSessionStartingIfIdle must then be called and return true (row was idle).
	s.store.EXPECT().
		MarkSessionStartingIfIdle(gomock.Any(), sessionID).
		Return(true, nil).Times(1)

	s.server.markCanonicalSessionStartingForSync(context.Background(), specTaskID)
}

// TestMarkCanonicalSessionStartingForSync_LiveWS_SkipsMark verifies that
// when a WS is already live the helper does not touch the row — the
// existing socket will deliver the prompt without any boot.
func (s *PromptHistoryHandlersSuite) TestMarkCanonicalSessionStartingForSync_LiveWS_SkipsMark() {
	sessionID := "ses_live_ws"
	specTaskID := "spt_live"

	s.store.EXPECT().
		GetSpecTask(gomock.Any(), specTaskID).
		Return(&types.SpecTask{ID: specTaskID, PlanningSessionID: sessionID}, nil)

	// Register a live WS for the session. Pass a non-nil placeholder
	// connection so the registration sticks.
	s.server.externalAgentWSManager.registerConnection(sessionID, &ExternalAgentWSConnection{})

	// MarkSessionStartingIfIdle must NOT be called — gomock enforces.
	s.server.markCanonicalSessionStartingForSync(context.Background(), specTaskID)
}

// TestMarkCanonicalSessionStartingForSync_NoPlanningSession_NoOp verifies that
// the helper bails cleanly when the spec task has no planning session yet
// (typical right after creation, before the planning session is wired up).
func (s *PromptHistoryHandlersSuite) TestMarkCanonicalSessionStartingForSync_NoPlanningSession_NoOp() {
	specTaskID := "spt_no_planning"

	s.store.EXPECT().
		GetSpecTask(gomock.Any(), specTaskID).
		Return(&types.SpecTask{ID: specTaskID, PlanningSessionID: ""}, nil)

	// MarkSessionStartingIfIdle must NOT be called.
	s.server.markCanonicalSessionStartingForSync(context.Background(), specTaskID)
}

// TestMarkCanonicalSessionStartingForSync_AlreadyStarting_NoUpdate verifies that
// the helper still calls MarkSessionStartingIfIdle (which is a no-op at the
// DB level because of its WHERE guard) and logs the no-update outcome.
func (s *PromptHistoryHandlersSuite) TestMarkCanonicalSessionStartingForSync_AlreadyStarting_NoUpdate() {
	sessionID := "ses_already_starting"
	specTaskID := "spt_already"

	s.store.EXPECT().
		GetSpecTask(gomock.Any(), specTaskID).
		Return(&types.SpecTask{ID: specTaskID, PlanningSessionID: sessionID}, nil)

	// Helper falls into the no-update branch when the WHERE guard skipped
	// the row.
	s.store.EXPECT().
		MarkSessionStartingIfIdle(gomock.Any(), sessionID).
		Return(false, nil).Times(1)

	s.server.markCanonicalSessionStartingForSync(context.Background(), specTaskID)
}
