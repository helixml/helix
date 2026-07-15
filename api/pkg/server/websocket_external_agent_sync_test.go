package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/server/wsprotocol"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"
)

// WebSocketSyncSuite tests the individual sync handler methods on HelixAPIServer
// using gomock MockStore for precise control over store expectations.
type WebSocketSyncSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
}

func TestWebSocketSyncSuite(t *testing.T) {
	suite.Run(t, new(WebSocketSyncSuite))
}

func (s *WebSocketSyncSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)

	// TouchSession is called as a fire-and-forget side effect in
	// handleMessageCompleted and handleMessageAdded (user messages).
	// Allow it anywhere without specific ordering.
	s.store.EXPECT().TouchSession(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// sendQueuedPromptToSession backfills any design-review comment linked to the
	// prompt at dispatch (backfillCommentLinkageForPrompt). For non-comment
	// prompts the lookup returns not-found; allow it anywhere by default. Tests
	// that exercise a comment-linked prompt override this with a specific expect.
	s.store.EXPECT().GetCommentByPromptID(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("record not found")).AnyTimes()

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

func (s *WebSocketSyncSuite) TearDownTest() {
	s.ctrl.Finish()
}

// ──────────────────────────────────────────────────────────────────────────────
// handleThreadCreated tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestThreadCreated_Priority1_RequestIDMapping() {
	// Setup: request_id → session mapping exists
	s.server.requestToSessionMapping["req-123"] = "ses_existing"

	existingSession := &types.Session{
		ID:    "ses_existing",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
		},
	}
	updatedSession := *existingSession
	updatedSession.Metadata.ZedThreadID = "thread-abc"

	s.store.EXPECT().GetSession(gomock.Any(), "ses_existing").Return(existingSession, nil)
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			s.Equal("thread-abc", session.Metadata.ZedThreadID)
			return &session, nil
		},
	)

	syncMsg := &types.SyncMessage{
		EventType: "thread_created",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-abc",
			"title":         "Test Thread",
			"request_id":    "req-123",
		},
	}

	err := s.server.handleThreadCreated("agent-1", syncMsg)
	s.NoError(err)

	// Verify contextMappings populated
	s.Equal("ses_existing", s.server.contextMappings["thread-abc"])

	// Verify requestToSessionMapping entry deleted
	_, exists := s.server.requestToSessionMapping["req-123"]
	s.False(exists, "requestToSessionMapping should be cleaned up")
}

func (s *WebSocketSyncSuite) TestThreadCreated_Priority2_SessionIDReuse() {
	// syncMsg has SessionID set (Helix-initiated request)
	existingSession := &types.Session{
		ID:    "ses_reuse",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
		},
	}

	s.store.EXPECT().GetSession(gomock.Any(), "ses_reuse").Return(existingSession, nil)
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			s.Equal("thread-def", session.Metadata.ZedThreadID)
			return &session, nil
		},
	)

	syncMsg := &types.SyncMessage{
		SessionID: "ses_reuse",
		EventType: "thread_created",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-def",
			"title":         "Reused Session",
		},
	}

	err := s.server.handleThreadCreated("agent-1", syncMsg)
	s.NoError(err)

	s.Equal("ses_reuse", s.server.contextMappings["thread-def"])
}

func (s *WebSocketSyncSuite) TestThreadCreated_Priority3_NewSession() {
	// No request_id mapping, no sessionID → creates new session
	s.server.externalAgentUserMapping["agent-1"] = "user-1"

	// findSessionByZedThreadID check: no existing session with this ZedThreadID
	s.store.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return([]*types.Session{}, int64(0), nil)

	createdSession := &types.Session{
		ID:    "ses_new",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-new",
		},
	}

	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			s.Equal("user-1", session.Owner)
			s.Equal("thread-new", session.Metadata.ZedThreadID)
			return createdSession, nil
		},
	)

	createdInteraction := &types.Interaction{
		ID:        "int-new",
		SessionID: "ses_new",
	}
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(createdInteraction, nil)

	syncMsg := &types.SyncMessage{
		EventType: "thread_created",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-new",
			"title":         "New Conversation",
		},
	}

	err := s.server.handleThreadCreated("agent-1", syncMsg)
	s.NoError(err)

	// Verify contextMappings populated
	s.Equal("ses_new", s.server.contextMappings["thread-new"])
}

func (s *WebSocketSyncSuite) TestThreadCreated_Priority3_SpectaskLink() {
	// sessionID starts with "ses_" and the original has a SpecTaskID
	s.server.externalAgentUserMapping["ses_original"] = "user-1"

	// findSessionByZedThreadID check: no existing session with this ZedThreadID
	s.store.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return([]*types.Session{}, int64(0), nil)

	// First call: no request_id mapping, no syncMsg.SessionID → creates new session
	createdSession := &types.Session{
		ID:    "ses_new_spectask",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-spectask",
		},
	}

	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(createdSession, nil)

	createdInteraction := &types.Interaction{
		ID:        "int-spectask",
		SessionID: "ses_new_spectask",
	}
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(createdInteraction, nil)

	// GetSession for the original session (ses_original)
	originalSession := &types.Session{
		ID:    "ses_original",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			SpecTaskID: "spec-task-123",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_original").Return(originalSession, nil)

	// getAgentNameForSession looks up the spec task and app to determine agent name.
	// For this test, return a spec task with no app (so it defaults to "zed-agent").
	s.store.EXPECT().GetSpecTask(gomock.Any(), "spec-task-123").
		Return(&types.SpecTask{ID: "spec-task-123"}, nil).AnyTimes()

	// UpdateSession to copy SpecTaskID and ZedAgentName
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			s.Equal("spec-task-123", session.Metadata.SpecTaskID)
			s.Equal("zed-agent", session.Metadata.ZedAgentName)
			return &session, nil
		},
	)

	// trackSpecTaskZedThread runs in goroutine and makes several store calls
	// We need to allow them to return errors since we don't care about them here
	s.store.EXPECT().GetSpecTaskZedThreadByZedThreadID(gomock.Any(), "thread-spectask").
		Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().CreateSpecTaskWorkSession(gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("not important")).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "thread_created",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-spectask",
			"title":         "Spectask Thread",
		},
	}

	err := s.server.handleThreadCreated("ses_original", syncMsg)
	s.NoError(err)

	// Give goroutine time to run
	time.Sleep(50 * time.Millisecond)
}

func (s *WebSocketSyncSuite) TestThreadCreated_MissingThreadID() {
	syncMsg := &types.SyncMessage{
		EventType: "thread_created",
		Data:      map[string]interface{}{},
	}

	err := s.server.handleThreadCreated("agent-1", syncMsg)
	s.Error(err)
	s.Contains(err.Error(), "missing or invalid acp_thread_id")
}

func (s *WebSocketSyncSuite) TestThreadCreated_StoreError() {
	s.server.requestToSessionMapping["req-err"] = "ses_err"

	s.store.EXPECT().GetSession(gomock.Any(), "ses_err").
		Return(nil, fmt.Errorf("database connection lost"))

	syncMsg := &types.SyncMessage{
		EventType: "thread_created",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-err",
			"request_id":    "req-err",
		},
	}

	err := s.server.handleThreadCreated("agent-1", syncMsg)
	s.Error(err)
	s.Contains(err.Error(), "database connection lost")
}

// ──────────────────────────────────────────────────────────────────────────────
// handleMessageAdded tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestMessageAdded_AssistantFirstMessage() {
	s.server.contextMappings["thread-1"] = "ses_1"

	session := &types.Session{
		ID:    "ses_1",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_1").Return(session, nil)

	existingInteraction := &types.Interaction{
		ID:              "int-1",
		SessionID:       "ses_1",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{existingInteraction}, int64(1), nil,
	)

	s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ int, responseMessage string, _ datatypes.JSON, _ int, lastZedMessageID string) error {
			s.Equal("Hello from AI", responseMessage)
			s.Equal("msg-1", lastZedMessageID)
			return nil
		},
	)

	// Goroutine calls
	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-1").
		Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses_1").
		Return(nil, nil).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-1",
			"message_id":    "msg-1",
			"content":       "Hello from AI",
			"role":          "assistant",
		},
	}

	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestMessageAdded_AssistantSameMessageID_StreamingUpdate() {
	s.server.contextMappings["thread-2"] = "ses_2"

	session := &types.Session{
		ID:    "ses_2",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_2").Return(session, nil)

	// Interaction already has content from msg-A
	existingInteraction := &types.Interaction{
		ID:                   "int-2",
		SessionID:            "ses_2",
		State:                types.InteractionStateWaiting,
		ResponseMessage:      "Hello",
		LastZedMessageID:     "msg-A",
		LastZedMessageOffset: 0,
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{existingInteraction}, int64(1), nil,
	)

	s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ int, responseMessage string, _ datatypes.JSON, _ int, lastZedMessageID string) error {
			// Same message_id → content replaced from offset (streaming update)
			s.Equal("Hello, world!", responseMessage)
			s.Equal("msg-A", lastZedMessageID)
			return nil
		},
	)

	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-2").
		Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses_2").
		Return(nil, nil).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-2",
			"message_id":    "msg-A",
			"content":       "Hello, world!",
			"role":          "assistant",
		},
	}

	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestMessageAdded_AssistantNewMessageID_MultiEntry() {
	s.server.contextMappings["thread-3"] = "ses_3"

	session := &types.Session{
		ID:    "ses_3",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_3").Return(session, nil)

	// Interaction already has content from msg-A (with structured entries)
	existingInteraction := &types.Interaction{
		ID:                   "int-3",
		SessionID:            "ses_3",
		State:                types.InteractionStateWaiting,
		ResponseMessage:      "First message",
		LastZedMessageID:     "msg-A",
		LastZedMessageOffset: 0,
		ResponseEntries:      []byte(`[{"type":"text","content":"First message","message_id":"msg-A"}]`),
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{existingInteraction}, int64(1), nil,
	)

	s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ int, responseMessage string, _ datatypes.JSON, lastZedMessageOffset int, lastZedMessageID string) error {
			// New message_id → content appended with \n\n separator
			s.Equal("First message\n\nSecond message", responseMessage)
			s.Equal("msg-B", lastZedMessageID)
			s.Equal(len("First message")+2, lastZedMessageOffset)
			return nil
		},
	)

	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-3").
		Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses_3").
		Return(nil, nil).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-3",
			"message_id":    "msg-B",
			"content":       "Second message",
			"role":          "assistant",
		},
	}

	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)
}

// TestMessageAdded_PriorInteractionMessageIDsAreFiltered reproduces the
// cross-interaction response_entries leak surfaced by the e2e RESPONSE
// ENTRIES ISOLATION VALIDATION step in Drone build #1024 (tag 2.11.0).
//
// Scenario: session has a completed prior interaction (int-prior, msg_id
// "msg-A") and a fresh waiting interaction (int-current, no entries yet).
// Zed's flush_streaming_throttle replays msg-A as a message_added event
// targeted at the current thread. Without the prior-id filter the replay
// landed in int-current's response_entries; the validator then rejected the
// build for ISOLATION VIOLATION. The handler must drop the replay.
func (s *WebSocketSyncSuite) TestMessageAdded_PriorInteractionMessageIDsAreFiltered() {
	s.server.contextMappings["thread-leak"] = "ses_leak"

	session := &types.Session{
		ID:    "ses_leak",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_leak").Return(session, nil)

	priorInteraction := &types.Interaction{
		ID:              "int-prior",
		SessionID:       "ses_leak",
		State:           types.InteractionStateComplete,
		ResponseMessage: "Prior turn answer",
		ResponseEntries: []byte(`[{"type":"text","content":"Prior turn answer","message_id":"msg-A"}]`),
	}
	currentInteraction := &types.Interaction{
		ID:        "int-current",
		SessionID: "ses_leak",
		State:     types.InteractionStateWaiting,
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{priorInteraction, currentInteraction}, int64(2), nil,
	)

	// UpdateInteractionStreamingFields is the only place the leak would
	// surface (streaming flushes own the response entries column). Capture
	// every call so we can assert msg-A never appears in int-current's
	// response_entries.
	var lastEntries datatypes.JSON
	s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ int, _ string, responseEntries datatypes.JSON, _ int, _ string) error {
			lastEntries = responseEntries
			return nil
		},
	).AnyTimes()

	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-current").
		Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses_leak").
		Return(nil, nil).AnyTimes()

	// Zed's flush_streaming_throttle replays the prior turn's msg-A while
	// streaming the new turn. Helix routes it to int-current (the only
	// Waiting interaction). Without the filter this would land in
	// int-current.ResponseEntries.
	replay := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-leak",
			"message_id":    "msg-A",
			"content":       "Prior turn answer",
			"role":          "assistant",
		},
	}
	s.NoError(s.server.handleMessageAdded("agent-1", replay))

	if len(lastEntries) > 0 {
		var entries []wsprotocol.ResponseEntry
		s.NoError(json.Unmarshal(lastEntries, &entries))
		for _, e := range entries {
			s.NotEqual("msg-A", e.MessageID,
				"msg-A belongs to int-prior and must never be persisted to int-current")
		}
	}
}

// TestMessageAdded_WrapperRestartRenumberedMessageIDsAreAccepted reproduces the
// empty-response bounce on int_01kqjsrhndcpwb9zv068dn7mv9 (2026-05-01): Zed's
// claude-agent-acp wrapper restarted, message_ids reset to 1 and were reused
// for legitimately new content. The id-only dedup added in 714d6036a dropped
// every new entry because their ids matched a prior interaction's ids; the
// completion fired with empty response and the prompt bounced. Content-aware
// dedup distinguishes a true replay (same id+content) from renumbered new
// content (same id, different content).
func (s *WebSocketSyncSuite) TestMessageAdded_WrapperRestartRenumberedMessageIDsAreAccepted() {
	s.server.contextMappings["thread-restart"] = "ses_restart"

	session := &types.Session{ID: "ses_restart", Owner: "user-1"}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_restart").Return(session, nil)

	// Prior interaction has msg "348" with the OLD response content.
	priorInteraction := &types.Interaction{
		ID:              "int-prior",
		SessionID:       "ses_restart",
		State:           types.InteractionStateComplete,
		ResponseEntries: []byte(`[{"type":"text","content":"old turn from before wrapper restart","message_id":"348"}]`),
	}
	currentInteraction := &types.Interaction{
		ID:        "int-current",
		SessionID: "ses_restart",
		State:     types.InteractionStateWaiting,
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{priorInteraction, currentInteraction}, int64(2), nil,
	)

	var lastResponseMessage string
	var updateCalled bool
	s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ int, responseMessage string, _ datatypes.JSON, _ int, _ string) error {
			lastResponseMessage = responseMessage
			updateCalled = true
			return nil
		},
	).AnyTimes()

	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-current").
		Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses_restart").
		Return(nil, nil).AnyTimes()

	// Wrapper restarted; it sends NEW content under the reused id "348".
	// The old (id-only) filter would have dropped this; content-aware accepts.
	newContent := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-restart",
			"message_id":    "348",
			"content":       "<thinking>\n\n</thinking>",
			"role":          "assistant",
		},
	}
	s.NoError(s.server.handleMessageAdded("agent-1", newContent))

	// The new content must have landed in int-current.
	s.True(updateCalled, "interaction must have been updated")
	s.Contains(lastResponseMessage, "<thinking>",
		"renumbered new content must be accepted, not silently dropped")
}

