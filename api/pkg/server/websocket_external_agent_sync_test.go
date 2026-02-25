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
