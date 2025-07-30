package agent

import (
	"context"
	"fmt"
	"strings"

	helix_openai "github.com/helixml/helix/api/pkg/openai"
	openai "github.com/sashabaranov/go-openai"
)

// Define a custom type for context keys
type ContextKey string

type LLM struct {
	ReasoningModel       *LLMModelConfig
	GenerationModel      *LLMModelConfig
	SmallReasoningModel  *LLMModelConfig
	SmallGenerationModel *LLMModelConfig
}

// LLMModelConfig holds info for a specific LLM model
type LLMModelConfig struct {
	Client          helix_openai.Client
	Model           string
	ReasoningEffort string
}

func NewLLM(reasoningModel *LLMModelConfig, generationModel *LLMModelConfig, smallReasoningModel *LLMModelConfig, smallGenerationModel *LLMModelConfig) *LLM {
	return &LLM{
		ReasoningModel:       reasoningModel,
		GenerationModel:      generationModel,
		SmallReasoningModel:  smallReasoningModel,
		SmallGenerationModel: smallGenerationModel,
	}
}

func (c *LLM) New(ctx context.Context, model *LLMModelConfig, params openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	// Reset the reasoning effort to none if it's not set
	if params.ReasoningEffort == "none" {
		params.ReasoningEffort = ""
	}

	resp, err := model.Client.CreateChatCompletion(ctx, params)
	if err != nil {
		return openai.ChatCompletionResponse{}, err
	}

	return resp, nil
}

func (c *LLM) NewStreaming(ctx context.Context, model *LLMModelConfig, params openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	// Reset the reasoning effort to none if it's not set
	if params.ReasoningEffort == "none" {
		params.ReasoningEffort = ""
	}

	params.StreamOptions = &openai.StreamOptions{
		IncludeUsage: true,
	}

	// Detect provider types that need special handling
	modelLower := strings.ToLower(model.Model)
	paramsModelLower := strings.ToLower(params.Model)

	// Together.ai and vLLM backends need tool call workarounds
	isTogetherAIOrVLLM := strings.Contains(modelLower, "qwen") ||
		strings.Contains(paramsModelLower, "together") ||
		strings.Contains(modelLower, "together") ||
		strings.Contains(modelLower, "vllm")

	// TOGETHER.AI/VLLM TOOL CALL CONVERSION:
	// Convert tool_calls in assistant messages to readable text descriptions for LLM context.
	// This fixes compatibility issues with Together.ai and vLLM streaming APIs that reject
	// requests with tool_calls in message history. OpenAI handles tool_calls properly.
	if isTogetherAIOrVLLM {
		fixedMessages := make([]openai.ChatCompletionMessage, len(params.Messages))
		copy(fixedMessages, params.Messages)

		for i, msg := range fixedMessages {
			if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
				// Convert tool calls to readable text and append to content
				var toolDescriptions []string
				for _, toolCall := range msg.ToolCalls {
					if toolCall.Function.Name != "" {
						description := fmt.Sprintf("I used the %s tool", toolCall.Function.Name)
						if toolCall.Function.Arguments != "" {
							description += fmt.Sprintf(" with arguments: %s", toolCall.Function.Arguments)
						}
						toolDescriptions = append(toolDescriptions, description)
					}
				}

				if len(toolDescriptions) > 0 {
					toolText := "\n\n[Previous tool calls: " + strings.Join(toolDescriptions, "; ") + "]"
					fixedMessages[i].Content = msg.Content + toolText
				}

				// Remove the original tool_calls
				fixedMessages[i].ToolCalls = nil
			}
		}

		params.Messages = fixedMessages
	}

	stream, err := model.Client.CreateChatCompletionStream(ctx, params)
	if err != nil {
		return nil, err
	}

	return stream, nil
}
