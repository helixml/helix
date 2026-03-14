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
		sessionToWaitingInteraction: make(map[string]string),
		requestToSessionMapping:     make(map[string]string),
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

	// GetSession for the session (used to load session + check interactions)
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
