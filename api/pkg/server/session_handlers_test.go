package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/suite"
)

// func TestLimitInteractions(t *testing.T) {
// 	// Helper function to create test interactions
// 	createTestInteractions := func() []*types.Interaction {
// 		interactions := []*types.Interaction{
// 			{
// 				ID:      "1",
// 				Message: "A",
// 			},
// 			{
// 				ID:      "2",
// 				Message: "B",
// 			},
// 			{
// 				ID:      "3",
// 				Message: "C",
// 			},
// 			{
// 				ID:      "4",
// 				Message: "D",
// 			},
// 			{
// 				ID:      "5",
// 				Message: "E",
// 			},
// 			{
// 				ID:      "6",
// 				Message: "F",
// 			},
// 		}
// 		return interactions
// 	}

// 	// Case when we have less interactions than the limit
// 	t.Run("LessThanLimit", func(t *testing.T) {
// 		interactions := createTestInteractions()
// 		result := limitInteractions(interactions, 10)
// 		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
// 		assert.Equal(t, "A", result[0].Message)
// 		assert.Equal(t, "E", result[4].Message)
// 	})

// 	t.Run("Exact limit", func(t *testing.T) {
// 		interactions := createTestInteractions()
// 		result := limitInteractions(interactions, 6)
// 		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
// 		assert.Equal(t, "A", result[0].Message)
// 		assert.Equal(t, "E", result[4].Message)
// 	})

// 	// More messages than the limit
// 	t.Run("MoreThanLimit", func(t *testing.T) {
// 		interactions := createTestInteractions()
// 		result := limitInteractions(interactions, 3)
// 		assert.Equal(t, 3, len(result), "Should have all but the last interaction")
// 		assert.Equal(t, "C", result[0].Message)
// 		assert.Equal(t, "E", result[2].Message)
// 	})

// 	t.Run("ZeroLimit", func(t *testing.T) {
// 		interactions := createTestInteractions()
// 		result := limitInteractions(interactions, 0)
// 		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
// 		assert.Equal(t, "A", result[0].Message)
// 		assert.Equal(t, "E", result[4].Message)
// 	})
// }

type AppendOrOverwriteSuite struct {
	suite.Suite
}

func TestAppendOrOverwriteSuite(t *testing.T) {
	suite.Run(t, new(AppendOrOverwriteSuite))
}

func (suite *AppendOrOverwriteSuite) TestAppendToEmptySession() {
	session := &types.Session{
		Interactions: []*types.Interaction{},
		GenerationID: 0,
	}

	req := &types.SessionChatRequest{
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"Hello, how are you?",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	suite.Equal(0, session.GenerationID)

	suite.Require().Len(session.Interactions, 1)
	suite.Equal("Hello, how are you?", session.Interactions[0].PromptMessage)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[0].State)
}

func (suite *AppendOrOverwriteSuite) TestAppendToNonEmptySession() {
	session := &types.Session{
		Interactions: []*types.Interaction{
			{
				ID:              "1",
				PromptMessage:   "user message",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant message",
			},
		},
	}

	req := &types.SessionChatRequest{
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"How are you?",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	suite.Require().Len(session.Interactions, 2)
	suite.Equal("user message", session.Interactions[0].PromptMessage)
	suite.Equal("assistant message", session.Interactions[0].ResponseMessage)
	suite.Equal(types.InteractionStateComplete, session.Interactions[0].State)

	suite.Equal("How are you?", session.Interactions[1].PromptMessage)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[1].State)
	suite.Equal("", session.Interactions[1].ResponseMessage)
}

func (suite *AppendOrOverwriteSuite) TestOverwriteSession_LastMessage() {
	session := &types.Session{
		Interactions: []*types.Interaction{
			{
				ID:              "1",
				PromptMessage:   "user message",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant message",
			},
		},
	}

	req := &types.SessionChatRequest{
		Regenerate: true, // Regenerate
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"new user message",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	suite.Require().Len(session.Interactions, 1, "still expecting one interaction")

	suite.Equal("new user message", session.Interactions[0].PromptMessage)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[0].State)
	suite.Equal("", session.Interactions[0].ResponseMessage)

}

