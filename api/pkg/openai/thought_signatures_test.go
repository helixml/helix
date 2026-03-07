package openai

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestThoughtSignatureCache_SetGet(t *testing.T) {
	cache := &thoughtSignatureCache{store: make(map[string][]byte)}

	cache.Set("call-1", []byte("signature-data"))

	sig, ok := cache.Get("call-1")
	require.True(t, ok)
	assert.Equal(t, []byte("signature-data"), sig)

	_, ok = cache.Get("call-2")
	assert.False(t, ok)
}

func TestStoreThoughtSignatures(t *testing.T) {
	cache := &thoughtSignatureCache{store: make(map[string][]byte)}

	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Role: "model",
					Parts: []*genai.Part{
						{
							FunctionCall: &genai.FunctionCall{
								ID:   "call-1",
								Name: "get_weather",
								Args: map[string]any{"city": "London"},
							},
							ThoughtSignature: []byte("sig-abc"),
						},
						{
							FunctionCall: &genai.FunctionCall{
								ID:   "call-2",
								Name: "book_taxi",
								Args: map[string]any{},
							},
							// No thought signature
						},
					},
				},
			},
		},
	}

	assignIDsAndStoreSignatures(resp, cache)

	sig, ok := cache.Get("call-1")
	require.True(t, ok)
	assert.Equal(t, []byte("sig-abc"), sig)

	_, ok = cache.Get("call-2")
	assert.False(t, ok, "call-2 had no thought signature, should not be cached")
}

