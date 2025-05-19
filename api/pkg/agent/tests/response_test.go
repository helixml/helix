package tests

import (
	"context"
	"testing"

	agentpod "github.com/helixml/helix/api/pkg/agent"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
)

func TestNewResponseWithWebSearchTool(t *testing.T) {
	config, err := LoadConfig()
	if err != nil {
		t.Fatal("Failed to load config:", err)
	}
	if config.OpenAIAPIKey == "" {
		t.Skip("Skipping test because OpenAI credentials are not set")
	}

	// Create a new LLM client with Keywords AI configuration
	llm := agentpod.NewLLM(
		config.OpenAIAPIKey,
		config.BaseURL,
		config.ReasoningModel,
		config.GenerationModel,
		config.SmallReasoningModel,
		config.SmallGenerationModel,
	)

	// Create a context with metadata
	ctx := context.WithValue(context.Background(), agentpod.ContextKey("sessionID"), "test-session-123")
	ctx = context.WithValue(ctx, agentpod.ContextKey("customerID"), "test-customer-456")
	ctx = context.WithValue(ctx, agentpod.ContextKey("extra"), map[string]string{
		"user_id":  "test-user-789",
		"test_key": "test_value",
	})

	// Create test parameters for the Response API
	params := responses.ResponseNewParams{
		Model: shared.ResponsesModel("gpt-4o"),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: param.Opt[string]{Value: "Where to buy the Sargent 8888 F Rim Exit Device?"},
		},
		Tools: []responses.ToolUnionParam{
			{
				OfWebSearch: &responses.WebSearchToolParam{
					Type: responses.WebSearchToolTypeWebSearchPreview,
				},
			},
		},
		ToolChoice: responses.ResponseNewParamsToolChoiceUnion{
			OfHostedTool: &responses.ToolChoiceTypesParam{
				Type: responses.ToolChoiceTypesTypeWebSearchPreview,
			},
		},
	}

	// Call the NewResponse function
	response, err := llm.NewResponse(ctx, params)
	if err != nil {
		t.Fatalf("Failed to create response: %v", err)
	}

	// Basic validation of the response
	if response == nil {
		t.Fatal("Expected non-nil response")
	}

	// Check if the response contains the expected content
	// Note: The actual content will depend on the model's response
	if response.OutputText() == "" {
		t.Error("Expected non-empty response content")
	}

	if response.Output[0].Type != "web_search_call" {
		t.Errorf("Expected output type to be 'web_search_call', got: %s", response.Output[0].Type)
	}

	if response.Output[0].Status == "" {
		t.Error("Expected non-empty status for web search call")
	}

	// Verify there is at least one annotation in the response
	foundAnnotation := false
	for _, output := range response.Output {
		if output.Type == "refusal" {
			continue
		}
		for _, content := range output.Content {
			if content.Type == "output_text" && len(content.Annotations) > 0 {
				foundAnnotation = true
				break
			}
		}
		if foundAnnotation {
			break
		}
	}
	if !foundAnnotation {
		t.Error("Expected at least one annotation in the response")
	}

	// Verify the response contains the correct answer
	// if !strings.Contains(strings.ToLower(response.OutputText()), "paris") {
	// 	t.Errorf("Expected response to contain 'Paris', got: %s", response.OutputText())
	// }
}