func (s *WebSocketSyncSuite) TestMessageAdded_UserMessage() {
	s.server.contextMappings["thread-user"] = "ses_user"

	session := &types.Session{
		ID:           "ses_user",
		Owner:        "user-1",
		GenerationID: 0,
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_user").Return(session, nil)

	createdInteraction := &types.Interaction{
		ID:        "int-user-new",
		SessionID: "ses_user",
	}
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			s.Equal("Hello from user", interaction.PromptMessage)
			s.Equal(types.InteractionStateWaiting, interaction.State)
			s.Equal(0, interaction.GenerationID) // Must match session's generation
			return createdInteraction, nil
		},
	)

	syncMsg := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-user",
			"message_id":    "msg-user-1",
			"content":       "Hello from user",
			"role":          "user",
		},
	}

	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

}

// TestMessageAdded_UserMessage_PreCreatedInteractionReuse verifies Bug 1 fix:
// When sendMessageToSpecTaskAgent pre-creates an interaction and sets requestToInteractionMapping,
// the subsequent Zed echo of the user message must NOT create a duplicate interaction and must
// NOT overwrite the mapping. This ensures the assistant response lands in the pre-created interaction.
func (s *WebSocketSyncSuite) TestMessageAdded_UserMessage_PreCreatedInteractionReuse() {
	s.server.contextMappings["thread-spec"] = "ses_spec"

	// Simulate sendMessageToSpecTaskAgent having pre-created an interaction
	preCreatedID := "int-pre-created"
	requestID := preCreatedID // sendMessageToSpecTaskAgent uses interaction ID as request ID
	s.server.requestToSessionMapping[requestID] = "ses_spec"
	s.server.requestToInteractionMapping[requestID] = preCreatedID

	// No store expectations: CreateInteraction must NOT be called, GetSession must NOT be called

	syncMsg := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-spec",
			"message_id":    "msg-echo-1",
			"content":       "Your implementation has been approved",
			"role":          "user",
		},
	}

	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	// requestToInteractionMapping must still contain the pre-created interaction (not removed by echo)
	s.Equal(preCreatedID, s.server.requestToInteractionMapping[requestID],
		"requestToInteractionMapping must not be removed by Zed user-message echo")
}

func (s *WebSocketSyncSuite) TestMessageAdded_ContextMappingMiss_DBFallback() {
	// contextMappings is empty — should fall back to database lookup
	session := &types.Session{
		ID:    "ses_fallback",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-fallback",
		},
	}

	// findSessionByZedThreadID calls ListSessions
	s.store.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return(
		[]*types.Session{session}, int64(1), nil,
	)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_fallback").Return(session, nil)

	createdInteraction := &types.Interaction{
		ID:        "int-fb",
		SessionID: "ses_fallback",
	}
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(createdInteraction, nil)

	syncMsg := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-fallback",
			"message_id":    "msg-fb-1",
			"content":       "User msg after restart",
			"role":          "user",
		},
	}

	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	// Verify contextMappings was restored
	s.Equal("ses_fallback", s.server.contextMappings["thread-fallback"])
}

func (s *WebSocketSyncSuite) TestMessageAdded_MissingFields() {
	tests := []struct {
		name string
		data map[string]interface{}
		want string
	}{
		{
			name: "missing acp_thread_id",
			data: map[string]interface{}{"message_id": "m", "content": "c", "role": "user"},
			want: "missing or invalid acp_thread_id",
		},
		{
			name: "missing message_id",
			data: map[string]interface{}{"acp_thread_id": "t", "content": "c", "role": "user"},
			want: "missing or invalid message_id",
		},
		{
			name: "missing content",
			data: map[string]interface{}{"acp_thread_id": "t", "message_id": "m", "role": "user"},
			want: "missing or invalid content",
		},
		{
			name: "missing role",
			data: map[string]interface{}{"acp_thread_id": "t", "message_id": "m", "content": "c"},
			want: "missing or invalid role",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			syncMsg := &types.SyncMessage{
				EventType: "message_added",
				Data:      tt.data,
			}
			err := s.server.handleMessageAdded("agent-1", syncMsg)
			s.Error(err)
			s.Contains(err.Error(), tt.want)
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// handleMessageCompleted tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestMessageCompleted_Normal() {
	s.server.contextMappings["thread-mc"] = "ses_mc"

	session := &types.Session{
		ID:    "ses_mc",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_mc").Return(session, nil).AnyTimes() // handler, final publish, and processPromptQueue goroutine

	waitingInteraction := &types.Interaction{
		ID:              "int-mc",
		SessionID:       "ses_mc",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "AI response",
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{waitingInteraction}, int64(1), nil,
	).AnyTimes() // finding waiting, final publish, and processPromptQueue goroutine

	// Reload interaction
	reloadedInteraction := &types.Interaction{
		ID:              "int-mc",
		SessionID:       "ses_mc",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "AI response updated",
	}
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-mc").Return(reloadedInteraction, nil)

	// Update interaction to complete
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			s.Equal(types.InteractionStateComplete, interaction.State)
			s.False(interaction.Completed.IsZero())
			s.Equal("AI response updated", interaction.ResponseMessage) // Preserved from reload
			return interaction, nil
		},
	)

	// processPromptQueue calls GetNextPendingPrompt
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_mc").Return(nil, nil).AnyTimes()
	// GetPendingCommentByPlanningSessionID for fallback comment handling
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses_mc").Return(nil, nil).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-mc",
		},
	}

	err := s.server.handleMessageCompleted("agent-1", syncMsg)
	s.NoError(err)

	// Give goroutines time to complete
	time.Sleep(50 * time.Millisecond)
}

// TestMessageCompleted_PreservesInterruptedStateOnEmptyResponse pins down the
// race between handleTurnCancelled and handleMessageCompleted that Phase 13 of
// the Zed WS E2E test exercises: a cancel lands before any token streamed, so
// the interaction is marked Interrupted with an empty ResponseMessage. The
// agent then emits message_completed for the same request_id as it tears the
// turn down. Previously the empty-response branch in handleMessageCompleted
// clobbered the Interrupted state with Error and re-queued the prompt, which
// broke the E2E assertion "interaction in store with state=interrupted".
// Verify the handler returns without touching the interaction.
func (s *WebSocketSyncSuite) TestMessageCompleted_PreservesInterruptedStateOnEmptyResponse() {
	s.server.contextMappings["thread-int"] = "ses_int"
	s.server.requestToInteractionMapping = map[string]string{
		"req-int": "int-int",
	}

	session := &types.Session{
		ID:    "ses_int",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_int").Return(session, nil).AnyTimes()

	interruptedInteraction := &types.Interaction{
		ID:              "int-int",
		SessionID:       "ses_int",
		State:           types.InteractionStateInterrupted,
		ResponseMessage: "",
	}
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-int").Return(interruptedInteraction, nil)

	// Critical: no UpdateInteraction, no RequeueBouncedPrompt, no
	// processPromptQueue follow-up. The handler must return as soon as it
	// sees the Interrupted-with-empty-response pattern. gomock's default
	// strict mode fails the test if any unexpected store call is made.

	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-int",
			"request_id":    "req-int",
		},
	}

	err := s.server.handleMessageCompleted("agent-1", syncMsg)
	s.NoError(err)

	time.Sleep(50 * time.Millisecond)
}

func (s *WebSocketSyncSuite) TestMessageCompleted_NoWaitingInteraction() {
	s.server.contextMappings["thread-nw"] = "ses_nw"

	session := &types.Session{
		ID:    "ses_nw",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_nw").Return(session, nil)

	// All interactions are already complete
	completeInteraction := &types.Interaction{
		ID:        "int-nw",
		SessionID: "ses_nw",
		State:     types.InteractionStateComplete,
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{completeInteraction}, int64(1), nil,
	)

	// processPromptQueue still runs
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_nw").Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses_nw").Return(nil, nil).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-nw",
		},
	}

	err := s.server.handleMessageCompleted("agent-1", syncMsg)
	s.NoError(err) // Returns nil, no UpdateInteraction called

	time.Sleep(50 * time.Millisecond)
}

func (s *WebSocketSyncSuite) TestMessageCompleted_ContextMappingMiss_DBFallback() {
	// contextMappings is empty
	session := &types.Session{
		ID:    "ses_mc_fb",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-mc-fb",
		},
	}

	// findSessionByZedThreadID fallback
	s.store.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return(
		[]*types.Session{session}, int64(1), nil,
	)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_mc_fb").Return(session, nil).AnyTimes()

	waitingInteraction := &types.Interaction{
		ID:              "int-mc-fb",
		SessionID:       "ses_mc_fb",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "partial response flushed before restart",
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{waitingInteraction}, int64(1), nil,
	).AnyTimes()
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-mc-fb").Return(waitingInteraction, nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			s.Equal(types.InteractionStateComplete, interaction.State)
			return interaction, nil
		},
	)
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_mc_fb").Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses_mc_fb").Return(nil, nil).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-mc-fb",
		},
	}

	err := s.server.handleMessageCompleted("agent-1", syncMsg)
	s.NoError(err)

	// Verify contextMappings restored
	s.Equal("ses_mc_fb", s.server.contextMappings["thread-mc-fb"])

	time.Sleep(50 * time.Millisecond)
}

func (s *WebSocketSyncSuite) TestMessageCompleted_MissingThreadID() {
	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data:      map[string]interface{}{},
	}

	err := s.server.handleMessageCompleted("agent-1", syncMsg)
	s.Error(err)
	s.Contains(err.Error(), "missing acp_thread_id")
}

func (s *WebSocketSyncSuite) TestMessageCompleted_WithCommentFinalization() {
	s.server.contextMappings["thread-cf"] = "ses_cf"

	session := &types.Session{
		ID:    "ses_cf",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_cf").Return(session, nil).AnyTimes()

	waitingInteraction := &types.Interaction{
		ID:              "int-cf",
		SessionID:       "ses_cf",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "I've reviewed the design and it looks good.",
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{waitingInteraction}, int64(1), nil,
	).AnyTimes()
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-cf").Return(waitingInteraction, nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(waitingInteraction, nil)
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_cf").Return(nil, nil).AnyTimes()

	// PRIMARY comment finalization path: request_id in message data
	comment := &types.SpecTaskDesignReviewComment{
		ID:        "comment-1",
		ReviewID:  "review-1",
		RequestID: "req-cf-123",
	}
	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-cf-123").Return(comment, nil).AnyTimes()
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Empty(c.RequestID, "RequestID should be cleared")
			s.Nil(c.QueuedAt, "QueuedAt should be cleared")
			return nil
		},
	).AnyTimes()

	// finalizeCommentResponse tries to get design review and spec task
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-1").
		Return(nil, fmt.Errorf("not found")).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-cf",
			"request_id":    "req-cf-123",
		},
	}

	err := s.server.handleMessageCompleted("agent-1", syncMsg)
	s.NoError(err)

	time.Sleep(100 * time.Millisecond)
}

// TestMessageCompleted_SkipsAttentionWhenUserActive verifies that the
// agent_interaction_completed attention event is suppressed when a newer
// waiting interaction already exists in the session — i.e. the user has
// either interrupted the agent or sent a quick follow-up. In both cases the
// user is clearly looking at the UI and an "agent finished" notification
// would just be noise.
func (s *WebSocketSyncSuite) TestMessageCompleted_SkipsAttentionWhenUserActive() {
	s.server.contextMappings["thread-skip"] = "ses_skip"
	s.server.requestToInteractionMapping["req-skip"] = "int-target-skip"
	s.server.attentionService = services.NewAttentionService(s.store, s.server.Cfg)

	session := &types.Session{
		ID:    "ses_skip",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			SpecTaskID: "task-skip",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_skip").Return(session, nil).AnyTimes()

	baseTime := time.Now().Add(-1 * time.Minute)
	targetInteraction := &types.Interaction{
		ID:              "int-target-skip",
		SessionID:       "ses_skip",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "AI response",
		Created:         baseTime,
	}
	newerWaiting := &types.Interaction{
		ID:        "int-newer-skip",
		SessionID: "ses_skip",
		State:     types.InteractionStateWaiting,
		Created:   baseTime.Add(30 * time.Second),
	}

	s.store.EXPECT().GetInteraction(gomock.Any(), "int-target-skip").Return(targetInteraction, nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(targetInteraction, nil)

	// ListInteractions is called for both the suppression check (PerPage=1)
	// and the final session publish (PerPage=1000). Both return the same
	// list — newerWaiting at the end is what triggers suppression.
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{targetInteraction, newerWaiting}, int64(2), nil,
	).AnyTimes()

	// updateSpecTaskZedThreadActivity goroutine — not a tracked thread
	s.store.EXPECT().GetSpecTaskZedThreadByZedThreadID(gomock.Any(), "thread-skip").
		Return(nil, fmt.Errorf("not found")).AnyTimes()

	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_skip").Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-skip").
		Return(nil, fmt.Errorf("not found")).AnyTimes()

	// CRITICAL: GetSpecTask MUST NOT be called when the notification is suppressed.
	// gomock's default strict mode will fail the test if it is called without an EXPECT.

	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-skip",
			"request_id":    "req-skip",
		},
	}

	err := s.server.handleMessageCompleted("agent-1", syncMsg)
	s.NoError(err)

	// Give any goroutines (which we expect NOT to fire GetSpecTask) time to run
	time.Sleep(150 * time.Millisecond)
}

