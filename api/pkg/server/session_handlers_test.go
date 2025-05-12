package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestLimitInteractions(t *testing.T) {
	// Helper function to create test interactions
	createTestInteractions := func() []*types.Interaction {
		interactions := []*types.Interaction{
			{
				ID:      "1",
				Message: "A",
			},
			{
				ID:      "2",
				Message: "B",
			},
			{
				ID:      "3",
				Message: "C",
			},
			{
				ID:      "4",
				Message: "D",
			},
			{
				ID:      "5",
				Message: "E",
			},
			{
				ID:      "6",
				Message: "F",
			},
		}
		return interactions
	}

	// Case when we have less interactions than the limit
	t.Run("LessThanLimit", func(t *testing.T) {
		interactions := createTestInteractions()
		result := limitInteractions(interactions, 10)
		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
		assert.Equal(t, "A", result[0].Message)
		assert.Equal(t, "E", result[4].Message)
	})

	t.Run("Exact limit", func(t *testing.T) {
		interactions := createTestInteractions()
		result := limitInteractions(interactions, 6)
		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
		assert.Equal(t, "A", result[0].Message)
		assert.Equal(t, "E", result[4].Message)
	})

	// More messages than the limit
	t.Run("MoreThanLimit", func(t *testing.T) {
		interactions := createTestInteractions()
		result := limitInteractions(interactions, 3)
		assert.Equal(t, 3, len(result), "Should have all but the last interaction")
		assert.Equal(t, "C", result[0].Message)
		assert.Equal(t, "E", result[2].Message)
	})

	t.Run("ZeroLimit", func(t *testing.T) {
		interactions := createTestInteractions()
		result := limitInteractions(interactions, 0)
		assert.Equal(t, 5, len(result), "Should have all but the last interaction")
		assert.Equal(t, "A", result[0].Message)
		assert.Equal(t, "E", result[4].Message)
	})
}

type AppendOrOverwriteSuite struct {
	suite.Suite
}

func TestAppendOrOverwriteSuite(t *testing.T) {
	suite.Run(t, new(AppendOrOverwriteSuite))
}

func (suite *AppendOrOverwriteSuite) TestAppendToEmptySession() {
	session := &types.Session{
		Interactions: []*types.Interaction{},
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

	suite.Require().Len(session.Interactions, 2)
	suite.Equal("Hello, how are you?", session.Interactions[0].Message)
	suite.Equal(types.CreatorTypeUser, session.Interactions[0].Creator)
	suite.Equal(types.InteractionStateComplete, session.Interactions[0].State)
	suite.True(session.Interactions[0].Finished)

	suite.Equal("", session.Interactions[1].Message)
	suite.Equal(types.CreatorTypeAssistant, session.Interactions[1].Creator)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[1].State)
	suite.False(session.Interactions[1].Finished)
}

