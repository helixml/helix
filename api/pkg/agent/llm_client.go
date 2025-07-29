package agent

import (
	"context"
	"strings"

	helix_openai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/system"
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

	// If we have got a response with a tool call, ensure we have an ID set, otherwise generate one
	if len(resp.Choices) > 0 && len(resp.Choices[0].Message.ToolCalls) > 0 {
		for idx, toolCall := range resp.Choices[0].Message.ToolCalls {
			if toolCall.ID == "" {
				resp.Choices[0].Message.ToolCalls[idx].ID = system.GenerateCallID()
			}
		}
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
	
	// TODO: Enable reasoning token streaming when OpenAI SDK supports it
	// For reasoning models, include reasoning tokens if supported
	// if isReasoningModel(model.Model) {
	//     params.StreamOptions.IncludeReasoning = true
	// }
	
	return model.Client.CreateChatCompletionStream(ctx, params)
}

// isReasoningModel checks if the model supports reasoning token streaming
func isReasoningModel(modelName string) bool {
	reasoningModels := []string{
		"o1", "o1-preview", "o1-mini", "o3", "o3-mini", "o4-mini",
		"claude-3-5-sonnet", "claude-3-7-sonnet", "claude-4-sonnet",
		"qwen3", "qwen-3",
	}
	
	for _, reasoning := range reasoningModels {
		if strings.Contains(strings.ToLower(modelName), reasoning) {
			return true
		}
	}
	return false
}