// TestMessageCompleted_EmitsAttentionWhenNoFollowup verifies that the
// agent_interaction_completed attention event IS emitted when there is no
// newer waiting interaction — the normal completion case where the user is
// not actively engaged.
func (s *WebSocketSyncSuite) TestMessageCompleted_EmitsAttentionWhenNoFollowup() {
	s.server.contextMappings["thread-emit"] = "ses_emit"
	s.server.requestToInteractionMapping["req-emit"] = "int-target-emit"
	s.server.attentionService = services.NewAttentionService(s.store, s.server.Cfg)

	session := &types.Session{
		ID:    "ses_emit",
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			SpecTaskID: "task-emit",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_emit").Return(session, nil).AnyTimes()

	baseTime := time.Now().Add(-1 * time.Minute)
	targetInteraction := &types.Interaction{
		ID:              "int-target-emit",
		SessionID:       "ses_emit",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "AI response",
		Created:         baseTime,
	}

	s.store.EXPECT().GetInteraction(gomock.Any(), "int-target-emit").Return(targetInteraction, nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(targetInteraction, nil)

	// Only the target interaction exists — no newer waiting one. Suppression
	// check sees Created.After(targetInteraction.Created) == false → emit.
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{targetInteraction}, int64(1), nil,
	).AnyTimes()

	s.store.EXPECT().GetSpecTaskZedThreadByZedThreadID(gomock.Any(), "thread-emit").
		Return(nil, fmt.Errorf("not found")).AnyTimes()

	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_emit").Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-emit").
		Return(nil, fmt.Errorf("not found")).AnyTimes()

	// GetSpecTask MUST be called when the notification is not suppressed.
	// We return an error to short-circuit before EmitEvent — the test only
	// needs to verify the goroutine reached this point.
	gotSpecTaskCall := make(chan struct{}, 1)
	s.store.EXPECT().GetSpecTask(gomock.Any(), "task-emit").DoAndReturn(
		func(_ context.Context, _ string) (*types.SpecTask, error) {
			select {
			case gotSpecTaskCall <- struct{}{}:
			default:
			}
			return nil, fmt.Errorf("test stub: spectask not loaded")
		},
	).MinTimes(1)

	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-emit",
			"request_id":    "req-emit",
		},
	}

	err := s.server.handleMessageCompleted("agent-1", syncMsg)
	s.NoError(err)

	select {
	case <-gotSpecTaskCall:
		// success — GetSpecTask was reached, meaning the goroutine fired
	case <-time.After(500 * time.Millisecond):
		s.Fail("GetSpecTask was not called within timeout — attention event goroutine did not fire")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// handleChatResponseError tests
// ──────────────────────────────────────────────────────────────────────────────

// TestMessageCompleted_AlreadyCompleteStillSignalsDone is the second half of
// the ses_01kx8knjxsa8rap7fxpe1bzafs regression: a first message_completed
// finalizes the interaction; a second one (or any late message_completed for a
// turn the transition logic already completed) must still poke doneChan so
// waitForExternalAgentResponse unblocks instead of hanging until the 180s
// timeout and clobbering the reply.
func (s *WebSocketSyncSuite) TestMessageCompleted_AlreadyCompleteStillSignalsDone() {
	const (
		helixSessionID = "ses_already_done"
		interactionID  = "int_already_done"
		requestID      = "req_waiter"
	)

	s.server.contextMappings["thread-already"] = helixSessionID
	s.server.requestToInteractionMapping[requestID] = interactionID

	// Register a waiter the same way RunExternalAgent does.
	doneChan := make(chan bool, 1)
	s.server.storeResponseChannel(helixSessionID, requestID, make(chan string, 1), doneChan, make(chan error, 1))
	defer s.server.cleanupResponseChannel(helixSessionID, requestID)

	session := &types.Session{ID: helixSessionID, Owner: "user-1"}
	s.store.EXPECT().GetSession(gomock.Any(), helixSessionID).Return(session, nil).AnyTimes()
	s.store.EXPECT().GetInteraction(gomock.Any(), interactionID).Return(&types.Interaction{
		ID:              interactionID,
		SessionID:       helixSessionID,
		State:           types.InteractionStateComplete,
		ResponseMessage: "full reply already in DB",
	}, nil)
	// No UpdateInteraction — early return on already-complete.

	err := s.server.handleMessageCompleted("agent-1", &types.SyncMessage{
		EventType: "message_completed",
		SessionID: helixSessionID,
		Data: map[string]interface{}{
			"acp_thread_id": "thread-already",
			"message_id":    "0",
			"request_id":    requestID,
		},
	})
	s.NoError(err)

	select {
	case <-doneChan:
		// expected
	case <-time.After(500 * time.Millisecond):
		s.Fail("doneChan was not signaled on already-complete message_completed")
	}
}

// TestMessageCompleted_SignalsDoneUnderInteractionID covers the case where the
// waiter registered under interaction.ID (the post-fix request_id convention)
// and message_completed carries that same id.
func (s *WebSocketSyncSuite) TestMessageCompleted_SignalsDoneUnderInteractionID() {
	const (
		helixSessionID = "ses_int_id"
		interactionID  = "int_is_request"
	)

	s.server.contextMappings["thread-intid"] = helixSessionID
	s.server.requestToInteractionMapping[interactionID] = interactionID

	doneChan := make(chan bool, 1)
	s.server.storeResponseChannel(helixSessionID, interactionID, make(chan string, 1), doneChan, make(chan error, 1))
	defer s.server.cleanupResponseChannel(helixSessionID, interactionID)

	session := &types.Session{ID: helixSessionID, Owner: "user-1"}
	s.store.EXPECT().GetSession(gomock.Any(), helixSessionID).Return(session, nil).AnyTimes()

	waiting := &types.Interaction{
		ID:              interactionID,
		SessionID:       helixSessionID,
		State:           types.InteractionStateWaiting,
		ResponseMessage: "streamed content",
	}
	s.store.EXPECT().GetInteraction(gomock.Any(), interactionID).Return(waiting, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{waiting}, int64(1), nil,
	).AnyTimes()
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, i *types.Interaction) (*types.Interaction, error) {
			s.Equal(types.InteractionStateComplete, i.State)
			s.Equal("streamed content", i.ResponseMessage)
			return i, nil
		},
	)
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), helixSessionID).Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), helixSessionID).Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), interactionID).
		Return(nil, fmt.Errorf("record not found")).AnyTimes()

	err := s.server.handleMessageCompleted("agent-1", &types.SyncMessage{
		EventType: "message_completed",
		SessionID: helixSessionID,
		Data: map[string]interface{}{
			"acp_thread_id": "thread-intid",
			"message_id":    "0",
			"request_id":    interactionID,
		},
	})
	s.NoError(err)

	select {
	case <-doneChan:
	case <-time.After(500 * time.Millisecond):
		s.Fail("doneChan was not signaled for interaction-id request_id")
	}

	time.Sleep(50 * time.Millisecond) // drain processPromptQueue goroutine
}

// TestChatResponseError_PersistsAgentErrorToInteraction is the red-then-green
// regression test for the "user sees nothing when the agent fails" bug.
//
// Repro: hire an AI worker, open chat, send "hello", agent fails internally
// with e.g. "Authentication required" (Claude Code subscription missing). The
// desktop emits chat_response_error AND then a downstream empty
// message_completed. Before this fix, handleChatResponseError only pushed the
// error to a legacy HTTP-streaming response channel that doesn't exist for
// WebSocket-driven chat — the interaction was silently left in Waiting, then
// handleMessageCompleted's empty-response branch overwrote it with the
// generic "Agent unresponsive: it returned an empty response. Retrying
// automatically." which buries the actual cause.
//
// This test pins the desired behaviour: when an active interaction exists for
// the failing request_id, the agent's error message MUST land on the
// interaction (state=error, error=<msg>), so the chat UI surfaces it
// faithfully instead of inventing a generic excuse.
func (s *WebSocketSyncSuite) TestChatResponseError_PersistsAgentErrorToInteraction() {
	s.server.requestToInteractionMapping["req-auth"] = "int-auth"

	interaction := &types.Interaction{
		ID:        "int-auth",
		SessionID: "ses_auth",
		State:     types.InteractionStateWaiting,
	}
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-auth").Return(interaction, nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, i *types.Interaction) (*types.Interaction, error) {
			s.Equal(types.InteractionStateError, i.State,
				"interaction must transition to error state")
			s.Equal("Authentication required", i.Error,
				"interaction.error must hold the agent's verbatim error, not a generic stand-in")
			return i, nil
		},
	)

	syncMsg := &types.SyncMessage{
		EventType: "chat_response_error",
		Data: map[string]interface{}{
			"request_id": "req-auth",
			"error":      "Authentication required",
		},
	}

	err := s.server.handleChatResponseError("agent-1", syncMsg)
	s.NoError(err)
}

// TestChatResponseError_NoOpWhenNoMappingOrChannel covers the case where
// neither a request-mapping nor a legacy response channel exists — must
// degrade silently (return nil) rather than panic or surface noise.
func (s *WebSocketSyncSuite) TestChatResponseError_NoOpWhenNoMappingOrChannel() {
	syncMsg := &types.SyncMessage{
		EventType: "chat_response_error",
		Data: map[string]interface{}{
			"request_id": "req-orphan",
			"error":      "boom",
		},
	}
	s.NoError(s.server.handleChatResponseError("agent-1", syncMsg))
}

// TestMessageCompleted_PreservesPriorAgentError pins the second half of the
// surfacing contract: if a chat_response_error already moved the interaction
// into Error state with a real cause, a subsequent empty message_completed
// must NOT overwrite that with the generic "Agent returned empty response"
// stand-in.
func (s *WebSocketSyncSuite) TestMessageCompleted_PreservesPriorAgentError() {
	s.server.contextMappings["thread-preserve"] = "ses_preserve"
	s.server.requestToInteractionMapping["req-preserve"] = "int-preserve"

	session := &types.Session{ID: "ses_preserve", Owner: "user-1"}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_preserve").Return(session, nil).AnyTimes()

	// Interaction is already in Error state from a prior chat_response_error.
	interaction := &types.Interaction{
		ID:              "int-preserve",
		SessionID:       "ses_preserve",
		State:           types.InteractionStateError,
		Error:           "Authentication required",
		ResponseMessage: "",
	}
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-preserve").Return(interaction, nil)

	// UpdateInteraction must NOT be called — the prior error is sacrosanct.
	// gomock strict mode fails if it's invoked without an EXPECT.

	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).
		Return([]*types.Interaction{interaction}, int64(1), nil).AnyTimes()
	s.store.EXPECT().GetSpecTaskZedThreadByZedThreadID(gomock.Any(), "thread-preserve").
		Return(nil, fmt.Errorf("not found")).AnyTimes()
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_preserve").Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-preserve").
		Return(nil, fmt.Errorf("not found")).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-preserve",
			"request_id":    "req-preserve",
		},
	}
	s.NoError(s.server.handleMessageCompleted("agent-1", syncMsg))
}

// ──────────────────────────────────────────────────────────────────────────────
// handleAgentReady tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestAgentReady_Basic() {
	// Initialize readiness tracking
	s.server.externalAgentWSManager.initReadinessState("ses_ready", false, nil)
	defer s.server.externalAgentWSManager.cleanupReadinessState("ses_ready")

	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), "ses_ready").Return(nil, nil).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "agent_ready",
		Data: map[string]interface{}{
			"agent_name": "zed-agent",
		},
	}

	err := s.server.handleAgentReady("ses_ready", syncMsg)
	s.NoError(err)

	// Verify session is now ready
	s.True(s.server.externalAgentWSManager.isSessionReady("ses_ready"))

	time.Sleep(50 * time.Millisecond)
}

func (s *WebSocketSyncSuite) TestAgentReady_NoReadinessState() {
	// No initReadinessState called — should return nil without error
	syncMsg := &types.SyncMessage{
		EventType: "agent_ready",
		Data:      map[string]interface{}{},
	}

	err := s.server.handleAgentReady("ses_nostate", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestAgentReady_WithPendingPrompt() {
	s.server.externalAgentWSManager.initReadinessState("ses_pending", false, nil)
	defer s.server.externalAgentWSManager.cleanupReadinessState("ses_pending")

	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-1",
		SessionID: "ses_pending",
		Content:   "queued prompt",
	}
	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), "ses_pending").Return(prompt, nil)
	// Note: ClaimPromptForSending is NOT called — GetAnyPendingPrompt already atomically claimed it
	// MarkPromptAsSent is NOT called here under the deferred-mark flow — the
	// prompt stays 'sending' until Zed acknowledges via the first message_added.

	// sendQueuedPromptToSession calls
	session := &types.Session{
		ID:    "ses_pending",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_pending").Return(session, nil).AnyTimes()
	// Re-check idle state before creating interaction
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{}, int64(0), nil,
	)
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			return &types.Interaction{ID: "int-prompt", SessionID: "ses_pending"}, nil
		},
	)
	// No-WS dispatch path: the interaction is persisted and the
	// interactionToPromptMapping is left intact so handleMessageAdded marks the
	// prompt 'sent' once Zed acknowledges via pickupWaitingInteraction. We must
	// NOT mark it failed here.
	// GetSpecTask for getAgentNameForSession + autoStartDevContainerForSession
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "agent_ready",
		Data:      map[string]interface{}{},
	}

	err := s.server.handleAgentReady("ses_pending", syncMsg)
	s.NoError(err)

	time.Sleep(100 * time.Millisecond)
}

func (s *WebSocketSyncSuite) TestAgentReady_ReconnectDoesNotSendOpenThread() {
	// open_thread is now sent on connect (handleExternalAgentConnection), BEFORE
	// the agent_ready gate. handleAgentReady should NOT send open_thread — doing
	// so would cause it to arrive after the queued chat_message, triggering
	// history replay that corrupts the current interaction.
	sessionID := "ses_reconnect"

	s.server.externalAgentWSManager.initReadinessState(sessionID, false, nil)
	defer s.server.externalAgentWSManager.cleanupReadinessState(sessionID)

	// Register a fake connection with a SendChan we can inspect
	sendChan := make(chan types.ExternalAgentCommand, 10)
	conn := &ExternalAgentWSConnection{
		SessionID: sessionID,
		SendChan:  sendChan,
	}
	s.server.externalAgentWSManager.registerConnection(sessionID, conn)

	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), sessionID).Return(nil, nil).AnyTimes()

	// Send agent_ready with no thread_id (fresh reconnect)
	syncMsg := &types.SyncMessage{
		EventType: "agent_ready",
		Data: map[string]interface{}{
			"agent_name": "zed-connection",
		},
	}

	err := s.server.handleAgentReady(sessionID, syncMsg)
	s.NoError(err)

	time.Sleep(50 * time.Millisecond)

	// Verify NO open_thread command was sent (it's now sent on connect, not on agent_ready)
	s.Equal(0, len(sendChan), "handleAgentReady should NOT send open_thread — it's sent on connect now")
}

func (s *WebSocketSyncSuite) TestAgentReady_NoOpenThreadWhenThreadIDPresent() {
	// When agent_ready includes a thread_id (Zed loaded a specific thread),
	// we should NOT send open_thread — Zed already has the subscription.
	sessionID := "ses_already_loaded"

	s.server.externalAgentWSManager.initReadinessState(sessionID, false, nil)
	defer s.server.externalAgentWSManager.cleanupReadinessState(sessionID)

	sendChan := make(chan types.ExternalAgentCommand, 10)
	conn := &ExternalAgentWSConnection{
		SessionID: sessionID,
		SendChan:  sendChan,
	}
	s.server.externalAgentWSManager.registerConnection(sessionID, conn)

	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), sessionID).Return(nil, nil).AnyTimes()

	// agent_ready WITH thread_id — Zed already loaded the thread
	syncMsg := &types.SyncMessage{
		EventType: "agent_ready",
		Data: map[string]interface{}{
			"agent_name": "claude",
			"thread_id":  "thread-already-loaded",
		},
	}

	err := s.server.handleAgentReady(sessionID, syncMsg)
	s.NoError(err)

	time.Sleep(50 * time.Millisecond)

	// No open_thread should be sent
	s.Equal(0, len(sendChan), "should NOT send open_thread when agent_ready includes thread_id")
}