func (suite *AppendOrOverwriteSuite) TestAppendToNonEmptySession() {
	session := &types.Session{
		Interactions: []*types.Interaction{
			{
				ID:       "1",
				Message:  "Hello",
				Creator:  types.CreatorTypeUser,
				State:    types.InteractionStateComplete,
				Finished: true,
			},
			{
				ID:       "2",
				Message:  "Hi there!",
				Creator:  types.CreatorTypeAssistant,
				State:    types.InteractionStateComplete,
				Finished: true,
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

	suite.Require().Len(session.Interactions, 4)
	suite.Equal("Hello", session.Interactions[0].Message)
	suite.Equal("Hi there!", session.Interactions[1].Message)
	suite.Equal("How are you?", session.Interactions[2].Message)
	suite.Equal(types.CreatorTypeUser, session.Interactions[2].Creator)
	suite.Equal(types.InteractionStateComplete, session.Interactions[2].State)
	suite.True(session.Interactions[2].Finished)

	suite.Equal("", session.Interactions[3].Message)
	suite.Equal(types.CreatorTypeAssistant, session.Interactions[3].Creator)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[3].State)
	suite.False(session.Interactions[3].Finished)
}

func (suite *AppendOrOverwriteSuite) TestOverwriteSession_LastMessage() {
	session := &types.Session{
		Interactions: []*types.Interaction{
			{
				Message:  "Hello",
				Creator:  types.CreatorTypeUser,
				State:    types.InteractionStateComplete,
				Finished: true,
			},
			{
				Message:  "Hi there!",
				Creator:  types.CreatorTypeAssistant,
				State:    types.InteractionStateComplete,
				Finished: true,
			},
		},
	}

	req := &types.SessionChatRequest{
		Regenerate: true,
		Messages: []*types.Message{
			{
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"Hello! I am user",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	suite.Require().Len(session.Interactions, 2)

	suite.Equal("Hello! I am user", session.Interactions[0].Message)
	suite.Equal(types.CreatorTypeUser, session.Interactions[0].Creator)
	suite.Equal(types.InteractionStateComplete, session.Interactions[0].State)
	suite.True(session.Interactions[0].Finished)

	suite.Equal("", session.Interactions[1].Message)
	suite.Equal(types.CreatorTypeAssistant, session.Interactions[1].Creator)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[1].State)
	suite.False(session.Interactions[1].Finished)
}

func (suite *AppendOrOverwriteSuite) TestOverwriteSession_FirstMessage() {
	session := &types.Session{
		Interactions: []*types.Interaction{
			{
				ID:       "1",
				Message:  "Hello",
				Creator:  types.CreatorTypeUser,
				State:    types.InteractionStateComplete,
				Finished: true,
			},
			{
				ID:       "2",
				Message:  "Hi there!",
				Creator:  types.CreatorTypeAssistant,
				State:    types.InteractionStateComplete,
				Finished: true,
			},
			{
				ID:       "3",
				Message:  "How are you?",
				Creator:  types.CreatorTypeUser,
				State:    types.InteractionStateComplete,
				Finished: true,
			},
		},
	}

	req := &types.SessionChatRequest{
		Regenerate: true,
		Messages: []*types.Message{
			{
				ID:   "1",
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"Hi, I have a question",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	suite.Require().Len(session.Interactions, 2)
	suite.Equal("Hi, I have a question", session.Interactions[0].Message)
	suite.Equal(types.CreatorTypeUser, session.Interactions[0].Creator)
	suite.Equal(types.InteractionStateComplete, session.Interactions[0].State)
	suite.True(session.Interactions[0].Finished)

	suite.Equal("", session.Interactions[1].Message)
	suite.Equal(types.CreatorTypeAssistant, session.Interactions[1].Creator)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[1].State)
	suite.False(session.Interactions[1].Finished)
}

func (suite *AppendOrOverwriteSuite) TestOverwriteSession_MiddleMessage() {
	session := &types.Session{
		Interactions: []*types.Interaction{
			{
				ID:       "1",
				Message:  "Hello",
				Creator:  types.CreatorTypeUser,
				State:    types.InteractionStateComplete,
				Finished: true,
			},
			{
				ID:       "2",
				Message:  "Hi there!",
				Creator:  types.CreatorTypeAssistant,
				State:    types.InteractionStateComplete,
				Finished: true,
			},
			{
				ID:       "3",
				Message:  "How are you?",
				Creator:  types.CreatorTypeUser,
				State:    types.InteractionStateComplete,
				Finished: true,
			},
			{
				ID:       "4",
				Message:  "I'm good, thanks!",
				Creator:  types.CreatorTypeAssistant,
				State:    types.InteractionStateComplete,
				Finished: true,
			},
		},
	}

	req := &types.SessionChatRequest{
		Regenerate: true,
		Messages: []*types.Message{
			{
				ID:   "1",
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"Hello",
					},
				},
			},
			{
				ID:   "2",
				Role: "assistant",
				Content: types.MessageContent{
					Parts: []interface{}{
						"Hi there!",
					},
				},
			},
			// Overwriting the last user message
			{
				ID:   "3",
				Role: "user",
				Content: types.MessageContent{
					Parts: []interface{}{
						"How are you doing?",
					},
				},
			},
		},
	}

	session, err := appendOrOverwrite(session, req)
	suite.NoError(err)

	// Should be 4 interactions
	suite.Require().Len(session.Interactions, 4)

	// First interaction should be the new user message
	suite.Equal("Hello", session.Interactions[0].Message)
	suite.Equal(types.CreatorTypeUser, session.Interactions[0].Creator)
	suite.Equal(types.InteractionStateComplete, session.Interactions[0].State)
	suite.True(session.Interactions[0].Finished)

	// Last user interaction should be the new user message
	suite.Equal("How are you doing?", session.Interactions[2].Message)
	suite.Equal(types.CreatorTypeUser, session.Interactions[2].Creator)
	suite.Equal(types.InteractionStateComplete, session.Interactions[2].State)
	suite.True(session.Interactions[2].Finished)

	// Last interaction should be the assistant placeholder in "waiting" state
	suite.Equal("", session.Interactions[3].Message)
	suite.Equal(types.CreatorTypeAssistant, session.Interactions[3].Creator)
	suite.Equal(types.InteractionStateWaiting, session.Interactions[3].State)
	suite.False(session.Interactions[3].Finished)

}