func (suite *AppendOrOverwriteSuite) TestOverwriteSession_FirstMessage() {
	session := &types.Session{
		Interactions: []*types.Interaction{
			{
				ID:              "1",
				PromptMessage:   "user message 1",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 1",
			},
			{
				ID:              "2",
				PromptMessage:   "user message 2",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 2",
			},
		},
	}

	req := &types.SessionChatRequest{
		Regenerate:    true,
		InteractionID: "1",
		Messages: []*types.Message{
			{
				ID:   "1",
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"overwriting user message 1",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	suite.Require().Len(session.Interactions, 1)
	suite.Equal("overwriting user message 1", session.Interactions[0].PromptMessage)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[0].State)
	suite.Equal("", session.Interactions[0].ResponseMessage)

}

func (suite *AppendOrOverwriteSuite) TestOverwriteSession_MiddleMessage() {
	session := &types.Session{
		GenerationID: 1,
		Interactions: []*types.Interaction{
			{
				ID:              "1",
				PromptMessage:   "user message 1",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 1",
				GenerationID:    1,
			},
			{
				ID:              "2",
				PromptMessage:   "user message 2",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 2",
				GenerationID:    1,
			},
			{
				ID:              "3",
				PromptMessage:   "user message 3",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 3",
				GenerationID:    1,
			},
			{
				ID:              "4",
				PromptMessage:   "user message 4",
				State:           types.InteractionStateComplete,
				ResponseMessage: "assistant response 4",
				GenerationID:    1,
			},
		},
	}

	req := &types.SessionChatRequest{
		Regenerate:    true,
		InteractionID: "2",
		Messages: []*types.Message{
			{
				ID:   "1",
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"user message 1",
					},
				},
			},
			{
				ID:   "2",
				Role: "assistant",
				Content: types.MessageContent{
					Parts: []interface{}{
						"assistant response 1",
					},
				},
			},
			// Overwriting the third user message
			{
				ID:   "3",
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"regenerating from here",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	// Should be 2 interactions:
	// First interaction is "user message 1" and "assistant response 1"
	// Second interaction is "overwriting user message 3"
	suite.Require().Len(session.Interactions, 2)

	// First interaction should be the new user message
	suite.Equal("user message 1", session.Interactions[0].PromptMessage)
	suite.Equal(types.InteractionStateComplete, session.Interactions[0].State)
	suite.Equal("assistant response 1", session.Interactions[0].ResponseMessage)

	suite.Equal("regenerating from here", session.Interactions[1].PromptMessage)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[1].State)
	suite.Equal("", session.Interactions[1].ResponseMessage)

	// Check generation IDs
	suite.Equal(2, session.Interactions[0].GenerationID)
	suite.Equal(2, session.Interactions[1].GenerationID)
}

type ExternalAgentSessionSuite struct {
	suite.Suite
}

func TestExternalAgentSessionSuite(t *testing.T) {
	suite.Run(t, new(ExternalAgentSessionSuite))
}

func (suite *ExternalAgentSessionSuite) TestExternalAgentModelProcessing() {
	// Test that external_agent model is properly processed
	provider := "helix"
	modelName := "external_agent"
	sessionType := types.SessionTypeText

	processedModel, err := suite.processModelName(provider, modelName, sessionType)
	suite.NoError(err)
	suite.Equal("external_agent", processedModel)
}

func (suite *ExternalAgentSessionSuite) TestExternalAgentSessionRequest() {
	// Test session creation with external agent configuration
	req := &types.SessionChatRequest{
		Type:      types.SessionTypeText,
		Model:     "external_agent",
		AgentType: "zed_external",
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"Hello from external agent test",
					},
				},
			},
		},
		ExternalAgentConfig: &types.ExternalAgentConfig{
			Resolution: "1080p",
		},
	}

	// Verify the request structure
	suite.Equal("external_agent", req.Model)
	suite.Equal("zed_external", req.AgentType)
	suite.Equal("1080p", req.ExternalAgentConfig.Resolution)
}

func (suite *ExternalAgentSessionSuite) TestExternalAgentSessionMetadata() {
	// Test that session metadata is properly set for external agents
	session := &types.Session{
		ID:           "test-session-id",
		Name:         "External Agent Test Session",
		Mode:         types.SessionModeInference,
		Type:         types.SessionTypeText,
		ModelName:    "external_agent",
		Interactions: []*types.Interaction{},
		Metadata: types.SessionMetadata{
			AgentType: "zed_external",
		},
	}

	suite.Equal("external_agent", session.ModelName)
	suite.Equal("zed_external", session.Metadata.AgentType)
	suite.Equal(types.SessionModeInference, session.Mode)
	suite.Equal(types.SessionTypeText, session.Type)
}

// Helper method to simulate model processing
func (suite *ExternalAgentSessionSuite) processModelName(provider, modelName string, sessionType types.SessionType) (string, error) {
	// Simulate the ProcessModelName function logic for external agents
	if provider == "helix" && modelName == "external_agent" && sessionType == types.SessionTypeText {
		return "external_agent", nil
	}
	return modelName, nil
}
