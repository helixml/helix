package agent

import (
	"context"

	"github.com/invopop/jsonschema"
	// "github.com/openai/openai-go"

	helix_openai "github.com/helixml/helix/api/pkg/openai"
	openai "github.com/sashabaranov/go-openai"
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
	client               helix_openai.Client
}

func NewLLM(client helix_openai.Client, reasoningModel string, generationModel string, smallReasoningModel string, smallGenerationModel string) *LLM {
	// var client openai.Client
	// if baseURL != "" {
	// 	client = openai.NewClient(option.WithBaseURL(baseURL), option.WithAPIKey(apiKey))
	// } else {
	// 	client = openai.NewClient(option.WithAPIKey(apiKey))
	// }
	return &LLM{
		// APIKey:               apiKey,
		// BaseURL:              baseURL,
		ReasoningModel:       reasoningModel,
		GenerationModel:      generationModel,
		SmallReasoningModel:  smallReasoningModel,
		SmallGenerationModel: smallGenerationModel,
		client:               client,
	}
}

// TODO failures like too long, non-processable etc from the LLM needs to be handled
func (c *LLM) New(ctx context.Context, params openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	return c.client.CreateChatCompletion(ctx, params)
}

func (c *LLM) NewStreaming(ctx context.Context, params openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return c.client.CreateChatCompletionStream(ctx, params)
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
