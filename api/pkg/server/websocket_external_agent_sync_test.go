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
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
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

	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			s.Equal("Hello from AI", interaction.ResponseMessage)
			s.Equal("msg-1", interaction.LastZedMessageID)
			return interaction, nil
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

	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			// Same message_id → content replaced from offset (streaming update)
			s.Equal("Hello, world!", interaction.ResponseMessage)
			s.Equal("msg-A", interaction.LastZedMessageID)
			return interaction, nil
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

	// Interaction already has content from msg-A
	existingInteraction := &types.Interaction{
		ID:                   "int-3",
		SessionID:            "ses_3",
		State:                types.InteractionStateWaiting,
		ResponseMessage:      "First message",
		LastZedMessageID:     "msg-A",
		LastZedMessageOffset: 0,
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{existingInteraction}, int64(1), nil,
	)

	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			// New message_id → content appended with \n\n separator
			s.Equal("First message\n\nSecond message", interaction.ResponseMessage)
			s.Equal("msg-B", interaction.LastZedMessageID)
			s.Equal(len("First message")+2, interaction.LastZedMessageOffset)
			return interaction, nil
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
		ID:        "int-mc-fb",
		SessionID: "ses_mc_fb",
		State:     types.InteractionStateWaiting,
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
		ID:        "int-cf",
		SessionID: "ses_cf",
		State:     types.InteractionStateWaiting,
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
	s.store.EXPECT().MarkPromptAsSent(gomock.Any(), "prompt-1").Return(nil)

	// sendQueuedPromptToSession calls
	session := &types.Session{
		ID:    "ses_pending",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_pending").Return(session, nil).AnyTimes()
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			return &types.Interaction{ID: "int-prompt", SessionID: "ses_pending"}, nil
		},
	)
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
	// MarkPromptAsPending no longer called - prompt is atomically claimed by GetNextPendingPrompt

	// sendQueuedPromptToSession calls
	session := &types.Session{
		ID:    "ses_pq",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_pq").Return(session, nil).AnyTimes()
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(
		&types.Interaction{ID: "int-pq", SessionID: "ses_pq"}, nil,
	)
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()

	// sendCommandToExternalAgent will fail (no WS connection), but interaction was created
	// so MarkPromptAsSent is still called
	s.store.EXPECT().MarkPromptAsSent(gomock.Any(), "prompt-pq").Return(nil)

	s.server.processPromptQueue(context.Background(), "ses_pq")
	time.Sleep(50 * time.Millisecond) // let autoStartDevContainerForSession goroutine complete
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
	s.store.EXPECT().MarkPromptAsFailed(gomock.Any(), "prompt-fail").Return(nil)

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
	// Note: ClaimPromptForSending is NOT called — GetAnyPendingPrompt already atomically claimed it
	s.store.EXPECT().MarkPromptAsSent(gomock.Any(), "prompt-any").Return(nil)

	session := &types.Session{
		ID:    "ses_any",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_any").Return(session, nil).AnyTimes()
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(
		&types.Interaction{ID: "int-any", SessionID: "ses_any"}, nil,
	)
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()

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
	s.store.EXPECT().MarkPromptAsFailed(gomock.Any(), "prompt-anyfail").Return(nil)

	s.server.processAnyPendingPrompt(context.Background(), "ses_anyfail")
}

// ──────────────────────────────────────────────────────────────────────────────
// sendQueuedPromptToSession — in-memory state cleanup on send failure
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestSendQueuedPrompt_SendFails_CleansUpInMemoryState() {
	// When sendCommandToExternalAgent fails (no WS connection),
	// requestToSessionMapping and requestToInteractionMapping must be cleaned
	// up so pickupWaitingInteraction sets them fresh on reconnect.
	// Otherwise a stale message_completed from the agent's previous context
	// gets matched to the new interaction (wrong response bug).

	session := &types.Session{
		ID:    "ses_cleanup",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_cleanup").Return(session, nil).AnyTimes()
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(
		&types.Interaction{ID: "int-cleanup", SessionID: "ses_cleanup"}, nil,
	)
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()

	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-cleanup",
		SessionID: "ses_cleanup",
		Content:   "test prompt",
	}

	// No WS connection registered → sendCommandToExternalAgent will fail
	err := s.server.sendQueuedPromptToSession(context.Background(), "ses_cleanup", prompt)
	s.NoError(err) // interaction was created, send failure is not returned as error

	time.Sleep(50 * time.Millisecond) // let autoStartDevContainerForSession goroutine complete

	// Verify in-memory state was cleaned up
	s.server.contextMappingsMutex.Lock()
	_, hasSessionMapping := s.server.requestToSessionMapping["int-cleanup"]
	_, hasInteractionMapping := s.server.requestToInteractionMapping["int-cleanup"]
	s.server.contextMappingsMutex.Unlock()

	s.False(hasSessionMapping, "requestToSessionMapping should be cleaned up after send failure")
	s.False(hasInteractionMapping, "requestToInteractionMapping should be cleaned up after send failure")
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
	// Second token within 200ms is throttled — no UpdateInteraction call.
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			s.Equal("Hello", interaction.ResponseMessage)
			return interaction, nil
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

	// Verify in-memory content is updated despite no DB write
	sctx.mu.Lock()
	s.Equal("Hello, world!", sctx.interaction.ResponseMessage)
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
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			return interaction, nil
		},
	).Times(1)

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

	// Verify dirty flag
	s.server.streamingContextsMu.RLock()
	sctx := s.server.streamingContexts["ses_throttle"]
	s.server.streamingContextsMu.RUnlock()
	sctx.mu.Lock()
	s.True(sctx.dirty, "should be dirty after throttled write")
	s.Equal("Token 1 Token 2", sctx.interaction.ResponseMessage)
	// Artificially expire the throttle interval
	sctx.lastDBWrite = time.Now().Add(-300 * time.Millisecond)
	sctx.mu.Unlock()

	// Now expect another DB write since interval expired
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			s.Equal("Token 1 Token 2 Token 3", interaction.ResponseMessage)
			return interaction, nil
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

	// flushAndClearStreamingContext should flush the dirty interaction
	flushUpdate := s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			s.Equal("dirty unflushed content", interaction.ResponseMessage)
			s.Equal(types.InteractionStateWaiting, interaction.State) // Not yet complete
			return interaction, nil
		},
	)

	// handleMessageCompleted then does its normal flow
	s.store.EXPECT().GetSession(gomock.Any(), "ses_flush").Return(session, nil).AnyTimes()
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{existingInteraction}, int64(1), nil,
	).AnyTimes()
	s.store.EXPECT().GetInteraction(gomock.Any(), "int-flush").Return(&types.Interaction{
		ID:              "int-flush",
		SessionID:       "ses_flush",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "dirty unflushed content", // DB was just flushed
	}, nil)

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
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			return interaction, nil
		},
	).Times(1)

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

	// Verify accumulated content in memory
	s.server.streamingContextsMu.RLock()
	sctx := s.server.streamingContexts["ses_multi"]
	s.server.streamingContextsMu.RUnlock()

	sctx.mu.Lock()
	s.Equal("Let me help\n\n[Running: ls -la]", sctx.interaction.ResponseMessage)
	s.Equal("msg-tool", sctx.interaction.LastZedMessageID)

	// Third update: tool call status changes (same message_id, content replaces from offset)
	sctx.mu.Unlock()

	syncMsg.Data["content"] = "[Finished: ls -la]\nfile1.txt\nfile2.txt"
	err = s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	sctx.mu.Lock()
	s.Equal("Let me help\n\n[Finished: ls -la]\nfile1.txt\nfile2.txt", sctx.interaction.ResponseMessage)
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
	s.store.EXPECT().UpdateInteraction(gomock.Any(), gomock.Any()).Return(interaction, nil).AnyTimes()

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