func TestOpenaiToGenai_ToolCallsWithThoughtSignatures(t *testing.T) {
	cache := &thoughtSignatureCache{store: make(map[string][]byte)}
	cache.Set("call-1", []byte("sig-abc"))

	req := openai.ChatCompletionRequest{
		Model: "gemini-2.5-flash",
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "What's the weather?"},
			{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{
					{
						ID:   "call-1",
						Type: openai.ToolTypeFunction,
						Function: openai.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"city":"London"}`,
						},
					},
				},
			},
			{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: "call-1",
				Content:    `{"temp":"15C"}`,
			},
		},
	}

	contents, _ := openaiToGenai(req, cache)

	// Find the model content with FunctionCall
	var modelContent *genai.Content
	for _, c := range contents {
		if c.Role == "model" {
			modelContent = c
			break
		}
	}
	require.NotNil(t, modelContent)

	// Verify thought signature was restored on the FunctionCall part
	var fcPart *genai.Part
	for _, p := range modelContent.Parts {
		if p.FunctionCall != nil {
			fcPart = p
			break
		}
	}
	require.NotNil(t, fcPart)
	assert.Equal(t, []byte("sig-abc"), fcPart.ThoughtSignature)
}

func TestOpenaiToGenai_SystemMessage(t *testing.T) {
	req := openai.ChatCompletionRequest{
		Model: "gemini-2.5-flash",
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You are helpful"},
			{Role: openai.ChatMessageRoleUser, Content: "Hello"},
		},
	}

	contents, config := openaiToGenai(req, nil)

	// System message should be in config, not contents
	require.NotNil(t, config.SystemInstruction)
	assert.Equal(t, "You are helpful", config.SystemInstruction.Parts[0].Text)

	// Only user message in contents
	require.Len(t, contents, 1)
	assert.Equal(t, "user", string(contents[0].Role))
}

func TestOpenaiToGenai_MergesConsecutiveRoles(t *testing.T) {
	req := openai.ChatCompletionRequest{
		Model: "gemini-2.5-flash",
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "Check flight"},
			{
				Role: openai.ChatMessageRoleAssistant,
				ToolCalls: []openai.ToolCall{
					{ID: "c1", Type: openai.ToolTypeFunction, Function: openai.FunctionCall{Name: "check_flight", Arguments: "{}"}},
				},
			},
			{Role: openai.ChatMessageRoleTool, ToolCallID: "c1", Content: `{"status":"ok"}`},
			// This second tool result has the same effective role ("user") as the previous one
		},
	}

	contents, _ := openaiToGenai(req, nil)

	// Should be: user, model, user (tool result merged with nothing but still user role)
	require.Len(t, contents, 3)
	assert.Equal(t, "user", string(contents[0].Role))
	assert.Equal(t, "model", string(contents[1].Role))
	assert.Equal(t, "user", string(contents[2].Role))
}

func TestGenaiToOpenaiResponse_TextAndToolCalls(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		ResponseID: "resp-123",
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Role: "model",
					Parts: []*genai.Part{
						{
							FunctionCall: &genai.FunctionCall{
								ID:   "call-1",
								Name: "get_weather",
								Args: map[string]any{"city": "London"},
							},
						},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 20,
			TotalTokenCount:      30,
		},
	}

	result := genaiToOpenaiResponse(resp, "gemini-2.5-flash")

	assert.Equal(t, "resp-123", result.ID)
	assert.Equal(t, "gemini-2.5-flash", result.Model)
	assert.Equal(t, 10, result.Usage.PromptTokens)
	assert.Equal(t, 20, result.Usage.CompletionTokens)

	require.Len(t, result.Choices, 1)
	choice := result.Choices[0]
	assert.Equal(t, openai.FinishReasonStop, choice.FinishReason)

	require.Len(t, choice.Message.ToolCalls, 1)
	tc := choice.Message.ToolCalls[0]
	assert.Equal(t, "call-1", tc.ID)
	assert.Equal(t, "get_weather", tc.Function.Name)

	var args map[string]any
	require.NoError(t, json.Unmarshal([]byte(tc.Function.Arguments), &args))
	assert.Equal(t, "London", args["city"])
}

func TestGenaiToOpenaiResponse_SkipsThoughtParts(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Role: "model",
					Parts: []*genai.Part{
						{Text: "internal reasoning", Thought: true},
						{Text: "The weather is sunny."},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}

	result := genaiToOpenaiResponse(resp, "gemini-2.5-flash")

	require.Len(t, result.Choices, 1)
	// Should only contain the non-thought text
	assert.Equal(t, "The weather is sunny.", result.Choices[0].Message.Content)
}

func TestOpenaiToGenaiTools(t *testing.T) {
	tools := []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"city": map[string]interface{}{
							"type":        "string",
							"description": "City name",
						},
					},
					"required": []string{"city"},
				},
			},
		},
	}

	result := openaiToGenaiTools(tools)
	require.Len(t, result, 1)
	require.Len(t, result[0].FunctionDeclarations, 1)

	fd := result[0].FunctionDeclarations[0]
	assert.Equal(t, "get_weather", fd.Name)
	assert.Equal(t, "Get weather", fd.Description)
	assert.NotNil(t, fd.ParametersJsonSchema)
}

// Integration tests that call the real Gemini API.
// Skipped unless GEMINI_API_KEY is set.

func TestGeminiToolCall_Integration(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set, skipping integration test")
	}

	client := New(apiKey, "https://generativelanguage.googleapis.com/v1beta/openai", false)
	ctx := context.Background()
	model := "gemini-2.5-flash"

	tools := []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "get_weather",
				Description: "Get the current weather for a city",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"city": map[string]interface{}{
							"type":        "string",
							"description": "The city name",
						},
					},
					"required": []string{"city"},
				},
			},
		},
	}

	// Step 1: Send a message that should trigger a tool call
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "What's the weather in London? Use the get_weather tool."},
		},
		Tools: tools,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Choices)

	assistantMsg := resp.Choices[0].Message
	require.NotEmpty(t, assistantMsg.ToolCalls, "Expected Gemini to return tool calls")

	toolCall := assistantMsg.ToolCalls[0]
	assert.Equal(t, "get_weather", toolCall.Function.Name)
	assert.NotEmpty(t, toolCall.ID)

	t.Logf("Got tool call: id=%s name=%s args=%s", toolCall.ID, toolCall.Function.Name, toolCall.Function.Arguments)

	// Step 2: Send the tool result back — thought signatures should be
	// automatically preserved via the thoughtSignatureCache.
	resp2, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "What's the weather in London? Use the get_weather tool."},
			assistantMsg,
			{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: toolCall.ID,
				Content:    `{"temperature": "15°C", "condition": "cloudy", "humidity": "72%"}`,
			},
		},
		Tools: tools,
	})
	require.NoError(t, err, "Second request (with tool result) should succeed — thought signatures must be preserved")
	require.NotEmpty(t, resp2.Choices)

	finalContent := resp2.Choices[0].Message.Content
	t.Logf("Final response: %s", finalContent)
	assert.NotEmpty(t, finalContent, "Expected a text response after tool result")
}

func TestGeminiMultiTurnToolCall_Integration(t *testing.T) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("GEMINI_API_KEY not set, skipping integration test")
	}

	client := New(apiKey, "https://generativelanguage.googleapis.com/v1beta/openai", false)
	ctx := context.Background()
	model := "gemini-2.5-flash"

	tools := []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "check_flight",
				Description: "Check the status of a flight",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"flight_number": map[string]interface{}{
							"type":        "string",
							"description": "The flight number (e.g. AA100)",
						},
					},
					"required": []string{"flight_number"},
				},
			},
		},
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "book_taxi",
				Description: "Book a taxi for pickup",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"pickup_time": map[string]interface{}{
							"type":        "string",
							"description": "The pickup time",
						},
						"destination": map[string]interface{}{
							"type":        "string",
							"description": "The destination address",
						},
					},
					"required": []string{"pickup_time", "destination"},
				},
			},
		},
	}

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "Check flight AA100 status using the check_flight tool. You must use the tool."},
	}

	// Turn 1: Should get check_flight tool call
	resp1, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:      model,
		Messages:   messages,
		Tools:      tools,
		ToolChoice: "required",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resp1.Choices)

	msg1 := resp1.Choices[0].Message
	require.NotEmpty(t, msg1.ToolCalls, "Expected tool call in turn 1")

	t.Logf("Turn 1 tool calls: %d", len(msg1.ToolCalls))
	for _, tc := range msg1.ToolCalls {
		t.Logf("  id=%s name=%s args=%s", tc.ID, tc.Function.Name, tc.Function.Arguments)
	}

	// Build tool results for all tool calls in turn 1
	messages = append(messages, msg1)
	for _, tc := range msg1.ToolCalls {
		var content string
		switch tc.Function.Name {
		case "check_flight":
			content = `{"status": "delayed", "original_departure": "14:00", "new_departure": "16:00", "airport": "JFK"}`
		case "book_taxi":
			content = `{"confirmation": "TAXI-123", "pickup_time": "14:00", "destination": "JFK Airport"}`
		default:
			t.Fatalf("unexpected tool call: %s", tc.Function.Name)
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			ToolCallID: tc.ID,
			Content:    content,
		})
	}

	// Turn 2: Send tool results — may get another tool call or final response
	resp2, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
	})
	require.NoError(t, err, "Turn 2 should succeed — thought signatures from turn 1 must be preserved")
	require.NotEmpty(t, resp2.Choices)

	msg2 := resp2.Choices[0].Message
	t.Logf("Turn 2: content=%q toolCalls=%d", msg2.Content, len(msg2.ToolCalls))

	// If there are more tool calls (e.g. book_taxi), handle them
	if len(msg2.ToolCalls) > 0 {
		messages = append(messages, msg2)
		for _, tc := range msg2.ToolCalls {
			var content string
			switch tc.Function.Name {
			case "book_taxi":
				content = `{"confirmation": "TAXI-456", "pickup_time": "14:00", "destination": "JFK Airport"}`
			default:
				content = `{"result": "ok"}`
			}
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: tc.ID,
				Content:    content,
			})
		}

		// Turn 3
		resp3, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:    model,
			Messages: messages,
			Tools:    tools,
		})
		require.NoError(t, err, "Turn 3 should succeed — thought signatures from all turns must be preserved")
		require.NotEmpty(t, resp3.Choices)

		finalContent := resp3.Choices[0].Message.Content
		t.Logf("Turn 3 final response: %s", finalContent)
		assert.NotEmpty(t, finalContent, "Expected a final text response")
	} else {
		assert.NotEmpty(t, msg2.Content, "Expected a final text response in turn 2")
	}
}
