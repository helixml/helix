package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// SessionMCPBackendSuite tests the Session MCP backend
type SessionMCPBackendSuite struct {
	suite.Suite
	ctx       context.Context
	ctrl      *gomock.Controller
	mockStore *store.MockStore
	backend   *SessionMCPBackend
}

func TestSessionMCPBackendSuite(t *testing.T) {
	suite.Run(t, new(SessionMCPBackendSuite))
}

func (suite *SessionMCPBackendSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.ctrl = gomock.NewController(suite.T())
	suite.mockStore = store.NewMockStore(suite.ctrl)
	suite.backend = NewSessionMCPBackend(suite.mockStore)
}

func (suite *SessionMCPBackendSuite) TearDownTest() {
	suite.ctrl.Finish()
}

// =============================================================================
// Test Helpers
// =============================================================================

func (suite *SessionMCPBackendSuite) testSession() *types.Session {
	return &types.Session{
		ID:      "session-123",
		Name:    "Test Session",
		Created: time.Now().Add(-1 * time.Hour),
		Updated: time.Now(),
		Metadata: types.SessionMetadata{
			TitleHistory: []*types.TitleHistoryEntry{
				{
					Title:         "Initial Title",
					ChangedAt:     time.Now().Add(-30 * time.Minute),
					Turn:          1,
					InteractionID: "int-1",
				},
				{
					Title:         "Test Session",
					ChangedAt:     time.Now().Add(-10 * time.Minute),
					Turn:          3,
					InteractionID: "int-3",
				},
			},
		},
	}
}

func (suite *SessionMCPBackendSuite) testInteractions() []*types.Interaction {
	return []*types.Interaction{
		{
			ID:              "int-1",
			PromptMessage:   "What is the capital of France?",
			ResponseMessage: "The capital of France is Paris.",
			Summary:         "Asked about capital of France",
			Created:         time.Now().Add(-45 * time.Minute),
		},
		{
			ID:              "int-2",
			PromptMessage:   "What is the population?",
			ResponseMessage: "The population of Paris is approximately 2.1 million.",
			Summary:         "Asked about population of Paris",
			Created:         time.Now().Add(-30 * time.Minute),
		},
		{
			ID:              "int-3",
			PromptMessage:   "Tell me about the Eiffel Tower",
			ResponseMessage: "The Eiffel Tower is a wrought-iron lattice tower...",
			Summary:         "Discussed Eiffel Tower",
			Created:         time.Now().Add(-15 * time.Minute),
		},
	}
}

func (suite *SessionMCPBackendSuite) ctxWithSessionID(sessionID string) context.Context {
	return context.WithValue(suite.ctx, "session_id", sessionID)
}

// =============================================================================
// Tests for handleCurrentSession
// =============================================================================

func (suite *SessionMCPBackendSuite) TestHandleCurrentSession_Success() {
	session := suite.testSession()
	ctx := suite.ctxWithSessionID("session-123")

	suite.mockStore.EXPECT().
		GetSession(gomock.Any(), "session-123").
		Return(session, nil)

	suite.mockStore.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return([]*types.Interaction{}, int64(3), nil)

	result, err := suite.backend.handleCurrentSession(ctx, mcp.CallToolRequest{})
	suite.NoError(err)
	suite.NotNil(result)

	// Parse the result
	var data map[string]interface{}
	content := result.Content[0].(mcp.TextContent)
	err = json.Unmarshal([]byte(content.Text), &data)
	suite.NoError(err)

	suite.Equal("session-123", data["session_id"])
	suite.Equal("Test Session", data["name"])
	suite.Equal(float64(3), data["total_turns"])
	suite.Equal(float64(2), data["title_changes"])
}

func (suite *SessionMCPBackendSuite) TestHandleCurrentSession_MissingSessionID() {
	result, err := suite.backend.handleCurrentSession(suite.ctx, mcp.CallToolRequest{})
	suite.NoError(err) // MCP tools return error in result, not as Go error
	suite.NotNil(result)
	suite.True(result.IsError)

	content := result.Content[0].(mcp.TextContent)
	suite.Contains(content.Text, "session_id is required")
}

// =============================================================================
// Tests for handleSessionTOC
// =============================================================================