// ──────────────────────────────────────────────────────────────────────────────
// sendChatMessageToExternalAgent tests — interrupt flag plumbing
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestSendChatMessage_InterruptTrue() {
	// Verifies the interrupt flag is forwarded into the chat_message command Data,
	// matching the semantic spec-task design-review comments rely on.
	sessionID := "ses_interrupt_true"

	sendChan := make(chan types.ExternalAgentCommand, 4)
	conn := &ExternalAgentWSConnection{SessionID: sessionID, SendChan: sendChan}
	s.server.externalAgentWSManager.registerConnection(sessionID, conn)

	session := &types.Session{
		ID:    sessionID,
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType:   "zed_external",
			ZedThreadID: "thread-int",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(
		&types.Interaction{ID: "int-1", SessionID: sessionID}, nil,
	)

	_, err := s.server.sendChatMessageToExternalAgent(sessionID, "review feedback", "req-int-1", true)
	s.NoError(err)

	select {
	case cmd := <-sendChan:
		s.Equal("chat_message", cmd.Type)
		s.Equal(true, cmd.Data["interrupt"], "interrupt=true must be forwarded into chat_message command")
		s.Equal("review feedback", cmd.Data["message"])
		s.Equal("req-int-1", cmd.Data["request_id"])
	case <-time.After(time.Second):
		s.Fail("expected chat_message command on send channel")
	}
}

func (s *WebSocketSyncSuite) TestSendChatMessage_InterruptFalse() {
	// System-driven messages (approval kickoff, post-merge instructions) must NOT
	// carry interrupt=true — they should respect the agent's queue.
	sessionID := "ses_interrupt_false"

	sendChan := make(chan types.ExternalAgentCommand, 4)
	conn := &ExternalAgentWSConnection{SessionID: sessionID, SendChan: sendChan}
	s.server.externalAgentWSManager.registerConnection(sessionID, conn)

	session := &types.Session{
		ID:    sessionID,
		Owner: "user-2",
		Metadata: types.SessionMetadata{
			AgentType:   "zed_external",
			ZedThreadID: "thread-noint",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(
		&types.Interaction{ID: "int-2", SessionID: sessionID}, nil,
	)

	_, err := s.server.sendChatMessageToExternalAgent(sessionID, "approval kickoff", "req-noint-1", false)
	s.NoError(err)

	select {
	case cmd := <-sendChan:
		s.Equal("chat_message", cmd.Type)
		s.Equal(false, cmd.Data["interrupt"], "interrupt=false must be forwarded into chat_message command")
	case <-time.After(time.Second):
		s.Fail("expected chat_message command on send channel")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// processPromptQueue tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestProcessPromptQueue_NoPending() {
	// Session busy check (new): session with no waiting interactions
	s.store.EXPECT().GetSession(gomock.Any(), "ses_nopq").Return(&types.Session{ID: "ses_nopq"}, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int-done", State: types.InteractionStateComplete}}, int64(1), nil,
	)
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_nopq").Return(nil, nil)

	s.server.processPromptQueue(context.Background(), "ses_nopq")
	// No further store calls expected
}

func (s *WebSocketSyncSuite) TestProcessPromptQueue_HasPending() {
	// Session busy check (new): session is idle (last interaction complete)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_pq").Return(&types.Session{ID: "ses_pq"}, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int-done", State: types.InteractionStateComplete}}, int64(1), nil,
	)

	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-pq",
		SessionID: "ses_pq",
		Content:   "queued content",
		Status:    "pending",
	}
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_pq").Return(prompt, nil)

	// sendQueuedPromptToSession calls
	session := &types.Session{
		ID:    "ses_pq",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_pq").Return(session, nil).AnyTimes()
	// Re-check idle state before creating interaction
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{}, int64(0), nil,
	)
	var capturedPromptID string
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			capturedPromptID = interaction.PromptID
			return &types.Interaction{ID: "int-pq", SessionID: "ses_pq", PromptID: interaction.PromptID}, nil
		},
	)
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()

	// sendCommandToExternalAgent fails with ErrNoExternalAgentWS (no WS
	// registered). The agent is sleeping, autoStartDevContainerForSession kicks
	// off, and the persisted Waiting interaction will be delivered by
	// pickupWaitingInteraction once the agent reconnects. The prompt MUST stay
	// in 'sending' (handleMessageAdded marks it 'sent' via the persisted
	// Interaction.PromptID column when Zed acknowledges) — marking it failed
	// here used to surface a misleading "no WebSocket connection" error in the
	// queue UI and trigger a retry that collided with the in-flight delivery.
	// So: no MarkPromptAsFailed expectation.

	s.server.processPromptQueue(context.Background(), "ses_pq")
	time.Sleep(50 * time.Millisecond) // let autoStartDevContainerForSession goroutine complete

	// The interaction must have been created with the prompt_id baked in so
	// handleMessageAdded can mark the prompt 'sent' from the DB row when Zed
	// eventually responds — this survives API restart, unlike the old
	// in-memory interactionToPromptMapping.
	s.Equal("prompt-pq", capturedPromptID, "Interaction must be created with PromptID linking back to the originating queue prompt")
}

func (s *WebSocketSyncSuite) TestProcessPromptQueue_SendFails_GetSessionFails() {
	// Session busy check (new): first GetSession succeeds (idle check)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_fail").Return(&types.Session{ID: "ses_fail"}, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{}, int64(0), nil,
	)

	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-fail",
		SessionID: "ses_fail",
		Content:   "fail content",
	}
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_fail").Return(prompt, nil)
	// MarkPromptAsPending no longer called - prompt is atomically claimed by GetNextPendingPrompt

	// sendQueuedPromptToSession fails because second GetSession fails
	s.store.EXPECT().GetSession(gomock.Any(), "ses_fail").Return(nil, fmt.Errorf("db error"))

	// Should mark as failed
	s.store.EXPECT().MarkPromptAsFailed(gomock.Any(), "prompt-fail", gomock.Any()).Return(nil)

	s.server.processPromptQueue(context.Background(), "ses_fail")
}

// ──────────────────────────────────────────────────────────────────────────────
// processAnyPendingPrompt tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestProcessAnyPendingPrompt_NoPending() {
	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), "ses_none").Return(nil, nil)

	s.server.processAnyPendingPrompt(context.Background(), "ses_none")
}

func (s *WebSocketSyncSuite) TestProcessAnyPendingPrompt_HasPending() {
	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-any",
		SessionID: "ses_any",
		Content:   "any prompt",
	}
	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), "ses_any").Return(prompt, nil)

	session := &types.Session{
		ID:    "ses_any",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_any").Return(session, nil).AnyTimes()
	// Re-check idle state before creating interaction
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{}, int64(0), nil,
	)
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(
		&types.Interaction{ID: "int-any", SessionID: "ses_any"}, nil,
	)
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	// No WS connection → dispatch returns ErrNoExternalAgentWS → prompt stays
	// in 'sending' (interaction persisted, pickupWaitingInteraction will deliver
	// it on reconnect, handleMessageAdded marks it sent).
	// No MarkPromptAsFailed expectation.

	s.server.processAnyPendingPrompt(context.Background(), "ses_any")
	time.Sleep(50 * time.Millisecond) // let autoStartDevContainerForSession goroutine complete
}

func (s *WebSocketSyncSuite) TestProcessAnyPendingPrompt_SendFails_MarkedFailed() {
	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-anyfail",
		SessionID: "ses_anyfail",
		Content:   "fail",
	}
	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), "ses_anyfail").Return(prompt, nil)
	// Note: ClaimPromptForSending is NOT called — GetAnyPendingPrompt already atomically claimed it

	// GetSession fails → sendQueuedPromptToSession fails
	s.store.EXPECT().GetSession(gomock.Any(), "ses_anyfail").Return(nil, fmt.Errorf("db error"))
	s.store.EXPECT().MarkPromptAsFailed(gomock.Any(), "prompt-anyfail", gomock.Any()).Return(nil)

	s.server.processAnyPendingPrompt(context.Background(), "ses_anyfail")
}

// ──────────────────────────────────────────────────────────────────────────────
// sendQueuedPromptToSession — in-memory state cleanup on send failure
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestSendQueuedPrompt_NoWS_PersistsPromptIDOnInteraction() {
	// When sendCommandToExternalAgent returns ErrNoExternalAgentWS (agent is
	// sleeping), the persisted Waiting interaction will be delivered by
	// pickupWaitingInteraction once the agent reconnects. The link from the
	// interaction back to the queue prompt now lives in the persisted
	// Interaction.PromptID column (no in-memory map). The prompt is NOT marked
	// failed in this case.

	session := &types.Session{
		ID:    "ses_nows",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_nows").Return(session, nil).AnyTimes()
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{}, int64(0), nil,
	)
	var capturedPromptID string
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			capturedPromptID = interaction.PromptID
			return &types.Interaction{ID: "int-nows", SessionID: "ses_nows", PromptID: interaction.PromptID}, nil
		},
	)
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()

	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-nows",
		SessionID: "ses_nows",
		Content:   "test prompt",
	}

	// No MarkPromptAsFailed expectation — the no-WS path must not surface as a
	// queue failure.

	err := s.server.sendQueuedPromptToSession(context.Background(), "ses_nows", prompt)
	s.NoError(err)
	time.Sleep(50 * time.Millisecond) // let autoStartDevContainerForSession goroutine complete

	// The persisted column carries the link past API restart — the old
	// in-memory interactionToPromptMapping was the source of the 6-day stuck
	// prompts in design/2026-04-30-queue-and-other-stuck-state-bugs.md.
	s.Equal("prompt-nows", capturedPromptID, "Interaction.PromptID must be persisted on creation")
}

func (s *WebSocketSyncSuite) TestSendQueuedPrompt_BusyWithOwnInteraction_ReturnsSuccess() {
	// Regression: after the no-WS path persists I1 with PromptID = P1 and the
	// retry timer fires while pickupWaitingInteraction is still in flight, the
	// busy re-check must recognise the in-flight interaction as our own and
	// return success — NOT "session became busy" (which previously marked the
	// prompt failed and made it look like delivery had failed twice).

	session := &types.Session{ID: "ses_busy", Owner: "user-1"}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_busy").Return(session, nil)
	// Latest interaction is the one we already dispatched, still Waiting, with
	// PromptID linking back to this prompt.
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int-inflight", State: types.InteractionStateWaiting, PromptID: "prompt-busy"}}, int64(1), nil,
	)
	// No CreateInteraction, no MarkPromptAsFailed — the function must return
	// nil without doing any further work.

	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-busy",
		SessionID: "ses_busy",
		Content:   "retry of in-flight prompt",
	}

	err := s.server.sendQueuedPromptToSession(context.Background(), "ses_busy", prompt)
	s.NoError(err, "in-flight delivery must not surface as a busy/failure error")
}

func (s *WebSocketSyncSuite) TestSendQueuedPrompt_BusyWithOtherInteraction_DefersAsBefore() {
	// Counterpart to the test above: when the in-flight Waiting interaction is
	// NOT our own (some other prompt or a Zed-initiated user message), the
	// busy-check must still defer with an error so the queue retries later.

	session := &types.Session{ID: "ses_busy2", Owner: "user-1"}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_busy2").Return(session, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int-other", State: types.InteractionStateWaiting, PromptID: "prompt-other"}}, int64(1), nil,
	)

	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-mine",
		SessionID: "ses_busy2",
		Content:   "I should wait my turn",
	}

	err := s.server.sendQueuedPromptToSession(context.Background(), "ses_busy2", prompt)
	s.Error(err)
	s.Contains(err.Error(), "became busy")
}

// ──────────────────────────────────────────────────────────────────────────────
// findSessionByZedThreadID tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestFindSessionByZedThreadID_Found() {
	session := &types.Session{
		ID: "ses_found",
		Metadata: types.SessionMetadata{
			ZedThreadID: "thread-find",
		},
	}
	s.store.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return(
		[]*types.Session{session}, int64(1), nil,
	)

	found, err := s.server.findSessionByZedThreadID(context.Background(), "thread-find")
	s.NoError(err)
	s.NotNil(found)
	s.Equal("ses_found", found.ID)
}

func (s *WebSocketSyncSuite) TestFindSessionByZedThreadID_NotFound() {
	s.store.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return(
		[]*types.Session{}, int64(0), nil,
	)

	found, err := s.server.findSessionByZedThreadID(context.Background(), "thread-missing")
	s.Error(err)
	s.Nil(found)
	s.Contains(err.Error(), "no session found with ZedThreadID")
}

// ──────────────────────────────────────────────────────────────────────────────
// getOrCreateStreamingContext — stale request_id rebind protection
// (regression for design/2026-04-28-stale-request-id-rebind-loses-zed-updates.md)
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestStreamingContext_StaleRequestIDRebind_PreservesConsumedSentinel() {
	// Scenario reproduced from the spt_01kq8cnjzfqc51nn0c6ddxkw8r incident:
	// 1. handleMessageCompleted previously consumed the mapping for req_X by
	//    setting requestToInteractionMapping[req_X] = "" (the dedup sentinel).
	// 2. The wrapper inside Zed flushes a buffered message_added later, tagged
	//    with the *stale* req_X.
	// 3. getOrCreateStreamingContext is called with helixSessionID + req_X for
	//    that flushed event, finds the most-recent Waiting interaction (a
	//    different turn), and used to overwrite the consumed sentinel — which
	//    later let a stale message_completed for req_X mark the new turn
	//    complete prematurely.
	//
	// After the fix the rebind must be skipped when the existing mapping is the
	// consumed sentinel; the sentinel must remain "" so the next stale
	// message_completed is routed through the duplicate-detection branch.

	s.server.requestToInteractionMapping["req_stale"] = "" // pre-consumed sentinel

	helixSession := &types.Session{ID: "ses_stale", GenerationID: 1}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_stale").Return(helixSession, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{
			{ID: "int-old", State: types.InteractionStateComplete},
			{ID: "int-new", State: types.InteractionStateWaiting},
		},
		int64(2), nil,
	)

	sctx := s.server.getOrCreateStreamingContext(context.Background(), "ses_stale", "req_stale")
	s.NotNil(sctx)
	// Streaming context must still find an interaction to attach content to —
	// the most-recent Waiting one — because the wrapper's buffered tokens have
	// to land somewhere visible to the user.
	s.Equal("int-new", sctx.interactionID)

	// The consumed sentinel must be intact so handleMessageCompleted's dedup
	// drops the stale completion that follows.
	s.server.contextMappingsMutex.Lock()
	mapped, exists := s.server.requestToInteractionMapping["req_stale"]
	s.server.contextMappingsMutex.Unlock()
	s.True(exists, "mapping entry should still exist")
	s.Equal("", mapped, "consumed sentinel must NOT be overwritten by stale rebind")
}

func (s *WebSocketSyncSuite) TestStreamingContext_FirstSightRequestID_RegistersMapping() {
	// Counterpart to the test above: when the request_id has never been seen
	// (genuine Zed-initiated message — user typed in Zed, no prior dispatch
	// from Helix), the mapping MUST still be populated so handleMessageCompleted
	// can route the eventual completion correctly.

	helixSession := &types.Session{ID: "ses_fresh", GenerationID: 1}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_fresh").Return(helixSession, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int-fresh", State: types.InteractionStateWaiting}},
		int64(1), nil,
	)

	sctx := s.server.getOrCreateStreamingContext(context.Background(), "ses_fresh", "req_fresh")
	s.NotNil(sctx)
	s.Equal("int-fresh", sctx.interactionID)

	s.server.contextMappingsMutex.Lock()
	mapped, exists := s.server.requestToInteractionMapping["req_fresh"]
	s.server.contextMappingsMutex.Unlock()
	s.True(exists)
	s.Equal("int-fresh", mapped, "first-sight request_id must be registered for completion routing")
}

