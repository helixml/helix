package server

import (
	"context"
	"fmt"
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
		streamingContexts:          make(map[string]*streamingContext),
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

	// Verify contextMappings and sessionToWaitingInteraction populated
	s.Equal("ses_new", s.server.contextMappings["thread-new"])
	s.Equal("int-new", s.server.sessionToWaitingInteraction["ses_new"])
}

func (s *WebSocketSyncSuite) TestThreadCreated_Priority3_SpectaskLink() {
	// sessionID starts with "ses_" and the original has a SpecTaskID
	s.server.externalAgentUserMapping["ses_original"] = "user-1"

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

	// UpdateSession to copy SpecTaskID
	s.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, session types.Session) (*types.Session, error) {
			s.Equal("spec-task-123", session.Metadata.SpecTaskID)
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
	s.server.sessionToWaitingInteraction["ses_1"] = "int-1"

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
	s.server.sessionToWaitingInteraction["ses_2"] = "int-2"

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
	s.server.sessionToWaitingInteraction["ses_3"] = "int-3"

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

	// Verify sessionToWaitingInteraction was updated
	s.Equal("int-user-new", s.server.sessionToWaitingInteraction["ses_user"])
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
	s.store.EXPECT().GetSession(gomock.Any(), "ses_mc").Return(session, nil).Times(2) // once for handler, once for final publish

	waitingInteraction := &types.Interaction{
		ID:              "int-mc",
		SessionID:       "ses_mc",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "AI response",
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{waitingInteraction}, int64(1), nil,
	).Times(2) // once for finding waiting, once for final publish

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
	s.store.EXPECT().GetSession(gomock.Any(), "ses_mc_fb").Return(session, nil).Times(2)

	waitingInteraction := &types.Interaction{
		ID:        "int-mc-fb",
		SessionID: "ses_mc_fb",
		State:     types.InteractionStateWaiting,
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{waitingInteraction}, int64(1), nil,
	).Times(2)
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
	s.store.EXPECT().GetSession(gomock.Any(), "ses_cf").Return(session, nil).Times(2)

	waitingInteraction := &types.Interaction{
		ID:        "int-cf",
		SessionID: "ses_cf",
		State:     types.InteractionStateWaiting,
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{waitingInteraction}, int64(1), nil,
	).Times(2)
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
	s.store.EXPECT().MarkPromptAsSent(gomock.Any(), "prompt-1").Return(nil)

	// sendQueuedPromptToSession calls
	session := &types.Session{
		ID:    "ses_pending",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_pending").Return(session, nil)
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			return &types.Interaction{ID: "int-prompt", SessionID: "ses_pending"}, nil
		},
	)
	// GetSpecTask for getAgentNameForSession
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()

	syncMsg := &types.SyncMessage{
		EventType: "agent_ready",
		Data:      map[string]interface{}{},
	}

	err := s.server.handleAgentReady("ses_pending", syncMsg)
	s.NoError(err)

	time.Sleep(100 * time.Millisecond)
}

// ──────────────────────────────────────────────────────────────────────────────
// processPromptQueue tests
// ──────────────────────────────────────────────────────────────────────────────

func (s *WebSocketSyncSuite) TestProcessPromptQueue_NoPending() {
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_nopq").Return(nil, nil)

	s.server.processPromptQueue(context.Background(), "ses_nopq")
	// No further store calls expected
}

func (s *WebSocketSyncSuite) TestProcessPromptQueue_HasPending() {
	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-pq",
		SessionID: "ses_pq",
		Content:   "queued content",
		Status:    "pending",
	}
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_pq").Return(prompt, nil)
	s.store.EXPECT().MarkPromptAsPending(gomock.Any(), "prompt-pq").Return(nil)

	// sendQueuedPromptToSession calls
	session := &types.Session{
		ID:    "ses_pq",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_pq").Return(session, nil)
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(
		&types.Interaction{ID: "int-pq", SessionID: "ses_pq"}, nil,
	)
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()

	// sendCommandToExternalAgent will fail (no WS connection), but interaction was created
	// so MarkPromptAsSent is still called
	s.store.EXPECT().MarkPromptAsSent(gomock.Any(), "prompt-pq").Return(nil)

	s.server.processPromptQueue(context.Background(), "ses_pq")
}

func (s *WebSocketSyncSuite) TestProcessPromptQueue_SendFails_GetSessionFails() {
	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-fail",
		SessionID: "ses_fail",
		Content:   "fail content",
	}
	s.store.EXPECT().GetNextPendingPrompt(gomock.Any(), "ses_fail").Return(prompt, nil)
	s.store.EXPECT().MarkPromptAsPending(gomock.Any(), "prompt-fail").Return(nil)

	// sendQueuedPromptToSession fails because GetSession fails
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
	s.store.EXPECT().MarkPromptAsSent(gomock.Any(), "prompt-any").Return(nil)

	session := &types.Session{
		ID:    "ses_any",
		Owner: "user-1",
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_any").Return(session, nil)
	s.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).Return(
		&types.Interaction{ID: "int-any", SessionID: "ses_any"}, nil,
	)
	s.store.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()

	s.server.processAnyPendingPrompt(context.Background(), "ses_any")
}

