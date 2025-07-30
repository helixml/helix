package agent

import (
	"context"
	"fmt"
	"strings"

	helix_openai "github.com/helixml/helix/api/pkg/openai"
	"github.com/rs/zerolog/log"
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

	// DEBUG: Log the COMPLETE non-streaming request for comparison
	log.Error().
		Str("model", params.Model).
		Int("message_count", len(params.Messages)).
		Str("tool_choice", fmt.Sprintf("%v", params.ToolChoice)).
		Int("tools_count", len(params.Tools)).
		Msg("🔍 COMPLETE NON-STREAMING REQUEST DETAILS")

	// Log each message in detail for comparison
	for i, msg := range params.Messages {
		log.Error().
			Int("msg_index", i).
			Str("role", msg.Role).
			Str("content", msg.Content).
			Int("content_length", len(msg.Content)).
			Bool("content_empty", msg.Content == "").
			Int("multi_content_parts", len(msg.MultiContent)).
			Int("tool_calls", len(msg.ToolCalls)).
			Str("tool_call_id", msg.ToolCallID).
			Msg("🔍 NON-STREAMING MESSAGE DETAIL")
	}

	resp, err := model.Client.CreateChatCompletion(ctx, params)
	if err != nil {
		// DEBUG: Log the exact error and request details for non-streaming
		log.Error().
			Err(err).
			Str("model", params.Model).
			Int("message_count", len(params.Messages)).
			Interface("complete_request", params).
			Msg("🚨 NON-STREAMING REQUEST FAILED - COMPLETE REQUEST ABOVE")
		return openai.ChatCompletionResponse{}, err
	}

	log.Error().
		Str("model", params.Model).
		Int("message_count", len(params.Messages)).
		Msg("✅ NON-STREAMING REQUEST SUCCEEDED")

	return resp, nil
}

func (c *LLM) NewStreaming(ctx context.Context, model *LLMModelConfig, params openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	// Reset the reasoning effort to none if it's not set
	if params.ReasoningEffort == "none" {
		params.ReasoningEffort = ""
	}

	// Only set StreamOptions for providers that support it (not Together.ai)
	// Together.ai might reject requests with StreamOptions
	isTogetherAI := strings.Contains(strings.ToLower(model.Model), "qwen") || strings.Contains(strings.ToLower(params.Model), "together")
	if !isTogetherAI {
		params.StreamOptions = &openai.StreamOptions{
			IncludeUsage: true,
		}
	}

	// UNIVERSAL TOOL CALL CONVERSION:
	// Convert tool_calls in assistant messages to readable text descriptions for LLM context.
	// This improves compatibility across all providers (especially Together.ai's streaming API)
	// while preserving conversation context. The LLM will be instructed not to copy this pattern.
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

				log.Debug().
					Int("msg_index", i).
					Int("tool_calls_count", len(msg.ToolCalls)).
					Str("tool_descriptions", strings.Join(toolDescriptions, "; ")).
					Msg("🔧 Converted tool_calls to context descriptions")
			}

			// Remove the original tool_calls
			fixedMessages[i].ToolCalls = nil
		}
	}

	params.Messages = fixedMessages

	// DEBUG: Log the COMPLETE request being sent to identify the failing request
	log.Error().
		Str("model", params.Model).
		Int("message_count", len(params.Messages)).
		Bool("is_together_ai", isTogetherAI).
		Bool("has_stream_options", params.StreamOptions != nil).
		Str("tool_choice", fmt.Sprintf("%v", params.ToolChoice)).
		Int("tools_count", len(params.Tools)).
		Msg("🔍 COMPLETE STREAMING REQUEST DETAILS")

	// Log each message in detail to find the problematic one
	for i, msg := range params.Messages {
		log.Error().
			Int("msg_index", i).
			Str("role", msg.Role).
			Str("content", msg.Content).
			Int("content_length", len(msg.Content)).
			Bool("content_empty", msg.Content == "").
			Int("multi_content_parts", len(msg.MultiContent)).
			Int("tool_calls", len(msg.ToolCalls)).
			Str("tool_call_id", msg.ToolCallID).
			Msg("🔍 MESSAGE DETAIL")

		// Log the full content if it's short, truncate if long
		if len(msg.Content) > 500 {
			log.Error().
				Int("msg_index", i).
				Str("content_truncated", msg.Content[:500]+"...").
				Msg("🔍 TRUNCATED CONTENT (>500 chars)")
		} else if len(msg.Content) > 0 {
			log.Error().
				Int("msg_index", i).
				Str("full_content", msg.Content).
				Msg("🔍 FULL CONTENT")
		}

		// Log tool calls in detail
		for j, toolCall := range msg.ToolCalls {
			log.Error().
				Int("msg_index", i).
				Int("tool_call_index", j).
				Str("tool_call_id", toolCall.ID).
				Str("tool_call_type", string(toolCall.Type)).
				Str("function_name", toolCall.Function.Name).
				Str("function_args", toolCall.Function.Arguments).
				Msg("🔍 TOOL CALL DETAIL")
		}

		// Log MultiContent parts
		for j, part := range msg.MultiContent {
			log.Error().
				Int("msg_index", i).
				Int("part_index", j).
				Str("part_type", string(part.Type)).
				Str("part_text", part.Text).
				Msg("🔍 MULTI CONTENT PART")
		}
	}

	// Log tools available
	for i, tool := range params.Tools {
		log.Error().
			Int("tool_index", i).
			Str("tool_type", string(tool.Type)).
			Str("function_name", tool.Function.Name).
			Str("function_description", tool.Function.Description).
			Msg("🔍 AVAILABLE TOOL")
	}

	stream, err := model.Client.CreateChatCompletionStream(ctx, params)
	if err != nil {
		// DEBUG: Log the exact error and request details
		log.Error().
			Err(err).
			Str("model", params.Model).
			Int("message_count", len(params.Messages)).
			Str("model_name", model.Model).
			Bool("has_stream_options", params.StreamOptions != nil).
			Bool("is_together_ai", isTogetherAI).
			Interface("complete_request", params).
			Msg("🚨 STREAMING REQUEST FAILED - COMPLETE REQUEST ABOVE")
		return nil, err
	}

	return stream, nil
}
