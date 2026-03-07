package openai

import (
	"context"
	"os"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGeminiCalculatorToolCall_Integration tests a full tool-call round trip
// against the real Gemini API: user asks a math question → model calls
// calculator tool → we return the result → model responds with the answer.
// This validates that thought signatures are correctly cached and echoed back.
func TestGeminiCalculatorToolCall_Integration(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set, skipping integration test")
	}

	client := New(apiKey, "https://generativelanguage.googleapis.com/v1beta/openai", false)
	ctx := context.Background()
	model := "models/gemini-3.1-flash-lite-preview"

	tools := []openai.Tool{
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

	// Step 1: Ask a math question — the model should call the calculator tool
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "What is 1337 * 42? Use the calculator tool to compute this.",
		},
	}

	resp1, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:      model,
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
	assert.NotEmpty(t, toolCall.ID, "tool call should have an ID")
	t.Logf("Step 1: tool call id=%s name=%s args=%s", toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments)

	// Verify thought signature was cached
	sig, hasSig := client.thoughtSigCache.Get(toolCall.ID)
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
		Model:    model,
		Messages: messages,
		Tools:    tools,
	})
	require.NoError(t, err, "Step 2: should succeed — thought signatures must be correctly echoed back")
	require.NotEmpty(t, resp2.Choices, "Step 2: should have choices")

	finalContent := resp2.Choices[0].Message.Content
	t.Logf("Step 2: final response: %s", finalContent)
	assert.NotEmpty(t, finalContent, "Step 2: expected a text response after tool result")
	assert.Contains(t, finalContent, "56154", "response should mention the calculation result")
}