func (suite *SessionMCPBackendSuite) TestHandleSessionTOC_Success() {
	session := suite.testSession()
	interactions := suite.testInteractions()
	ctx := suite.ctxWithSessionID("session-123")

	suite.mockStore.EXPECT().
		GetSession(gomock.Any(), "session-123").
		Return(session, nil)

	suite.mockStore.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return(interactions, int64(3), nil)

	result, err := suite.backend.handleSessionTOC(ctx, mcp.CallToolRequest{})
	suite.NoError(err)
	suite.NotNil(result)

	// Parse the result
	var data map[string]interface{}
	content := result.Content[0].(mcp.TextContent)
	err = json.Unmarshal([]byte(content.Text), &data)
	suite.NoError(err)

	suite.Equal("session-123", data["session_id"])
	suite.Equal("Test Session", data["session_name"])
	suite.Equal(float64(3), data["total_turns"])

	entries := data["entries"].([]interface{})
	suite.Len(entries, 3)

	entry1 := entries[0].(map[string]interface{})
	suite.Equal(float64(1), entry1["turn"])
	suite.Equal("int-1", entry1["id"])
	suite.Equal("Asked about capital of France", entry1["summary"])
}

func (suite *SessionMCPBackendSuite) TestHandleSessionTOC_FallbackSummary() {
	session := suite.testSession()
	ctx := suite.ctxWithSessionID("session-123")

	// Interaction without summary
	interactions := []*types.Interaction{
		{
			ID:            "int-1",
			PromptMessage: "This is a long prompt message that should be truncated if it exceeds 80 characters in length",
			Summary:       "", // Empty summary
		},
	}

	suite.mockStore.EXPECT().
		GetSession(gomock.Any(), "session-123").
		Return(session, nil)

	suite.mockStore.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return(interactions, int64(1), nil)

	result, err := suite.backend.handleSessionTOC(ctx, mcp.CallToolRequest{})
	suite.NoError(err)

	var data map[string]interface{}
	content := result.Content[0].(mcp.TextContent)
	err = json.Unmarshal([]byte(content.Text), &data)
	suite.NoError(err)

	entries := data["entries"].([]interface{})
	entry1 := entries[0].(map[string]interface{})
	summary := entry1["summary"].(string)
	suite.Contains(summary, "...")
	suite.LessOrEqual(len(summary), 84) // 80 chars + "..."
}

// =============================================================================
// Tests for handleGetTurn
// =============================================================================

func (suite *SessionMCPBackendSuite) TestHandleGetTurn_Success() {
	interactions := suite.testInteractions()
	ctx := suite.ctxWithSessionID("session-123")

	suite.mockStore.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return(interactions, int64(3), nil)

	// Create request with turn number
	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"turn": float64(2),
	}

	result, err := suite.backend.handleGetTurn(ctx, request)
	suite.NoError(err)
	suite.NotNil(result)

	var data map[string]interface{}
	content := result.Content[0].(mcp.TextContent)
	err = json.Unmarshal([]byte(content.Text), &data)
	suite.NoError(err)

	suite.Equal(float64(2), data["turn"])
	suite.Equal("int-2", data["id"])
	suite.Equal("What is the population?", data["prompt"])
	suite.Contains(data["response"].(string), "2.1 million")
}

func (suite *SessionMCPBackendSuite) TestHandleGetTurn_TurnNotFound() {
	interactions := suite.testInteractions()
	ctx := suite.ctxWithSessionID("session-123")

	suite.mockStore.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return(interactions, int64(3), nil)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"turn": float64(10), // Turn that doesn't exist
	}

	result, err := suite.backend.handleGetTurn(ctx, request)
	suite.NoError(err)
	suite.True(result.IsError)

	content := result.Content[0].(mcp.TextContent)
	suite.Contains(content.Text, "turn 10 not found")
	suite.Contains(content.Text, "has 3 turns")
}

func (suite *SessionMCPBackendSuite) TestHandleGetTurn_InvalidTurn() {
	ctx := suite.ctxWithSessionID("session-123")

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"turn": float64(0), // Invalid turn number
	}

	result, err := suite.backend.handleGetTurn(ctx, request)
	suite.NoError(err)
	suite.True(result.IsError)

	content := result.Content[0].(mcp.TextContent)
	suite.Contains(content.Text, "turn number is required")
}

// =============================================================================
// Tests for handleTitleHistory
// =============================================================================

func (suite *SessionMCPBackendSuite) TestHandleTitleHistory_Success() {
	session := suite.testSession()
	ctx := suite.ctxWithSessionID("session-123")

	suite.mockStore.EXPECT().
		GetSession(gomock.Any(), "session-123").
		Return(session, nil)

	result, err := suite.backend.handleTitleHistory(ctx, mcp.CallToolRequest{})
	suite.NoError(err)
	suite.NotNil(result)

	var data map[string]interface{}
	content := result.Content[0].(mcp.TextContent)
	err = json.Unmarshal([]byte(content.Text), &data)
	suite.NoError(err)

	suite.Equal("session-123", data["session_id"])
	suite.Equal("Test Session", data["current_title"])

	history := data["history"].([]interface{})
	suite.Len(history, 2)
}

