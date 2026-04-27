package openai

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const geminiTestModel = "models/gemini-3.1-flash-lite-preview"

func calculatorTools() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "calculator",
				Description: "Perform a mathematical calculation. Use this for any arithmetic.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"expression": map[string]interface{}{
							"type":        "string",
							"description": "The mathematical expression to evaluate, e.g. '2 + 2' or '144 / 12'",
						},
					},
					"required": []string{"expression"},
				},
			},
		},
	}
}

func skipWithoutGeminiKey(t *testing.T) string {
	t.Helper()
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set, skipping integration test")
	}
	return apiKey
}

// TestGeminiCalculatorToolCall_Integration tests a full tool-call round trip
// against the real Gemini API: user asks a math question → model calls
// calculator tool → we return the result → model responds with the answer.
// This validates that thought signatures are correctly cached and echoed back.
func TestGeminiCalculatorToolCall_Integration(t *testing.T) {
	apiKey := skipWithoutGeminiKey(t)

	client := New(apiKey, "https://generativelanguage.googleapis.com/v1beta/openai", false)
	ctx := context.Background()
	tools := calculatorTools()

	// Step 1: Ask a math question — the model should call the calculator tool
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "What is 1337 * 42? Use the calculator tool to compute this.",
		},
	}

	resp1, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:      geminiTestModel,
		Messages:   messages,
		Tools:      tools,
		ToolChoice: "required",
	})
	require.NoError(t, err, "Step 1: CreateChatCompletion should succeed")
	require.NotEmpty(t, resp1.Choices, "Step 1: should have choices")

	assistantMsg := resp1.Choices[0].Message
	require.NotEmpty(t, assistantMsg.ToolCalls, "Step 1: model should return a tool call")

	toolCall := assistantMsg.ToolCalls[0]
	assert.Equal(t, "calculator", toolCall.Function.Name)
	assert.NotEmpty(t, toolCall.ID, "tool call should have a generated ID")
	t.Logf("Step 1: tool call id=%s name=%s args=%s", toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments)

	// Verify thought signature was cached
	sig, hasSig := globalThoughtSigCache.Get(toolCall.ID)
	t.Logf("Step 1: thought signature cached=%v len=%d", hasSig, len(sig))

	// Step 2: Send the tool result back — this is where thought signatures matter.
	// If the signature isn't echoed back correctly, Gemini will reject the request.
	messages = append(messages, assistantMsg)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		ToolCallID: toolCall.ID,
		Content:    `{"result": 56154}`,
	})

	resp2, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    geminiTestModel,
		Messages: messages,
		Tools:    tools,
	})
	require.NoError(t, err, "Step 2: should succeed — thought signatures must be correctly echoed back")
	require.NotEmpty(t, resp2.Choices, "Step 2: should have choices")

	finalContent := resp2.Choices[0].Message.Content
	t.Logf("Step 2: final response: %s", finalContent)
	assert.NotEmpty(t, finalContent, "Step 2: expected a text response after tool result")
	assert.Contains(t, strings.ReplaceAll(finalContent, ",", ""), "56154", "response should mention the calculation result")
}

// TestGeminiCalculatorToolCallStream_Integration tests a full streaming
// tool-call round trip: user asks → model streams a tool call → we return
// the result → model streams the final answer.
func TestGeminiCalculatorToolCallStream_Integration(t *testing.T) {
	apiKey := skipWithoutGeminiKey(t)

	client := New(apiKey, "https://generativelanguage.googleapis.com/v1beta/openai", false)
	ctx := context.Background()
	tools := calculatorTools()

	// Step 1: Stream request that should produce a tool call
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "What is 1337 * 42? You must use the calculator tool.",
		},
	}

	stream1, err := client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:      geminiTestModel,
		Messages:   messages,
		Tools:      tools,
		ToolChoice: "required",
	})
	require.NoError(t, err, "Step 1: CreateChatCompletionStream should succeed")

	// Consume the stream and accumulate tool calls
	var toolCalls []openai.ToolCall
	for {
		chunk, err := stream1.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err, "Step 1: stream recv should not error")

		for _, choice := range chunk.Choices {
			for _, tc := range choice.Delta.ToolCalls {
				// Streaming may split tool calls across chunks; accumulate by index
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				for len(toolCalls) <= idx {
					toolCalls = append(toolCalls, openai.ToolCall{Type: openai.ToolTypeFunction})
				}
				if tc.ID != "" {
					toolCalls[idx].ID = tc.ID
				}
				if tc.Function.Name != "" {
					toolCalls[idx].Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					toolCalls[idx].Function.Arguments += tc.Function.Arguments
				}
			}
		}
	}
	stream1.Close()

	require.NotEmpty(t, toolCalls, "Step 1: should have received tool calls from stream")
	tc := toolCalls[0]
	assert.Equal(t, "calculator", tc.Function.Name)
	assert.NotEmpty(t, tc.ID, "streamed tool call should have an ID")
	t.Logf("Step 1 (stream): tool call id=%s name=%s args=%s", tc.ID, tc.Function.Name, tc.Function.Arguments)

	// Verify thought signature was cached during streaming
	sig, hasSig := globalThoughtSigCache.Get(tc.ID)
	t.Logf("Step 1 (stream): thought signature cached=%v len=%d", hasSig, len(sig))

	// Step 2: Send the tool result back via streaming and collect the final text
	assistantMsg := openai.ChatCompletionMessage{
		Role:      openai.ChatMessageRoleAssistant,
		ToolCalls: toolCalls,
	}
	messages = append(messages, assistantMsg)
	messages = append(messages, openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		ToolCallID: tc.ID,
		Content:    `{"result": 56154}`,
	})

	stream2, err := client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:    geminiTestModel,
		Messages: messages,
		Tools:    tools,
	})
	require.NoError(t, err, "Step 2: streaming with tool result should succeed")

	var contentBuilder strings.Builder
	for {
		chunk, err := stream2.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err, "Step 2: stream recv should not error")

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				contentBuilder.WriteString(choice.Delta.Content)
			}
		}
	}
	stream2.Close()

	finalContent := contentBuilder.String()
	t.Logf("Step 2 (stream): final response: %s", finalContent)
	assert.NotEmpty(t, finalContent, "Step 2: expected streamed text after tool result")
	assert.Contains(t, strings.ReplaceAll(finalContent, ",", ""), "56154", "streamed response should mention the calculation result")
}

// TestGeminiSimpleStream_Integration tests basic streaming without tool calls
// to ensure the streaming adapter works for plain text responses.
func TestGeminiSimpleStream_Integration(t *testing.T) {
	apiKey := skipWithoutGeminiKey(t)

	client := New(apiKey, "https://generativelanguage.googleapis.com/v1beta/openai", false)
	ctx := context.Background()

	stream, err := client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model: geminiTestModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "Say exactly: hello world"},
		},
	})
	require.NoError(t, err)

	var contentBuilder strings.Builder
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		for _, choice := range chunk.Choices {
			contentBuilder.WriteString(choice.Delta.Content)
		}
	}
	stream.Close()

	content := strings.ToLower(contentBuilder.String())
	t.Logf("Streamed response: %s", content)
	assert.Contains(t, content, "hello world")
}
