package agent

import (
	"context"

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
	// AcceptsNoneReasoningEffort is true when the model's catalog entry lists
	// "none" among its supported reasoning efforts. For those models "none"
	// must be SENT, not dropped: omitting reasoning_effort makes the provider
	// apply the model's default (e.g. "medium" on GPT-5.x), and on gpt-5.6-*
	// that default is rejected outright when the request carries function
	// tools ("Function tools with reasoning_effort are not supported ... set
	// reasoning_effort to 'none'"). Models that predate the value (o1/o3) must
	// keep having it stripped, hence the capability gate rather than a blanket
	// passthrough.
	AcceptsNoneReasoningEffort bool
}

// applyReasoningEffort resolves the effort actually sent on the wire.
//
// Callers such as decideNextAction build their request without an effort at
// all, so the model's configured effort has to be filled in here — otherwise
// the parameter is omitted and the provider silently applies its own default.
// The "none" sentinel is then resolved against the model's declared
// capability: sent verbatim when supported, stripped otherwise.
func applyReasoningEffort(model *LLMModelConfig, params openai.ChatCompletionRequest) openai.ChatCompletionRequest {
	effort := params.ReasoningEffort
	if effort == "" && model != nil {
		effort = model.ReasoningEffort
	}
	if effort == "none" && (model == nil || !model.AcceptsNoneReasoningEffort) {
		effort = ""
	}
	params.ReasoningEffort = effort
	return params
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
	params = applyReasoningEffort(model, params)

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
	params = applyReasoningEffort(model, params)

	params.StreamOptions = &openai.StreamOptions{
		IncludeUsage: true,
	}

	stream, err := model.Client.CreateChatCompletionStream(ctx, params)
	if err != nil {
		return nil, err
	}

	return stream, nil
}