func (suite *SessionMCPBackendSuite) TestHandleTitleHistory_EmptyHistory() {
	session := &types.Session{
		ID:       "session-123",
		Name:     "New Session",
		Metadata: types.SessionMetadata{},
	}
	ctx := suite.ctxWithSessionID("session-123")

	suite.mockStore.EXPECT().
		GetSession(gomock.Any(), "session-123").
		Return(session, nil)

	result, err := suite.backend.handleTitleHistory(ctx, mcp.CallToolRequest{})
	suite.NoError(err)

	var data map[string]interface{}
	content := result.Content[0].(mcp.TextContent)
	err = json.Unmarshal([]byte(content.Text), &data)
	suite.NoError(err)

	suite.Nil(data["history"]) // Empty history
}

// =============================================================================
// Tests for handleSearchSession
// =============================================================================

func (suite *SessionMCPBackendSuite) TestHandleSearchSession_Success() {
	interactions := suite.testInteractions()
	ctx := suite.ctxWithSessionID("session-123")

	suite.mockStore.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return(interactions, int64(3), nil)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"query": "Paris",
	}

	result, err := suite.backend.handleSearchSession(ctx, request)
	suite.NoError(err)
	suite.NotNil(result)

	var data map[string]interface{}
	content := result.Content[0].(mcp.TextContent)
	err = json.Unmarshal([]byte(content.Text), &data)
	suite.NoError(err)

	suite.Equal("session-123", data["session_id"])
	suite.Equal("Paris", data["query"])
	suite.Equal(float64(2), data["total"]) // 2 matches (int-1 and int-2)

	matches := data["matches"].([]interface{})
	suite.Len(matches, 2)
}

func (suite *SessionMCPBackendSuite) TestHandleSearchSession_CaseInsensitive() {
	interactions := suite.testInteractions()
	ctx := suite.ctxWithSessionID("session-123")

	suite.mockStore.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return(interactions, int64(3), nil)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"query": "EIFFEL", // Uppercase search
	}

	result, err := suite.backend.handleSearchSession(ctx, request)
	suite.NoError(err)

	var data map[string]interface{}
	content := result.Content[0].(mcp.TextContent)
	err = json.Unmarshal([]byte(content.Text), &data)
	suite.NoError(err)

	suite.Equal(float64(1), data["total"])
}

func (suite *SessionMCPBackendSuite) TestHandleSearchSession_NoMatches() {
	interactions := suite.testInteractions()
	ctx := suite.ctxWithSessionID("session-123")

	suite.mockStore.EXPECT().
		ListInteractions(gomock.Any(), gomock.Any()).
		Return(interactions, int64(3), nil)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"query": "nonexistent",
	}

	result, err := suite.backend.handleSearchSession(ctx, request)
	suite.NoError(err)

	var data map[string]interface{}
	content := result.Content[0].(mcp.TextContent)
	err = json.Unmarshal([]byte(content.Text), &data)
	suite.NoError(err)

	suite.Equal(float64(0), data["total"])
	suite.Nil(data["matches"]) // No matches
}

func (suite *SessionMCPBackendSuite) TestHandleSearchSession_MissingQuery() {
	ctx := suite.ctxWithSessionID("session-123")

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	result, err := suite.backend.handleSearchSession(ctx, request)
	suite.NoError(err)
	suite.True(result.IsError)

	content := result.Content[0].(mcp.TextContent)
	suite.Contains(content.Text, "query is required")
}

// =============================================================================
// Tests for getSessionID helper
// =============================================================================

func (suite *SessionMCPBackendSuite) TestGetSessionID_FromRequest() {
	ctx := suite.ctxWithSessionID("session-from-context")
	sessionID := suite.backend.getSessionID(ctx, "session-from-request")
	suite.Equal("session-from-request", sessionID)
}

func (suite *SessionMCPBackendSuite) TestGetSessionID_FromContext() {
	ctx := suite.ctxWithSessionID("session-from-context")
	sessionID := suite.backend.getSessionID(ctx, "")
	suite.Equal("session-from-context", sessionID)
}

func (suite *SessionMCPBackendSuite) TestGetSessionID_Empty() {
	sessionID := suite.backend.getSessionID(suite.ctx, "")
	suite.Equal("", sessionID)
}

// =============================================================================
// Tests for containsIgnoreCase helper
// =============================================================================

func (suite *SessionMCPBackendSuite) TestContainsIgnoreCase() {
	suite.True(containsIgnoreCase("Hello World", "hello"))
	suite.True(containsIgnoreCase("Hello World", "WORLD"))
	suite.True(containsIgnoreCase("Hello World", "lo Wo"))
	suite.False(containsIgnoreCase("Hello World", "foo"))
	suite.True(containsIgnoreCase("", ""))
	suite.True(containsIgnoreCase("Hello", "")) // Empty string is contained in any string
}
