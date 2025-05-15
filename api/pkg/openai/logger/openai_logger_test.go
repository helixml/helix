package logger

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tiktoken-go/tokenizer"
)

func Test_computeTokenUsage_SingleMessage(t *testing.T) {
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	require.NoError(t, err)
	mw := &LoggingMiddleware{
		defaultCodec: enc,
	}

	// Test case 1: OpenAI model
	req := &openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello, world!"},
		},
	}
	resp := &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{Content: "Hello, world!"},
			},
		},
	}

	promptTokens, completionTokens, totalTokens := mw.computeTokenUsage(req, resp)
	assert.Equal(t, 4, promptTokens)
	assert.Equal(t, 4, completionTokens)
	assert.Equal(t, 8, totalTokens)
}

func Test_computeTokenUsage_MultipleMessage(t *testing.T) {
	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	require.NoError(t, err)
	mw := &LoggingMiddleware{
		defaultCodec: enc,
	}

	// Test case 1: OpenAI model
	req := &openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: "Hello, world!"},
			{Role: "assistant", Content: "Hello"},
			{Role: "user", Content: "How are you?"},
		},
	}
	resp := &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{Content: "OK!"},
			},
		},
	}

	promptTokens, completionTokens, totalTokens := mw.computeTokenUsage(req, resp)
	assert.Equal(t, 9, promptTokens)
	assert.Equal(t, 2, completionTokens)
	assert.Equal(t, 11, totalTokens)
}
