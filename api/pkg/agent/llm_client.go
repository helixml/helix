package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/davecgh/go-spew/spew"
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
	return &LLM{
		ReasoningModel:       reasoningModel,
		GenerationModel:      generationModel,
		SmallReasoningModel:  smallReasoningModel,
		SmallGenerationModel: smallGenerationModel,
		client:               client,
	}
}

// TODO failures like too long, non-processable etc from the LLM needs to be handled
func (c *LLM) New(ctx context.Context, params openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {

	resp, err := c.client.CreateChatCompletion(ctx, params)
	if err != nil {
		// If we got bad request (400), then dump the request and response
		if strings.Contains(err.Error(), "400") {
			fmt.Println("==== FAILED LLM CALL ====")
			spew.Dump(params)
			fmt.Println("==== END LLM CALL ====")
		}
		return openai.ChatCompletionResponse{}, err
	}

	return resp, nil
}

func (c *LLM) NewStreaming(ctx context.Context, params openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	// fmt.Println("XXX NewStreaming")
	// spew.Dump(params)
	return c.client.CreateChatCompletionStream(ctx, params)
}