func (s *WebSocketSyncSuite) TestProcessAnyPendingPrompt_SendFails_MarkedFailed() {
	prompt := &types.PromptHistoryEntry{
		ID:        "prompt-anyfail",
		SessionID: "ses_anyfail",
		Content:   "fail",
	}
	s.store.EXPECT().GetAnyPendingPrompt(gomock.Any(), "ses_anyfail").Return(prompt, nil)
	s.store.EXPECT().MarkPromptAsSent(gomock.Any(), "prompt-anyfail").Return(nil)

	// GetSession fails → sendQueuedPromptToSession fails
	s.store.EXPECT().GetSession(gomock.Any(), "ses_anyfail").Return(nil, fmt.Errorf("db error"))
	s.store.EXPECT().MarkPromptAsFailed(gomock.Any(), "prompt-anyfail").Return(nil)

	s.server.processAnyPendingPrompt(context.Background(), "ses_anyfail")
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

	// finalizeCommentResponse tries to process next comment
	review := &types.SpecTaskDesignReview{
		ID:         "review-fin",
		SpecTaskID: "spec-fin",
	}
	s.store.EXPECT().GetSpecTaskDesignReview(gomock.Any(), "review-fin").Return(review, nil)

	specTask := &types.SpecTask{
		ID:                "spec-fin",
		PlanningSessionID: "ses_planning",
	}
	s.store.EXPECT().GetSpecTask(gomock.Any(), "spec-fin").Return(specTask, nil)

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
	s.server.sessionToWaitingInteraction["ses_cache"] = "int-cache"

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
	s.server.sessionToWaitingInteraction["ses_clear"] = "int-clear"

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
	s.store.EXPECT().GetSession(gomock.Any(), "ses_clear").Return(session, nil).Times(2)

	waitingInteraction := &types.Interaction{
		ID:              "int-clear",
		SessionID:       "ses_clear",
		State:           types.InteractionStateWaiting,
		ResponseMessage: "final content",
	}
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{waitingInteraction}, int64(1), nil,
	).Times(2)
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
	s.server.sessionToWaitingInteraction["ses_throttle"] = "int-throttle"

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
	s.server.sessionToWaitingInteraction["ses_flush"] = "int-flush"

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
	s.store.EXPECT().GetSession(gomock.Any(), "ses_flush").Return(session, nil).Times(2)
	s.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(
		[]*types.Interaction{existingInteraction}, int64(1), nil,
	).Times(2)
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
	s.server.sessionToWaitingInteraction["ses_multi"] = "int-multi"

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

// TestStreamingPatch_PreviousContentTracked verifies that the streaming context
// tracks previousContent for patch computation and updates it after each publish.
func (s *WebSocketSyncSuite) TestStreamingPatch_PreviousContentTracked() {
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
		},
	}

	// Send first token — creates streaming context with previousContent=""
	err := s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	// Verify previousContent is updated after publish
	sctx := s.server.streamingContexts[helixSessionID]
	sctx.mu.Lock()
	s.Equal("Hello", sctx.previousContent, "previousContent should be updated after first publish")
	s.Equal("Hello", sctx.interaction.ResponseMessage)
	sctx.mu.Unlock()

	// Expire throttles for second token
	sctx.mu.Lock()
	sctx.lastPublish = time.Now().Add(-100 * time.Millisecond)
	sctx.lastDBWrite = time.Now().Add(-300 * time.Millisecond)
	sctx.mu.Unlock()

	// Second token: append
	syncMsg.Data["content"] = "Hello world"
	err = s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	sctx.mu.Lock()
	s.Equal("Hello world", sctx.previousContent, "previousContent should track latest published content")
	s.Equal("Hello world", sctx.interaction.ResponseMessage)
	sctx.mu.Unlock()

	// Third token with backwards edit: tool call status change
	sctx.mu.Lock()
	sctx.lastPublish = time.Now().Add(-100 * time.Millisecond)
	sctx.lastDBWrite = time.Now().Add(-300 * time.Millisecond)
	sctx.mu.Unlock()

	syncMsg.Data["content"] = "Hello earth"
	err = s.server.handleMessageAdded("agent-1", syncMsg)
	s.NoError(err)

	sctx.mu.Lock()
	s.Equal("Hello earth", sctx.previousContent)
	// Verify patch computation would have produced the right delta
	offset, patch, totalLen := computePatch("Hello world", "Hello earth")
	s.Equal(6, offset)            // "Hello " is common prefix
	s.Equal("earth", patch)       // Changed portion
	s.Equal(11, totalLen)
	sctx.mu.Unlock()
}
