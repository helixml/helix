package agent

import (
	"context"

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
	return model.Client.CreateChatCompletionStream(ctx, params)
}
