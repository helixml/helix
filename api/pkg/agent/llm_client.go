package agent

import (
	"context"

	"github.com/invopop/jsonschema"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/ssestream"
)

// Define a custom type for context keys
type ContextKey string

type LLM struct {
	APIKey               string
	BaseURL              string
	ReasoningModel       string
	GenerationModel      string
	SmallReasoningModel  string
	SmallGenerationModel string
	client               openai.Client
}

func NewLLM(apiKey string, baseURL string, reasoningModel string, generationModel string, smallReasoningModel string, smallGenerationModel string) *LLM {
	var client openai.Client
	if baseURL != "" {
		client = openai.NewClient(option.WithBaseURL(baseURL), option.WithAPIKey(apiKey))
	} else {
		client = openai.NewClient(option.WithAPIKey(apiKey))
	}
	return &LLM{
		APIKey:               apiKey,
		BaseURL:              baseURL,
		ReasoningModel:       reasoningModel,
		GenerationModel:      generationModel,
		SmallReasoningModel:  smallReasoningModel,
		SmallGenerationModel: smallGenerationModel,
		client:               client,
	}
}

// TODO failures like too long, non-processable etc from the LLM needs to be handled
func (c *LLM) New(ctx context.Context, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {

	return c.client.Chat.Completions.New(ctx, params)
}

func (c *LLM) NewStreaming(ctx context.Context, params openai.ChatCompletionNewParams) *ssestream.Stream[openai.ChatCompletionChunk] {

	return c.client.Chat.Completions.NewStreaming(ctx, params)
}

func GenerateSchema[T any]() interface{} {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v T
	schema := reflector.Reflect(v)
	return schema
}