// ──────────────────────────────────────────────────────────────────────────────
// handleThreadLoadError tests — Claude Agent crash detection
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestThreadLoadError_AgentCrash_MarksPromptCrashed() {
	// Persist the interaction with PromptID set (the in-memory map is gone).
	s.server.contextMappings["thread-crashed"] = "ses_crash"

	session := &types.Session{ID: "ses_crash", GenerationID: 1}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_crash").Return(session, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int-crashed", State: types.InteractionStateWaiting, PromptID: "prompt-crashed"}}, int64(1), nil,
	)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(nil, nil)

	// Crash detection: the error string includes "Claude Agent process exited"
	// → MarkPromptAsCrashed (not MarkPromptAsFailed) so auto-retry stops.
	s.store.EXPECT().MarkPromptAsCrashed(gomock.Any(), "prompt-crashed", gomock.Any()).Return(nil)

	syncMsg := &types.SyncMessage{
		EventType: "thread_load_error",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-crashed",
			"request_id":    "int-crashed",
			"error":         "Failed to send follow-up: Internal error: The Claude Agent process exited unexpectedly. Please start a new session.",
		},
	}
	err := s.server.handleThreadLoadError("ses_crash", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestThreadLoadError_SessionNotFound_AlsoCrash() {
	// "Session not found" is the steady-state error after the Claude Agent
	// process is gone. Same crash path applies.
	s.server.contextMappings["thread-snf"] = "ses_snf"

	session := &types.Session{ID: "ses_snf", GenerationID: 1}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_snf").Return(session, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int-snf", State: types.InteractionStateWaiting, PromptID: "prompt-snf"}}, int64(1), nil,
	)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.store.EXPECT().MarkPromptAsCrashed(gomock.Any(), "prompt-snf", gomock.Any()).Return(nil)

	syncMsg := &types.SyncMessage{
		EventType: "thread_load_error",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-snf",
			"request_id":    "int-snf",
			"error":         "Failed to send follow-up: Session not found",
		},
	}
	err := s.server.handleThreadLoadError("ses_snf", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestThreadLoadError_TransientError_StillUsesMarkAsFailed() {
	// Non-crash thread_load_errors (e.g. socket closed mid-flight, transient
	// failure) must still go through the normal MarkPromptAsFailed path so the
	// queue's exponential backoff can recover automatically.
	s.server.contextMappings["thread-transient"] = "ses_transient"

	session := &types.Session{ID: "ses_transient", GenerationID: 1, Metadata: types.SessionMetadata{ZedThreadID: "thread-transient"}}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_transient").Return(session, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int-transient", State: types.InteractionStateWaiting, PromptID: "prompt-transient"}}, int64(1), nil,
	)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(nil, nil)
	// First occurrence (retry_count below the recurrence threshold) → normal retry.
	s.store.EXPECT().GetPromptHistoryEntry(gomock.Any(), "prompt-transient").
		Return(&types.PromptHistoryEntry{ID: "prompt-transient", RetryCount: 0}, nil)
	s.store.EXPECT().MarkPromptAsFailed(gomock.Any(), "prompt-transient", gomock.Any()).Return(nil)

	syncMsg := &types.SyncMessage{
		EventType: "thread_load_error",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-transient",
			"request_id":    "int-transient",
			"error":         "Failed to send follow-up: API Error: The socket connection was closed unexpectedly.",
		},
	}
	err := s.server.handleThreadLoadError("ses_transient", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestThreadLoadError_MissingCodexRolloutClearsThreadForRetry() {
	s.server.contextMappings["thread-codex"] = "ses_codex"
	sendChan := make(chan types.ExternalAgentCommand, 2)
	s.server.externalAgentWSManager.registerConnection("ses_codex", &ExternalAgentWSConnection{SessionID: "ses_codex", SendChan: sendChan})

	session := &types.Session{ID: "ses_codex", GenerationID: 1}
	session.Metadata.ZedThreadID = "thread-codex"
	session.Metadata.ZedAgentName = "codex"
	s.store.EXPECT().GetSession(gomock.Any(), "ses_codex").Return(session, nil)
	s.store.EXPECT().UpdateSessionMetadata(gomock.Any(), "ses_codex", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, metadata types.SessionMetadata) error {
			s.Empty(metadata.ZedThreadID)
			return nil
		},
	)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-codex").Return(
		&types.Interaction{ID: "int-codex", SessionID: "ses_codex", State: types.InteractionStateWaiting, PromptID: "prompt-codex", PromptMessage: "retry codex"}, nil,
	)

	err := s.server.handleThreadLoadError("ses_codex", &types.SyncMessage{
		EventType: "thread_load_error",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-codex",
			"request_id":    "int-codex",
			"error":         "Failed to load thread: Internal error: no rollout found for thread id thread-codex",
		},
	})
	s.NoError(err)

	s.server.contextMappingsMutex.RLock()
	_, exists := s.server.contextMappings["thread-codex"]
	s.server.contextMappingsMutex.RUnlock()
	s.False(exists)
	select {
	case command := <-sendChan:
		s.Equal("chat_message", command.Type)
		s.Equal("retry codex", command.Data["message"])
		_, hasThreadID := command.Data["acp_thread_id"]
		s.False(hasThreadID)
	default:
		s.Fail("missing Codex replay command")
	}
}

func (s *WebSocketSyncSuite) TestThreadLoadError_MissingZedThreadReplaysDirectInteractionOnce() {
	const primeError = `no thread found with ID: SessionId("019cba1e-2994-77d0-bc27-fc350cfdc2c2")`
	s.server.contextMappings["thread-prime"] = "ses_prime"
	s.server.requestToInteractionMapping["req-prime"] = "int-prime"
	sendChan := make(chan types.ExternalAgentCommand, 2)
	s.server.externalAgentWSManager.registerConnection("ses_prime", &ExternalAgentWSConnection{SessionID: "ses_prime", SendChan: sendChan})

	session := &types.Session{ID: "ses_prime", GenerationID: 1, Metadata: types.SessionMetadata{
		ZedThreadID: "thread-prime", ZedAgentName: "claude",
	}}
	interaction := &types.Interaction{
		ID: "int-prime", SessionID: "ses_prime", State: types.InteractionStateWaiting,
		PromptMessage: "direct Prime request",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_prime").Return(session, nil).Times(2)
	s.store.EXPECT().UpdateSessionMetadata(gomock.Any(), "ses_prime", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, metadata types.SessionMetadata) error {
			s.Empty(metadata.ZedThreadID)
			return nil
		},
	)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-prime").Return(interaction, nil)

	syncMsg := &types.SyncMessage{EventType: "thread_load_error", Data: map[string]interface{}{
		"acp_thread_id": "thread-prime",
		"request_id":    "req-prime",
		"error":         primeError,
	}}
	s.NoError(s.server.handleThreadLoadError("ses_prime", syncMsg))
	duplicate := &types.SyncMessage{EventType: "thread_load_error", Data: map[string]interface{}{
		"acp_thread_id": "thread-prime",
		"request_id":    "open-thread-request",
		"error":         primeError,
	}}
	s.NoError(s.server.handleThreadLoadError("ses_prime", duplicate))

	s.Equal(types.InteractionStateWaiting, interaction.State)
	s.Len(sendChan, 1)
	command := <-sendChan
	s.Equal("direct Prime request", command.Data["message"])
	s.Equal("req-prime", command.Data["request_id"])
	_, hasThreadID := command.Data["acp_thread_id"]
	s.False(hasThreadID)
	s.Equal("ses_prime", s.server.requestToSessionMapping["req-prime"])
}

func (s *WebSocketSyncSuite) TestThreadLoadError_ArbitraryLoadErrorDoesNotClearOrReplay() {
	s.server.contextMappings["thread-arbitrary"] = "ses_arbitrary"
	sendChan := make(chan types.ExternalAgentCommand, 1)
	s.server.externalAgentWSManager.registerConnection("ses_arbitrary", &ExternalAgentWSConnection{SessionID: "ses_arbitrary", SendChan: sendChan})

	session := &types.Session{ID: "ses_arbitrary", GenerationID: 1, Metadata: types.SessionMetadata{ZedThreadID: "thread-arbitrary"}}
	interaction := &types.Interaction{ID: "int-arbitrary", State: types.InteractionStateWaiting}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_arbitrary").Return(session, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{interaction}, int64(1), nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), interaction).Return(interaction, nil)

	s.NoError(s.server.handleThreadLoadError("ses_arbitrary", &types.SyncMessage{
		EventType: "thread_load_error",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-arbitrary",
			"request_id":    "int-arbitrary",
			"error":         "Failed to load thread: transport disconnected",
		},
	}))

	s.Equal("thread-arbitrary", session.Metadata.ZedThreadID)
	s.Empty(sendChan)
}

func (s *WebSocketSyncSuite) TestThreadLoadError_RecurringFailure_CrashesRegardlessOfWording() {
	// A thread_load_error that keeps failing across retries is terminal even when
	// the wording isn't a known hard-crash marker (e.g. the dead-connection
	// "send failed because receiver is gone"). Once retry_count reaches the
	// recurrence threshold we crash-mark so Restart surfaces instead of looping.
	s.server.contextMappings["thread-recur"] = "ses_recur"

	session := &types.Session{ID: "ses_recur", GenerationID: 1}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_recur").Return(session, nil)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{{ID: "int-recur", State: types.InteractionStateWaiting, PromptID: "prompt-recur"}}, int64(1), nil,
	)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.store.EXPECT().GetPromptHistoryEntry(gomock.Any(), "prompt-recur").
		Return(&types.PromptHistoryEntry{ID: "prompt-recur", RetryCount: acpWedgeCrashThreshold}, nil)
	s.store.EXPECT().MarkPromptAsCrashed(gomock.Any(), "prompt-recur", gomock.Any()).Return(nil)

	syncMsg := &types.SyncMessage{
		EventType: "thread_load_error",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-recur",
			"request_id":    "int-recur",
			"error":         "Failed to send follow-up: Internal error: \"send failed because receiver is gone\"",
		},
	}
	err := s.server.handleThreadLoadError("ses_recur", syncMsg)
	s.NoError(err)
}

// ──────────────────────────────────────────────────────────────────────────────
// handleUserCreatedThread tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestUserCreatedThread_NewSession() {
	existingSession := &types.Session{
		ID:             "ses_agent",
		Owner:          "user-1",
		OwnerType:      types.OwnerTypeUser,
		ModelName:      "claude-sonnet",
		ParentApp:      "app-1",
		OrganizationID: "org-1",
		Type:           types.SessionTypeText,
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_agent").Return(existingSession, nil)

	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			s.Equal("thread-user-new", session.Metadata.ZedThreadID)
			s.Equal("user-1", session.Owner)
			s.Equal("app-1", session.ParentApp) // Config copied from existing
			s.Equal("zed_external", session.Metadata.AgentType)
			return &session, nil
		},
	)

	syncMsg := &types.SyncMessage{
		EventType: "user_created_thread",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-user-new",
			"title":         "User Chat",
		},
	}

	err := s.server.handleUserCreatedThread("ses_agent", syncMsg)
	s.NoError(err)

	// Verify contextMappings
	_, exists := s.server.contextMappings["thread-user-new"]
	s.True(exists)
}

func (s *WebSocketSyncSuite) TestUserCreatedThread_Idempotent() {
	// Thread already has a session mapped
	s.server.contextMappings["thread-existing"] = "ses_existing"

	syncMsg := &types.SyncMessage{
		EventType: "user_created_thread",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-existing",
			"title":         "Existing",
		},
	}

	// No store calls expected
	err := s.server.handleUserCreatedThread("ses_agent", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestUserCreatedThread_MissingThreadID() {
	syncMsg := &types.SyncMessage{
		EventType: "user_created_thread",
		Data:      map[string]interface{}{},
	}

	err := s.server.handleUserCreatedThread("ses_agent", syncMsg)
	s.Error(err)
	s.Contains(err.Error(), "missing or invalid acp_thread_id")
}

// ──────────────────────────────────────────────────────────────────────────────
// linkAgentResponseToComment tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestLinkAgentResponse_CommentExists() {
	interaction := &types.Interaction{
		ID:              "int-link",
		ResponseMessage: "AI helped with the review",
	}
	comment := &types.SpecTaskDesignReviewComment{
		ID: "comment-link",
	}

	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-link").Return(comment, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Equal("AI helped with the review", c.AgentResponse)
			s.NotNil(c.AgentResponseAt)
			return nil
		},
	)

	err := s.server.linkAgentResponseToComment(context.Background(), interaction)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestLinkAgentResponse_NoComment() {
	interaction := &types.Interaction{
		ID:              "int-nolink",
		ResponseMessage: "response",
	}

	s.store.EXPECT().GetCommentByInteractionID(gomock.Any(), "int-nolink").
		Return(nil, store.ErrNotFound)

	err := s.server.linkAgentResponseToComment(context.Background(), interaction)
	s.Error(err) // Returns error, but callers handle this gracefully
}

func (s *WebSocketSyncSuite) TestLinkAgentResponse_EmptyInteractionID() {
	interaction := &types.Interaction{
		ID: "",
	}

	err := s.server.linkAgentResponseToComment(context.Background(), interaction)
	s.Error(err)
	s.Contains(err.Error(), "interaction ID is empty")
}

// ──────────────────────────────────────────────────────────────────────────────
// finalizeCommentResponse tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestFinalizeComment_CommentExists() {
	comment := &types.SpecTaskDesignReviewComment{
		ID:        "comment-fin",
		ReviewID:  "review-fin",
		RequestID: "req-fin",
	}

	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-fin").Return(comment, nil)
	s.store.EXPECT().UpdateSpecTaskDesignReviewComment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, c *types.SpecTaskDesignReviewComment) error {
			s.Empty(c.RequestID)
			s.Nil(c.QueuedAt)
			return nil
		},
	)

	review := &types.SpecTaskDesignReview{
		ID:         "review-fin",
		SpecTaskID: "spec-fin",
	}
	// Called twice: once from populateAgentResponseFromSession, once from processNextCommentInQueue
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-fin").Return(review, nil).Times(2)

	specTask := &types.SpecTask{
		ID:                "spec-fin",
		PlanningSessionID: "ses_planning",
	}
	// Called twice: once from populateAgentResponseFromSession, once from processNextCommentInQueue
	s.store.EXPECT().GetSpecTask(gomock.Any(), "spec-fin").Return(specTask, nil).Times(2)

	// populateAgentResponseFromSession tries to get the session to find a response
	// (comment has no AgentResponse, so this fallback fires)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_planning").Return(&types.Session{
		ID:           "ses_planning",
		Interactions: []*types.Interaction{},
	}, nil)

	// processNextCommentInQueue
	s.store.EXPECT().IsCommentBeingProcessedForSession(gomock.Any(), "ses_planning").Return(false, nil)
	s.store.EXPECT().GetNextQueuedCommentForSession(gomock.Any(), "ses_planning").Return(nil, store.ErrNotFound)

	err := s.server.finalizeCommentResponse(context.Background(), "req-fin")
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestFinalizeComment_NoComment() {
	s.store.EXPECT().GetCommentByRequestID(gomock.Any(), "req-missing").
		Return(nil, store.ErrNotFound)

	err := s.server.finalizeCommentResponse(context.Background(), "req-missing")
	s.Error(err)
	s.Contains(err.Error(), "no comment found")
}

func (s *WebSocketSyncSuite) TestFinalizeComment_EmptyRequestID() {
	err := s.server.finalizeCommentResponse(context.Background(), "")
	s.Error(err)
	s.Contains(err.Error(), "request ID is empty")
}

// ──────────────────────────────────────────────────────────────────────────────
// handleThreadTitleChanged tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestThreadTitleChanged_UpdatesSessionName() {
	s.server.contextMappings["thread-title"] = "ses_title"

	session := &types.Session{
		ID:    "ses_title",
		Owner: "user-1",
		Name:  "Old Title",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_title").Return(session, nil)
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, ses types.Session) (*types.Session, error) {
			s.Equal("New Title", ses.Name)
			return &ses, nil
		},
	)

	syncMsg := &types.SyncMessage{
		EventType: "thread_title_changed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-title",
			"title":         "New Title",
		},
	}

	err := s.server.handleThreadTitleChanged("agent-1", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestThreadTitleChanged_NoMappedSession() {
	syncMsg := &types.SyncMessage{
		EventType: "thread_title_changed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-unmapped",
			"title":         "Whatever",
		},
	}

	// No store calls expected — returns nil (not an error)
	err := s.server.handleThreadTitleChanged("agent-1", syncMsg)
	s.NoError(err)
}

// ──────────────────────────────────────────────────────────────────────────────
// processExternalAgentSyncMessage dispatch tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestProcessSyncMessage_UnknownEventType() {
	syncMsg := &types.SyncMessage{
		EventType: "unknown_event",
		Data:      map[string]interface{}{},
	}

	// Should not error — unknown events are logged and ignored
	err := s.server.processExternalAgentSyncMessage("agent-1", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestProcessSyncMessage_PingNoOp() {
	syncMsg := &types.SyncMessage{
		EventType: "ping",
		Data:      map[string]interface{}{},
	}

	err := s.server.processExternalAgentSyncMessage("agent-1", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestProcessSyncMessage_SyncEventHookFires() {
	var hookCalled bool
	var hookSessionID string
	var hookEventType string

	s.server.syncEventHook = func(sessionID string, msg *types.SyncMessage) {
		hookCalled = true
		hookSessionID = sessionID
		hookEventType = msg.EventType
	}

	syncMsg := &types.SyncMessage{
		EventType: "ping",
		Data:      map[string]interface{}{},
	}

	err := s.server.processExternalAgentSyncMessage("agent-hook", syncMsg)
	s.NoError(err)
	s.True(hookCalled)
	s.Equal("agent-hook", hookSessionID)
	s.Equal("ping", hookEventType)
}

// ──────────────────────────────────────────────────────────────────────────────
// Streaming context cache tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestStreamingContextCache_SecondTokenSkipsDBQueries() {
	// Setup: context mapping and waiting interaction
	s.server.contextMappings["thread-cache"] = "ses_cache"

	session := &types.Session{
		ID:    "ses_cache",
		Owner: "user-1",
	}
	existingInteraction := &types.Interaction{
		ID:              "int-cache",
		SessionID:       "ses_cache",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
	}

	// FIRST token: GetSession + ListInteractions called (cache miss)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_cache").Return(session, nil).Times(1)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{existingInteraction}, int64(1), nil,
	).Times(1)

	// Only FIRST token writes to DB (lastDBWrite is zero, so first always flushes).
	// Second token within 200ms is throttled — no streaming-fields call.
	s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ int, responseMessage string, _ datatypes.JSON, _ int, _ string) error {
			s.Equal("Hello", responseMessage)
			return nil
		},
	).Times(1)

	// First token
	syncMsg1 := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-cache",
			"message_id":    "msg-1",
			"content":       "Hello",
			"role":          "assistant",
		},
	}
	err := s.server.handleMessageAdded("agent-1", syncMsg1)
	s.NoError(err)

	// Verify cache was populated
	s.server.streamingContextsMu.RLock()
	sctx, exists := s.server.streamingContexts["ses_cache"]
	s.server.streamingContextsMu.RUnlock()
	s.True(exists, "streaming context should be cached after first token")
	s.NotNil(sctx.session)
	s.NotNil(sctx.interaction)

	// SECOND token: NO GetSession, ListInteractions, OR UpdateInteraction calls.
	// DB write is throttled (within 200ms). Content updated in-memory only.
	syncMsg2 := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-cache",
			"message_id":    "msg-1",
			"content":       "Hello, world!",
			"role":          "assistant",
		},
	}
	err = s.server.handleMessageAdded("agent-1", syncMsg2)
	s.NoError(err)

	// Verify accumulator has latest content despite no DB write.
	// ResponseMessage is only updated on DB write (deferred), so check
	// the accumulator which always has the latest content.
	sctx.mu.Lock()
	s.NotNil(sctx.accumulator, "accumulator should exist")
	sctx.accumulator.Rebuild()
	s.Equal("Hello, world!", sctx.accumulator.Content)
	s.True(sctx.dirty, "interaction should be dirty (not yet flushed)")
	sctx.mu.Unlock()
}

func (s *WebSocketSyncSuite) TestStreamingContextCache_ClearedOnMessageCompleted() {
	s.server.contextMappings["thread-clear"] = "ses_clear"

	session := &types.Session{
		ID:    "ses_clear",
		Owner: "user-1",
	}

	// Pre-populate the streaming context cache
	s.server.streamingContextsMu.Lock()
	s.server.streamingContexts["ses_clear"] = &streamingContext{
		session: session,
		interaction: &types.Interaction{
			ID:              "int-clear",
			SessionID:       "ses_clear",
			State:           types.InteractionStateWaiting,
			ResponseMessage: "cached content",
		},
	}
	s.server.streamingContextsMu.Unlock()

	// handleMessageCompleted should clear the cache
	s.store.EXPECT().GetSession(gomock.Any(), "ses_clear").Return(session, nil).AnyTimes()

	waitingInteraction := &types.Interaction{
		ID:              "int-clear",
		SessionID:       "ses_clear",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "final content",
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{waitingInteraction}, int64(1), nil,
	).AnyTimes()
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-clear").Return(waitingInteraction, nil)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(waitingInteraction, nil)
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_clear").Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses_clear").Return(nil, nil).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-clear",
		},
	}

	err := s.server.handleMessageCompleted("agent-1", syncMsg)
	s.NoError(err)

	// Verify cache was cleared
	s.server.streamingContextsMu.RLock()
	_, exists := s.server.streamingContexts["ses_clear"]
	s.server.streamingContextsMu.RUnlock()
	s.False(exists, "streaming context should be cleared after message_completed")

	time.Sleep(50 * time.Millisecond)
}

func (s *WebSocketSyncSuite) TestStreamingContextCache_UserMessageDoesNotUseCache() {
	s.server.contextMappings["thread-usermsg"] = "ses_usermsg"

	session := &types.Session{
		ID:           "ses_usermsg",
		Owner:        "user-1",
		GenerationID: 0,
	}
	// User messages always call GetSession (not cached)
	s.store.EXPECT().GetSession(gomock.Any(), "ses_usermsg").Return(session, nil)

	createdInteraction := &types.Interaction{
		ID:        "int-usermsg-new",
		SessionID: "ses_usermsg",
	}
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(createdInteraction, nil)

	syncMsg := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-usermsg",
			"message_id":    "msg-user",
			"content":       "User message",
			"role":          "user",
		},
	}

	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	// Verify no streaming context was created for user messages
	s.server.streamingContextsMu.RLock()
	_, exists := s.server.streamingContexts["ses_usermsg"]
	s.server.streamingContextsMu.RUnlock()
	s.False(exists, "user messages should not create streaming context")
}

func (s *WebSocketSyncSuite) TestStreamingThrottle_DBWriteAfterInterval() {
	// Test that DB write happens after the throttle interval expires.
	s.server.contextMappings["thread-throttle"] = "ses_throttle"

	session := &types.Session{
		ID:    "ses_throttle",
		Owner: "user-1",
	}
	existingInteraction := &types.Interaction{
		ID:              "int-throttle",
		SessionID:       "ses_throttle",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
	}

	s.store.EXPECT().GetSession(gomock.Any(), "ses_throttle").Return(session, nil).Times(1)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{existingInteraction}, int64(1), nil,
	).Times(1)

	// First token writes immediately (lastDBWrite is zero)
	s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// First token
	syncMsg := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-throttle",
			"message_id":    "msg-1",
			"content":       "Token 1",
			"role":          "assistant",
		},
	}
	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	// Second token immediately — throttled, no DB write
	syncMsg.Data["content"] = "Token 1 Token 2"
	err = s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	// Verify dirty flag — ResponseMessage is deferred to DB write, so check accumulator
	s.server.streamingContextsMu.RLock()
	sctx := s.server.streamingContexts["ses_throttle"]
	s.server.streamingContextsMu.RUnlock()
	sctx.mu.Lock()
	s.True(sctx.dirty, "should be dirty after throttled write")
	// ResponseMessage is deferred to DB write, so check accumulator instead.
	sctx.accumulator.Rebuild()
	s.Equal("Token 1 Token 2", sctx.accumulator.Content)
	// Artificially expire the throttle interval
	sctx.lastDBWrite = time.Now().Add(-10 * time.Second)
	sctx.mu.Unlock()

	// Now expect another DB write since interval expired
	s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ int, responseMessage string, _ datatypes.JSON, _ int, _ string) error {
			s.Equal("Token 1 Token 2 Token 3", responseMessage)
			return nil
		},
	).Times(1)

	syncMsg.Data["content"] = "Token 1 Token 2 Token 3"
	err = s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	sctx.mu.Lock()
	s.False(sctx.dirty, "should not be dirty after interval-triggered write")
	sctx.mu.Unlock()
}

func (s *WebSocketSyncSuite) TestStreamingThrottle_DirtyFlushOnMessageCompleted() {
	// Test that dirty interaction is flushed to DB when message_completed arrives.
	s.server.contextMappings["thread-flush"] = "ses_flush"

	session := &types.Session{
		ID:    "ses_flush",
		Owner: "user-1",
	}
	existingInteraction := &types.Interaction{
		ID:              "int-flush",
		SessionID:       "ses_flush",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
	}

	// Pre-populate cache with dirty state (simulates tokens that were throttled)
	s.server.streamingContextsMu.Lock()
	s.server.streamingContexts["ses_flush"] = &streamingContext{
		session: session,
		interaction: &types.Interaction{
			ID:              "int-flush",
			SessionID:       "ses_flush",
			State:           types.InteractionStateWaiting,
			ResponseMessage: "dirty unflushed content",
		},
		dirty:       true,
		lastDBWrite: time.Now(), // recent write, so it was throttled
	}
	s.server.streamingContextsMu.Unlock()

	// flushAndClearStreamingContext should flush the dirty interaction.
	// Column-scoped: only the content fields are written, never state, so
	// the in-progress state stays Waiting until handleMessageCompleted's
	// own UpdateInteraction transitions it.
	flushUpdate := s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ int, responseMessage string, _ datatypes.JSON, _ int, _ string) error {
			s.Equal("dirty unflushed content", responseMessage)
			return nil
		},
	)

	// handleMessageCompleted then does its normal flow
	s.store.EXPECT().GetSession(gomock.Any(), "ses_flush").Return(session, nil).AnyTimes()
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{existingInteraction}, int64(1), nil,
	).AnyTimes()
	// GetInteraction is called once inside handleMessageCompleted to reload
	// the latest response content from the DB. The flush no longer reloads
	// state separately because column-scoped writes can't clobber it.
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-flush").Return(&types.Interaction{
		ID:              "int-flush",
		SessionID:       "ses_flush",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "dirty unflushed content", // DB was just flushed
	}, nil).Times(1)

	// Final UpdateInteraction to mark complete (must come after flush)
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			s.Equal(types.InteractionStateComplete, interaction.State)
			return interaction, nil
		},
	).After(flushUpdate)

	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_flush").Return(nil, nil).AnyTimes()
	s.store.EXPECT().GetPendingCommentByPlanningSessionID(gomock.Any(), "ses_flush").Return(nil, nil).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_completed",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-flush",
		},
	}

	err := s.server.handleMessageCompleted("agent-1", syncMsg)
	s.NoError(err)

	// Verify cache was cleared
	s.server.streamingContextsMu.RLock()
	_, exists := s.server.streamingContexts["ses_flush"]
	s.server.streamingContextsMu.RUnlock()
	s.False(exists, "streaming context should be cleared after message_completed")

	time.Sleep(50 * time.Millisecond)
}

func (s *WebSocketSyncSuite) TestStreamingThrottle_MultiMessageAccumulation() {
	// Test content accumulation across different message_ids (text -> tool call -> text)
	s.server.contextMappings["thread-multi"] = "ses_multi"

	session := &types.Session{
		ID:    "ses_multi",
		Owner: "user-1",
	}
	existingInteraction := &types.Interaction{
		ID:              "int-multi",
		SessionID:       "ses_multi",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
	}

	s.store.EXPECT().GetSession(gomock.Any(), "ses_multi").Return(session, nil).Times(1)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{existingInteraction}, int64(1), nil,
	).Times(1)

	// First message_id writes immediately
	s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// First message_id: assistant text
	syncMsg := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-multi",
			"message_id":    "msg-text",
			"content":       "Let me help",
			"role":          "assistant",
		},
	}
	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	// Second message_id: tool call (different message_id triggers \n\n separator)
	// Throttled, no DB write
	syncMsg.Data["message_id"] = "msg-tool"
	syncMsg.Data["content"] = "[Running: ls -la]"
	err = s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	// Verify accumulated content in memory — ResponseMessage is deferred to DB
	// write, so check the accumulator which always has the latest content.
	s.server.streamingContextsMu.RLock()
	sctx := s.server.streamingContexts["ses_multi"]
	s.server.streamingContextsMu.RUnlock()

	sctx.mu.Lock()
	sctx.accumulator.Rebuild()
	s.Equal("Let me help\n\n[Running: ls -la]", sctx.accumulator.Content)
	s.Equal("msg-tool", sctx.interaction.LastZedMessageID)

	// Third update: tool call status changes (same message_id, content replaces from offset)
	sctx.mu.Unlock()

	syncMsg.Data["content"] = "[Finished: ls -la]\nfile1.txt\nfile2.txt"
	err = s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	sctx.mu.Lock()
	sctx.accumulator.Rebuild()
	s.Equal("Let me help\n\n[Finished: ls -la]\nfile1.txt\nfile2.txt", sctx.accumulator.Content)
	s.True(sctx.dirty)
	sctx.mu.Unlock()
}

// TestComputePatch_Append verifies the fast path: pure append returns offset=len(old).
func (s *WebSocketSyncSuite) TestComputePatch_Append() {
	old := "Hello "
	new := "Hello world"
	offset, patch, totalLen := computePatch(old, new)
	s.Equal(len(old), offset)
	s.Equal("world", patch)
	s.Equal(len(new), totalLen)

	// Reconstruct
	reconstructed := old[:offset] + patch
	s.Equal(new, reconstructed)
}

// TestComputePatch_BackwardsEdit verifies the slow path: tool call status change.
func (s *WebSocketSyncSuite) TestComputePatch_BackwardsEdit() {
	old := "Let me help\n\n[Running: ls -la]"
	new := "Let me help\n\n[Finished: ls -la]\nfile1.txt"
	offset, patch, totalLen := computePatch(old, new)
	// First difference is at position 14 ('R' vs 'F')
	s.Equal(14, offset)
	s.Equal("Finished: ls -la]\nfile1.txt", patch)
	s.Equal(len(new), totalLen)

	// Reconstruct
	reconstructed := old[:offset] + patch
	s.Equal(new, reconstructed)
}

// TestComputePatch_EmptyPrevious verifies first token (no previous content).
func (s *WebSocketSyncSuite) TestComputePatch_EmptyPrevious() {
	old := ""
	new := "Hello"
	offset, patch, totalLen := computePatch(old, new)
	s.Equal(0, offset)
	s.Equal("Hello", patch)
	s.Equal(5, totalLen)
}

// TestComputePatch_Identical verifies no change produces empty patch.
func (s *WebSocketSyncSuite) TestComputePatch_Identical() {
	content := "Hello world"
	offset, patch, totalLen := computePatch(content, content)
	s.Equal(len(content), offset)
	s.Equal("", patch)
	s.Equal(len(content), totalLen)
}

// TestComputePatch_Truncation verifies content getting shorter (deletion).
func (s *WebSocketSyncSuite) TestComputePatch_Truncation() {
	old := "Hello world, how are you?"
	new := "Hello world"
	offset, patch, totalLen := computePatch(old, new)
	// Content is identical up to len(new), then old has extra chars
	s.Equal(len(new), offset)
	s.Equal("", patch)
	s.Equal(len(new), totalLen)
}

// TestComputePatch_MultiByte verifies UTF-16 code unit offsets for multi-byte content.
// This is the exact bug: Go's len() returns bytes but JS string.slice() uses UTF-16 code units.
// Without the fix, emoji/unicode before the diff point would shift the patch offset,
// causing content like "desktop" to be corrupted to "de Statussktop".
func (s *WebSocketSyncSuite) TestComputePatch_MultiByte() {
	// ✅ is U+2705: 3 bytes in UTF-8, 1 UTF-16 code unit
	// 📤 is U+1F4E4: 4 bytes in UTF-8, 2 UTF-16 code units (surrogate pair)
	old := "✅ Tool: Running"
	new := "✅ Tool: Finished"

	offset, patch, totalLen := computePatch(old, new)

	// "✅ Tool: " = 1 + 7 = 8 UTF-16 code units (NOT 10 bytes)
	// First diff is at 'R' vs 'F', which is UTF-16 offset 8
	s.Equal(8, offset, "offset should be in UTF-16 code units, not bytes")
	s.Equal("Finished", patch)

	// totalLen should be UTF-16 length of new content, not byte length
	// "✅ Tool: Finished" = 1 + 7 + 8 = 16 UTF-16 code units (NOT 19 bytes)
	s.Equal(16, totalLen, "totalLen should be UTF-16 code units")
	s.NotEqual(len(new), totalLen, "totalLen should differ from byte length for multi-byte content")

	// Test with supplementary plane character (surrogate pair in UTF-16)
	old2 := "📤 desktop/Status: Pending"
	new2 := "📤 desktop/Status: Complete"
	offset2, patch2, totalLen2 := computePatch(old2, new2)

	// "📤 desktop/Status: " = 2 + 17 = 19 UTF-16 code units (NOT 22 bytes)
	// 📤 is U+1F4E4: 4 bytes UTF-8, 2 UTF-16 code units (surrogate pair)
	s.Equal(19, offset2, "supplementary char should count as 2 UTF-16 code units")
	s.Equal("Complete", patch2)
	s.Equal(27, totalLen2) // "📤 desktop/Status: Complete" = 2 + 25 = 27 UTF-16 code units

	// Test append fast path with emoji prefix
	old3 := "✅ Hello"
	new3 := "✅ Hello world"
	offset3, patch3, _ := computePatch(old3, new3)
	s.Equal(7, offset3, "append offset should be UTF-16 length of old content")
	s.Equal(" world", patch3)
}

// TestStreamingPatch_PreviousContentTracked verifies that the streaming context
// tracks previousEntries for per-entry patch computation and updates after each publish.
func (s *WebSocketSyncSuite) TestStreamingPatch_PreviousEntriesTracked() {
	helixSessionID := "ses-patch-test"
	acpThreadID := "thread-patch-1"
	interactionID := "int-patch-1"

	// Setup context mapping
	s.server.contextMappingsMutex.Lock()
	s.server.contextMappings[acpThreadID] = helixSessionID
	s.server.contextMappingsMutex.Unlock()

	session := &types.Session{
		ID:           helixSessionID,
		Owner:        "user-1",
		GenerationID: 1,
	}
	interaction := &types.Interaction{
		ID:              interactionID,
		SessionID:       helixSessionID,
		State:           types.InteractionStateWaiting,
		ResponseMessage: "",
	}

	// First token: expect DB queries (cache miss) + DB write + publish
	s.store.EXPECT().GetSession(gomock.Any(), helixSessionID).Return(session, nil).Times(1)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return([]*types.Interaction{interaction}, int64(1), nil).Times(1)
	s.store.EXPECT().UpdateInteractionStreamingFields(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "message_added",
		Data: map[string]interface{}{
			"acp_thread_id": acpThreadID,
			"role":          "assistant",
			"message_id":    "msg-1",
			"content":       "Hello",
			"entry_type":    "text",
		},
	}

	// Send first token — creates streaming context
	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	// Verify previousEntries is updated after publish
	sctx := s.server.streamingContexts[helixSessionID]
	sctx.mu.Lock()
	s.Require().Len(sctx.previousEntries, 1, "should have 1 entry after first publish")
	s.Equal("Hello", sctx.previousEntries[0].Content)
	s.Equal("text", sctx.previousEntries[0].Type)
	s.Equal("msg-1", sctx.previousEntries[0].MessageID)
	sctx.mu.Unlock()

	// Expire throttles for second token
	sctx.mu.Lock()
	sctx.lastPublish = time.Now().Add(-100 * time.Millisecond)
	sctx.lastDBWrite = time.Now().Add(-300 * time.Millisecond)
	sctx.mu.Unlock()

	// Second token: append to same entry
	syncMsg.Data["content"] = "Hello world"
	err = s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	sctx.mu.Lock()
	s.Require().Len(sctx.previousEntries, 1)
	s.Equal("Hello world", sctx.previousEntries[0].Content, "entry content should track latest published content")
	sctx.mu.Unlock()

	// Third token: new entry (tool call)
	sctx.mu.Lock()
	sctx.lastPublish = time.Now().Add(-100 * time.Millisecond)
	sctx.lastDBWrite = time.Now().Add(-300 * time.Millisecond)
	sctx.mu.Unlock()

	syncMsg.Data["message_id"] = "msg-2"
	syncMsg.Data["content"] = "**Tool Call: list_files**"
	syncMsg.Data["entry_type"] = "tool_call"
	err = s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	sctx.mu.Lock()
	s.Require().Len(sctx.previousEntries, 2, "should have 2 entries after new message_id")
	s.Equal("Hello world", sctx.previousEntries[0].Content, "first entry unchanged")
	s.Equal("**Tool Call: list_files**", sctx.previousEntries[1].Content)
	s.Equal("tool_call", sctx.previousEntries[1].Type)
	sctx.mu.Unlock()
}

// ──────────────────────────────────────────────────────────────────────────────
// buildFullStatePatchEvent tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestBuildFullStatePatchEvent_Empty() {
	payload, err := buildFullStatePatchEvent("ses-1", "owner-1", "int-1", nil)
	s.NoError(err)
	s.Nil(payload, "empty entries should return nil payload")
}

func (s *WebSocketSyncSuite) TestBuildFullStatePatchEvent_FullState() {
	entries := []wsprotocol.ResponseEntry{
		{MessageID: "msg-1", Type: "text", Content: "Hello world"},
		{MessageID: "msg-2", Type: "tool_call", Content: "ls -la", ToolName: "bash", ToolStatus: "complete"},
	}

	payload, err := buildFullStatePatchEvent("ses-1", "owner-1", "int-1", entries)
	s.NoError(err)
	s.NotNil(payload)

	var event types.WebsocketEvent
	s.Require().NoError(json.Unmarshal(payload, &event))

	s.Equal(types.WebsocketEventInteractionPatch, event.Type)
	s.Equal("ses-1", event.SessionID)
	s.Equal("int-1", event.InteractionID)
	s.Equal(2, event.EntryCount)
	s.Require().Len(event.EntryPatches, 2)

	// All patches must have offset=0 (full replace, no delta baseline)
	for i, ep := range event.EntryPatches {
		s.Equal(0, ep.PatchOffset, "entry %d: patch_offset must be 0 for full-state snapshot", i)
		s.Equal(entries[i].Content, ep.Patch, "entry %d: patch must equal full content", i)
		s.Equal(entries[i].Type, ep.Type, "entry %d: type preserved", i)
		s.Equal(entries[i].MessageID, ep.MessageID, "entry %d: message_id preserved", i)
	}

	s.Equal("bash", event.EntryPatches[1].ToolName)
	s.Equal("complete", event.EntryPatches[1].ToolStatus)
}

// TestLateJoinerCatchUp verifies that a streaming context present at WebSocket
// connect time produces a full-state patch via buildFullStatePatchEvent.
func (s *WebSocketSyncSuite) TestLateJoinerCatchUp_ActiveStreamingContext() {
	acc := &wsprotocol.MessageAccumulator{}
	acc.AddMessageWithType("msg-1", "I'll start by exploring", "text")
	acc.AddMessageWithType("msg-2", "**Tool Call: list_files**", "tool_call")

	s.server.streamingContextsMu.Lock()
	s.server.streamingContexts["ses-lj"] = &streamingContext{
		session:     &types.Session{ID: "ses-lj", Owner: "owner-lj"},
		interaction: &types.Interaction{ID: "int-lj"},
		accumulator: acc,
	}
	s.server.streamingContextsMu.Unlock()

	// Simulate what websocket_server_user.go does on connect
	s.server.streamingContextsMu.RLock()
	sctx := s.server.streamingContexts["ses-lj"]
	s.server.streamingContextsMu.RUnlock()

	s.Require().NotNil(sctx)

	sctx.mu.Lock()
	entries := sctx.accumulator.Entries()
	interactionID := sctx.interaction.ID
	owner := sctx.session.Owner
	sctx.mu.Unlock()

	payload, err := buildFullStatePatchEvent("ses-lj", owner, interactionID, entries)
	s.NoError(err)
	s.Require().NotNil(payload)

	var event types.WebsocketEvent
	s.Require().NoError(json.Unmarshal(payload, &event))

	s.Equal(types.WebsocketEventInteractionPatch, event.Type)
	s.Require().Len(event.EntryPatches, 2)

	s.Equal(0, event.EntryPatches[0].PatchOffset)
	s.Equal("I'll start by exploring", event.EntryPatches[0].Patch)
	s.Equal("text", event.EntryPatches[0].Type)

	s.Equal(0, event.EntryPatches[1].PatchOffset)
	s.Equal("**Tool Call: list_files**", event.EntryPatches[1].Patch)
	s.Equal("tool_call", event.EntryPatches[1].Type)
}

// ──────────────────────────────────────────────────────────────────────────────
// pickupWaitingInteraction tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestPickupWaitingInteraction_FallbackCreatesMapping() {
	// Scenario: session created via session handler (NOT sendMessageToSpecTaskAgent),
	// so requestToSessionMapping has no entry. The fallback should create one
	// using the interaction ID as request_id.
	sessionID := "ses_test123"
	interactionID := "int_waiting1"

	session := &types.Session{
		ID:           sessionID,
		GenerationID: 1,
		Metadata:     types.SessionMetadata{},
	}

	interactions := []*types.Interaction{
		{ID: "int_done", State: types.InteractionStateComplete, PromptMessage: "old"},
		{ID: interactionID, State: types.InteractionStateWaiting, PromptMessage: "Fix the bug"},
	}

	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(interactions, int64(2), nil)

	// Init readiness state so queueOrSend queues the command (not ready yet)
	s.server.externalAgentWSManager.initReadinessState(sessionID, false, nil)
	defer s.server.externalAgentWSManager.cleanupReadinessState(sessionID)

	s.server.pickupWaitingInteraction(context.Background(), sessionID, session, "agent-1")

	// Verify fallback created the mapping
	s.Equal(sessionID, s.server.requestToSessionMapping[interactionID],
		"fallback should map interaction ID → session ID")

	// Verify requestToInteractionMapping populated
	s.Equal(interactionID, s.server.requestToInteractionMapping[interactionID])

	// Verify command was queued in pending queue
	s.server.externalAgentWSManager.readinessMu.Lock()
	state := s.server.externalAgentWSManager.readinessState[sessionID]
	s.server.externalAgentWSManager.readinessMu.Unlock()
	s.Require().NotNil(state)
	s.Require().Len(state.PendingQueue, 1)
	s.Equal("chat_message", state.PendingQueue[0].Type)
	s.Equal(interactionID, state.PendingQueue[0].Data["request_id"])
	s.Equal("Fix the bug", state.PendingQueue[0].Data["message"])
}

func (s *WebSocketSyncSuite) TestPickupWaitingInteraction_UsesExistingMapping() {
	// Scenario: sendMessageToSpecTaskAgent already populated requestToSessionMapping.
	// pickupWaitingInteraction should use the existing request_id, not create a new one.
	sessionID := "ses_test456"
	existingReqID := "req_from_send"
	interactionID := "int_waiting2"

	s.server.requestToSessionMapping[existingReqID] = sessionID

	session := &types.Session{
		ID:           sessionID,
		GenerationID: 1,
		Metadata:     types.SessionMetadata{},
	}

	interactions := []*types.Interaction{
		{ID: interactionID, State: types.InteractionStateWaiting, PromptMessage: "Deploy to prod"},
	}

	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(interactions, int64(1), nil)

	s.server.externalAgentWSManager.initReadinessState(sessionID, false, nil)
	defer s.server.externalAgentWSManager.cleanupReadinessState(sessionID)

	s.server.pickupWaitingInteraction(context.Background(), sessionID, session, "agent-2")

	// Existing mapping should still be there, unchanged
	s.Equal(sessionID, s.server.requestToSessionMapping[existingReqID])

	// No new mapping should be created for the interaction ID
	_, fallbackExists := s.server.requestToSessionMapping[interactionID]
	s.False(fallbackExists, "should not create fallback mapping when existing one found")

	// Command should use the existing request_id
	s.server.externalAgentWSManager.readinessMu.Lock()
	state := s.server.externalAgentWSManager.readinessState[sessionID]
	s.server.externalAgentWSManager.readinessMu.Unlock()
	s.Require().Len(state.PendingQueue, 1)
	s.Equal(existingReqID, state.PendingQueue[0].Data["request_id"])
}

func (s *WebSocketSyncSuite) TestPickupWaitingInteraction_NoWaitingInteraction() {
	// Scenario: all interactions are complete — nothing to pick up.
	sessionID := "ses_test789"

	session := &types.Session{
		ID:           sessionID,
		GenerationID: 1,
		Metadata:     types.SessionMetadata{},
	}

	interactions := []*types.Interaction{
		{ID: "int_done1", State: types.InteractionStateComplete, PromptMessage: "done"},
		{ID: "int_done2", State: types.InteractionStateComplete, PromptMessage: "also done"},
	}

	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(interactions, int64(2), nil)

	s.server.externalAgentWSManager.initReadinessState(sessionID, false, nil)
	defer s.server.externalAgentWSManager.cleanupReadinessState(sessionID)

	s.server.pickupWaitingInteraction(context.Background(), sessionID, session, "agent-3")

	// No mappings should be created
	s.Empty(s.server.requestToSessionMapping)
	s.Empty(s.server.requestToInteractionMapping)

	// No command should be queued
	s.server.externalAgentWSManager.readinessMu.Lock()
	state := s.server.externalAgentWSManager.readinessState[sessionID]
	s.server.externalAgentWSManager.readinessMu.Unlock()
	s.Empty(state.PendingQueue)
}

func (s *WebSocketSyncSuite) TestPickupWaitingInteraction_WithSystemPrompt() {
	// Verify system prompt is combined with user message correctly.
	sessionID := "ses_sysprompt"
	interactionID := "int_sysprompt"

	session := &types.Session{
		ID:           sessionID,
		GenerationID: 1,
		Metadata:     types.SessionMetadata{},
	}

	interactions := []*types.Interaction{
		{
			ID:            interactionID,
			State:         types.InteractionStateWaiting,
			SystemPrompt:  "You are a coding assistant.",
			PromptMessage: "Fix the tests",
		},
	}

	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(interactions, int64(1), nil)

	s.server.externalAgentWSManager.initReadinessState(sessionID, false, nil)
	defer s.server.externalAgentWSManager.cleanupReadinessState(sessionID)

	s.server.pickupWaitingInteraction(context.Background(), sessionID, session, "agent-4")

	s.server.externalAgentWSManager.readinessMu.Lock()
	state := s.server.externalAgentWSManager.readinessState[sessionID]
	s.server.externalAgentWSManager.readinessMu.Unlock()
	s.Require().Len(state.PendingQueue, 1)
	s.Equal("You are a coding assistant.\n\n**User Request:**\nFix the tests",
		state.PendingQueue[0].Data["message"])
}

func (s *WebSocketSyncSuite) TestPickupWaitingInteraction_ResumesExistingThread() {
	// When session has a ZedThreadID, the command should include it for thread resume.
	sessionID := "ses_resume"
	interactionID := "int_resume"
	zedThreadID := "thread-abc-123"

	session := &types.Session{
		ID:           sessionID,
		GenerationID: 1,
		Metadata: types.SessionMetadata{
			ZedThreadID: zedThreadID,
		},
	}

	interactions := []*types.Interaction{
		{ID: interactionID, State: types.InteractionStateWaiting, PromptMessage: "Continue"},
	}

	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(interactions, int64(1), nil)

	s.server.externalAgentWSManager.initReadinessState(sessionID, false, nil)
	defer s.server.externalAgentWSManager.cleanupReadinessState(sessionID)

	s.server.pickupWaitingInteraction(context.Background(), sessionID, session, "agent-5")

	s.server.externalAgentWSManager.readinessMu.Lock()
	state := s.server.externalAgentWSManager.readinessState[sessionID]
	s.server.externalAgentWSManager.readinessMu.Unlock()
	s.Require().Len(state.PendingQueue, 1)
	s.Equal(zedThreadID, state.PendingQueue[0].Data["acp_thread_id"])
}

// --- handleUserCreatedThread tests ---

func (s *WebSocketSyncSuite) TestUserCreatedThread_CreatesWorkSessionForSpectask() {
	// Setup: existing session with spectask metadata
	existingSession := &types.Session{
		ID:             "ses_existing",
		Owner:          "user-1",
		OrganizationID: "org-1",
		ProjectID:      "prj-1",
		ParentApp:      "app-1",
		Metadata: types.SessionMetadata{
			AgentType:        "zed_external",
			SpecTaskID:       "spt_test",
			CodeAgentRuntime: "zed_agent",
			ZedThreadID:      "thread-original",
		},
	}

	s.store.EXPECT().GetSession(gomock.Any(), "ses_existing").Return(existingSession, nil)

	// Phantom-draft guard: returns empty list (no existing zed_threads to dedup
	// against), so the guard falls through to the normal create path.
	s.store.EXPECT().ListSpecTaskZedThreads(gomock.Any(), "spt_test").
		Return([]*types.SpecTaskZedThread{}, nil)

	// Expect new session to be created with all metadata copied
	var capturedSession types.Session
	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			capturedSession = session
			return &session, nil
		},
	)

	// Expect existing work session lookup for phase
	existingWorkSession := &types.SpecTaskWorkSession{
		ID:             "stws_existing",
		SpecTaskID:     "spt_test",
		HelixSessionID: "ses_existing",
		Phase:          types.SpecTaskPhaseImplementation,
		Status:         types.SpecTaskWorkSessionStatusActive,
	}
	s.store.EXPECT().GetSpecTaskWorkSessionByHelixSession(gomock.Any(), "ses_existing").
		Return(existingWorkSession, nil)

	// Expect work session creation
	var capturedWorkSession *types.SpecTaskWorkSession
	s.store.EXPECT().CreateSpecTaskWorkSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, ws *types.SpecTaskWorkSession) error {
			capturedWorkSession = ws
			return nil
		},
	)

	// Expect zed thread creation
	var capturedZedThread *types.SpecTaskZedThread
	s.store.EXPECT().CreateSpecTaskZedThread(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, zt *types.SpecTaskZedThread) error {
			capturedZedThread = zt
			return nil
		},
	)

	syncMsg := &types.SyncMessage{
		EventType: "user_created_thread",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-new-from-user",
			"title":         "My New Thread",
		},
	}

	err := s.server.handleUserCreatedThread("ses_existing", syncMsg)
	s.NoError(err)

	// Verify new session has all metadata from parent
	s.Equal("spt_test", capturedSession.Metadata.SpecTaskID)
	s.Equal(types.CodeAgentRuntime("zed_agent"), capturedSession.Metadata.CodeAgentRuntime)
	s.Equal("zed_external", capturedSession.Metadata.AgentType)
	s.Equal("thread-new-from-user", capturedSession.Metadata.ZedThreadID)
	s.Equal("prj-1", capturedSession.ProjectID)
	s.Equal("org-1", capturedSession.OrganizationID)
	s.Equal("app-1", capturedSession.ParentApp)
	s.Equal("My New Thread", capturedSession.Name)

	// Verify work session created with correct phase
	s.Require().NotNil(capturedWorkSession)
	s.Equal("spt_test", capturedWorkSession.SpecTaskID)
	s.Equal(types.SpecTaskPhaseImplementation, capturedWorkSession.Phase)
	s.Equal(types.SpecTaskWorkSessionStatusActive, capturedWorkSession.Status)

	// Verify zed thread created
	s.Require().NotNil(capturedZedThread)
	s.Equal("spt_test", capturedZedThread.SpecTaskID)
	s.Equal("thread-new-from-user", capturedZedThread.ZedThreadID)
	s.Equal(types.SpecTaskZedStatusActive, capturedZedThread.Status)

	// Verify context mapping updated
	s.server.contextMappingsMutex.RLock()
	mappedSession := s.server.contextMappings["thread-new-from-user"]
	s.server.contextMappingsMutex.RUnlock()
	s.Equal(capturedSession.ID, mappedSession)
}

// TestUserCreatedThread_PhantomDraftGuard_RefusesWhenEmptyWorkSessionExists
// is the regression test for the bug documented in
// design/2026-05-13-mcp-cache-contention-and-duplicate-claude-spawn.md.
//
// Without the guard at handleUserCreatedThread, every container restart
// of a long-running spec_task leaks an empty "New Chat" helix_session +
// spec_task_zed_threads row. Cause: Zed's agent panel speculatively calls
// new_session() to back its empty input editor (the "draft" thread), then
// fires UserCreatedThread back to us — even though the user never typed
// anything in it.
//
// The guard refuses to create a new session if the spec_task already has
// an active work_session whose helix_session has zero interactions.
//
// To make this test fail when the guard is removed, comment out the
// "PHANTOM-DRAFT GUARD" block in handleUserCreatedThread and re-run.
func (s *WebSocketSyncSuite) TestUserCreatedThread_PhantomDraftGuard_RefusesWhenEmptyWorkSessionExists() {
	// Existing helix_session that the dev container is bound to.
	existingSession := &types.Session{
		ID:             "ses_existing",
		Owner:          "user-1",
		OrganizationID: "org-1",
		ProjectID:      "prj-1",
		ParentApp:      "app-1",
		Metadata: types.SessionMetadata{
			AgentType:        "zed_external",
			SpecTaskID:       "spt_phantom_test",
			CodeAgentRuntime: "claude_code",
			ZedThreadID:      "thread-real",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_existing").Return(existingSession, nil)

	// The spec_task already has one active zed_thread (thread-real) tied to
	// a helix_session with no interactions. This is the scenario the bug
	// produces on every container restart.
	existingZedThread := &types.SpecTaskZedThread{
		ID:            "stzt_existing",
		WorkSessionID: "stws_existing",
		SpecTaskID:    "spt_phantom_test",
		ZedThreadID:   "thread-real",
		Status:        types.SpecTaskZedStatusActive,
	}
	s.store.EXPECT().ListSpecTaskZedThreads(gomock.Any(), "spt_phantom_test").
		Return([]*types.SpecTaskZedThread{existingZedThread}, nil)

	existingWorkSession := &types.SpecTaskWorkSession{
		ID:             "stws_existing",
		SpecTaskID:     "spt_phantom_test",
		HelixSessionID: "ses_existing",
		Status:         types.SpecTaskWorkSessionStatusActive,
	}
	s.store.EXPECT().GetSpecTaskWorkSession(gomock.Any(), "stws_existing").
		Return(existingWorkSession, nil)

	// helix_session has zero interactions — this is the signal that the
	// existing work_session is itself a phantom draft (or just not yet
	// touched by the user). The incoming UserCreatedThread is therefore a
	// duplicate phantom from another panel-restore cycle. Refuse it.
	s.store.EXPECT().ListInteractions(gomock.Any(), &types.ListInteractionsQuery{
		SessionID: "ses_existing",
	}).Return([]*types.Interaction{}, int64(0), nil)

	// THE ASSERTION: the guard must short-circuit BEFORE any of these
	// store mutations fire. If the guard is removed, gomock will fail
	// with "missing call to CreateSession" / "missing call to
	// CreateSpecTaskWorkSession" / "missing call to CreateSpecTaskZedThread"
	// because the handler will fall through to the create path (which we
	// have NOT mocked here). That test failure IS the regression signal.

	syncMsg := &types.SyncMessage{
		EventType: "user_created_thread",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-phantom-from-zed-draft",
			"title":         "New Chat",
		},
	}

	err := s.server.handleUserCreatedThread("ses_existing", syncMsg)
	s.NoError(err, "guard should silently skip creation, not return an error")

	// Belt-and-braces: also verify no context mapping was created for the
	// phantom thread_id (it would only be set if we'd fallen through to
	// the create path).
	s.server.contextMappingsMutex.RLock()
	_, mapped := s.server.contextMappings["thread-phantom-from-zed-draft"]
	s.server.contextMappingsMutex.RUnlock()
	s.False(mapped, "phantom thread should not be added to contextMappings")
}

// TestUserCreatedThread_PhantomDraftGuard_AllowsWhenExistingSessionHasInteractions
// verifies the guard does NOT block when the existing work_session has
// real activity in it. A user typing a follow-up that creates a genuinely
// new thread on top of an active conversation MUST still work.
func (s *WebSocketSyncSuite) TestUserCreatedThread_PhantomDraftGuard_AllowsWhenExistingSessionHasInteractions() {
	existingSession := &types.Session{
		ID:             "ses_existing",
		Owner:          "user-1",
		OrganizationID: "org-1",
		Metadata: types.SessionMetadata{
			AgentType:        "zed_external",
			SpecTaskID:       "spt_active_test",
			CodeAgentRuntime: "claude_code",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_existing").Return(existingSession, nil)

	existingZedThread := &types.SpecTaskZedThread{
		ID:            "stzt_existing",
		WorkSessionID: "stws_existing",
		SpecTaskID:    "spt_active_test",
		ZedThreadID:   "thread-active",
		Status:        types.SpecTaskZedStatusActive,
	}
	s.store.EXPECT().ListSpecTaskZedThreads(gomock.Any(), "spt_active_test").
		Return([]*types.SpecTaskZedThread{existingZedThread}, nil)

	existingWorkSession := &types.SpecTaskWorkSession{
		ID:             "stws_existing",
		SpecTaskID:     "spt_active_test",
		HelixSessionID: "ses_existing",
		Status:         types.SpecTaskWorkSessionStatusActive,
	}
	s.store.EXPECT().GetSpecTaskWorkSession(gomock.Any(), "stws_existing").
		Return(existingWorkSession, nil)

	// Existing session HAS interactions → guard does not fire → fall through
	// to the create path.
	s.store.EXPECT().ListInteractions(gomock.Any(), &types.ListInteractionsQuery{
		SessionID: "ses_existing",
	}).Return([]*types.Interaction{{ID: "int_one"}}, int64(1), nil)

	// Expect normal create path to execute.
	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(&types.Session{ID: "ses_new_active"}, nil)
	s.store.EXPECT().GetSpecTaskWorkSessionByHelixSession(gomock.Any(), "ses_existing").
		Return(existingWorkSession, nil)
	s.store.EXPECT().CreateSpecTaskWorkSession(gomock.Any(), gomock.Any()).Return(nil)
	s.store.EXPECT().CreateSpecTaskZedThread(gomock.Any(), gomock.Any()).Return(nil)

	syncMsg := &types.SyncMessage{
		EventType: "user_created_thread",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-genuinely-new",
			"title":         "Continuation",
		},
	}

	err := s.server.handleUserCreatedThread("ses_existing", syncMsg)
	s.NoError(err)
}

func (s *WebSocketSyncSuite) TestUserCreatedThread_NonSpectaskSkipsWorkSession() {
	// Session without SpecTaskID — should create session but skip work session
	existingSession := &types.Session{
		ID:             "ses_exploratory",
		Owner:          "user-1",
		OrganizationID: "org-1",
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
			// No SpecTaskID
		},
	}

	s.store.EXPECT().GetSession(gomock.Any(), "ses_exploratory").Return(existingSession, nil)
	s.store.EXPECT().CreateSession(gomock.Any(), gomock.Any()).Return(&types.Session{ID: "ses_new_exploratory"}, nil)

	// No CreateSpecTaskWorkSession or CreateSpecTaskZedThread expected

	syncMsg := &types.SyncMessage{
		EventType: "user_created_thread",
		Data: map[string]interface{}{
			"acp_thread_id": "thread-exploratory",
			"title":         "Exploratory Chat",
		},
	}

	err := s.server.handleUserCreatedThread("ses_exploratory", syncMsg)
	s.NoError(err)
}

// ──────────────────────────────────────────────────────────────────────────────
// sendChatMessageToExternalAgent — request→session mapping for new threads
//
// Regression test for the live-clear orphaning bug: after ClearSession resets a
// Zed session's ZedThreadID to "", the next message is sent with acp_thread_id=nil
// so the agent creates a NEW thread and emits thread_created. Without a
// request_id→session mapping, handleThreadCreated could not reattach that thread
// to the originating session and spawned an orphan session, leaving the original
// interaction stuck in "waiting". sendChatMessageToExternalAgent must therefore
// register requestToSessionMapping whenever it sends with no existing thread.
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestSendChatMessage_NewThread_RegistersSessionMapping() {
	sessionID := "ses_cleared"
	requestID := "req_newthread"

	// A live WS connection so sendCommandToExternalAgent succeeds (no auto-start path).
	s.server.externalAgentWSManager.registerConnection(sessionID, &ExternalAgentWSConnection{
		SessionID: sessionID,
		SendChan:  make(chan types.ExternalAgentCommand, 10),
	})

	session := &types.Session{
		ID:    sessionID,
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType:    "zed_external",
			ZedThreadID:  "", // cleared — next message creates a NEW thread
			ZedAgentName: "claude",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, i *types.Interaction) (*types.Interaction, error) {
			i.ID = "int_new"
			return i, nil
		})

	_, _ = s.server.sendChatMessageToExternalAgent(sessionID, "hello", requestID, false)

	s.server.contextMappingsMutex.RLock()
	mapped, ok := s.server.requestToSessionMapping[requestID]
	s.server.contextMappingsMutex.RUnlock()
	s.True(ok, "new-thread send must register request_id → session so thread_created reattaches")
	s.Equal(sessionID, mapped)
}

func (s *WebSocketSyncSuite) TestSendChatMessage_ExistingThread_NoSessionMapping() {
	sessionID := "ses_with_thread"
	requestID := "req_existing"

	s.server.externalAgentWSManager.registerConnection(sessionID, &ExternalAgentWSConnection{
		SessionID: sessionID,
		SendChan:  make(chan types.ExternalAgentCommand, 10),
	})

	session := &types.Session{
		ID:    sessionID,
		Owner: "user-1",
		Metadata: types.SessionMetadata{
			AgentType:    "zed_external",
			ZedThreadID:  "thr-existing", // continues the SAME thread; no thread_created
			ZedAgentName: "claude",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil)
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, i *types.Interaction) (*types.Interaction, error) {
			i.ID = "int_existing"
			return i, nil
		})

	_, _ = s.server.sendChatMessageToExternalAgent(sessionID, "hello", requestID, false)

	s.server.contextMappingsMutex.RLock()
	_, ok := s.server.requestToSessionMapping[requestID]
	s.server.contextMappingsMutex.RUnlock()
	s.False(ok, "same-thread continuation must NOT register a mapping (would leak — no thread_created consumes it)")
}

// TestLockPromptDrainSerializesPerSession verifies the per-session drain lock
// (a) serialises concurrent drains for the SAME session — the core fix for the
// out-of-order dispatch race — and (b) does NOT block drains for DIFFERENT
// sessions. See design/2026-06-23-queue-drain-out-of-order-dispatch.md.
func TestLockPromptDrainSerializesPerSession(t *testing.T) {
	apiServer := &HelixAPIServer{}

	// (a) Same session: a held lock must block a second acquisition until released.
	unlock := apiServer.lockPromptDrain("ses_same")
	acquired := make(chan struct{})
	go func() {
		release := apiServer.lockPromptDrain("ses_same")
		close(acquired)
		release()
	}()
	select {
	case <-acquired:
		t.Fatal("second lockPromptDrain for the same session acquired while first was held — not serialised")
	case <-time.After(100 * time.Millisecond):
		// expected: still blocked
	}
	unlock()
	select {
	case <-acquired:
		// expected: unblocked after release
	case <-time.After(time.Second):
		t.Fatal("second lockPromptDrain did not acquire after the first was released")
	}

	// (b) Different session: must not block even while ses_same's lock is held.
	hold := apiServer.lockPromptDrain("ses_same")
	defer hold()
	other := make(chan struct{})
	go func() {
		release := apiServer.lockPromptDrain("ses_other")
		close(other)
		release()
	}()
	select {
	case <-other:
		// expected: different session is independent
	case <-time.After(time.Second):
		t.Fatal("lockPromptDrain for a different session blocked — lock is not per-session")
	}
}
