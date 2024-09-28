package langchain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	helix_openai "github.com/helixml/helix/api/pkg/openai"
	openai "github.com/sashabaranov/go-openai"

	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/llms"
)

var _ llms.Model = (*LangchainAdapter)(nil)

type LangchainAdapter struct {
	client helix_openai.Client
	model  string

	CallbacksHandler callbacks.Handler
}

func New(client helix_openai.Client, model string) (*LangchainAdapter, error) {
	return &LangchainAdapter{
		client: client,
		model:  model,
	}, nil
}

func (a *LangchainAdapter) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	return llms.GenerateFromSinglePrompt(ctx, a, prompt, options...)
}

func (a *LangchainAdapter) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	if a.CallbacksHandler != nil {
		a.CallbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	}

	opts := llms.CallOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	chatMsgs := make([]openai.ChatCompletionMessage, 0, len(messages))

	for _, mc := range messages {
		msg := openai.ChatCompletionMessage{}

		for _, part := range mc.Parts {
			switch p := part.(type) {
			case llms.TextContent:
				msg.Content = p.Text
			case llms.ImageURLContent:
				msg.MultiContent = append(msg.MultiContent, openai.ChatMessagePart{
					Type:     openai.ChatMessagePartTypeImageURL,
					ImageURL: &openai.ChatMessageImageURL{URL: p.URL},
				})
			case llms.ToolCall:
				msg.ToolCalls = append(msg.ToolCalls, openai.ToolCall{
					ID:   p.ID,
					Type: openai.ToolType(p.Type),
					Function: openai.FunctionCall{
						Name:      p.FunctionCall.Name,
						Arguments: p.FunctionCall.Arguments,
					},
				})
			}
		}

		switch mc.Role {
		case llms.ChatMessageTypeSystem:
			msg.Role = openai.ChatMessageRoleSystem
		case llms.ChatMessageTypeAI:
			msg.Role = openai.ChatMessageRoleAssistant
		case llms.ChatMessageTypeHuman:
			msg.Role = openai.ChatMessageRoleUser
		case llms.ChatMessageTypeGeneric:
			msg.Role = openai.ChatMessageRoleUser
		case llms.ChatMessageTypeFunction:
			msg.Role = openai.ChatMessageRoleFunction
		case llms.ChatMessageTypeTool:
			// Here we extract tool calls from the message and populate the ToolCalls field.
			// parse mc.Parts (which should have one entry of type ToolCallResponse) and populate msg.Content and msg.ToolCallID
			if len(mc.Parts) != 1 {
				return nil, fmt.Errorf("expected exactly one part for role %v, got %v", mc.Role, len(mc.Parts))
			}
			switch p := mc.Parts[0].(type) {
			case llms.ToolCallResponse:
				msg.ToolCallID = p.ToolCallID
				msg.Content = p.Content
			default:
				return nil, fmt.Errorf("expected part of type ToolCallResponse for role %v, got %T", mc.Role, mc.Parts[0])
			}

		default:
			return nil, fmt.Errorf("role %v not supported", mc.Role)
		}

		chatMsgs = append(chatMsgs, msg)
	}

	var seed int

	if opts.Seed > 0 {
		seed = opts.Seed
	}

	model := a.model

	if opts.Model != "" {
		model = opts.Model
	}

	req := openai.ChatCompletionRequest{
		Model:    model,
		Stop:     opts.StopWords,
		Messages: chatMsgs,
		// TODO:
		// StreamingFunc:    opts.StreamingFunc,
		Temperature:      float32(opts.Temperature),
		MaxTokens:        opts.MaxTokens,
		N:                opts.N,
		FrequencyPenalty: float32(opts.FrequencyPenalty),
		PresencePenalty:  float32(opts.PresencePenalty),

		ToolChoice: opts.ToolChoice,
		// TODO:
		// FunctionCallBehavior: openaiclient.FunctionCallBehavior(opts.FunctionCallBehavior),
		Seed: &seed,
		// Metadata: opts.Metadata,
	}

	if opts.JSONMode {
		req.ResponseFormat = &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		}
	}

	for _, fn := range opts.Functions {
		req.Tools = append(req.Tools, openai.Tool{
			Type: "function",
			Function: &openai.FunctionDefinition{
				Name:        fn.Name,
				Description: fn.Description,
				Parameters:  fn.Parameters,
			},
		})
	}

	// if opts.Tools is not empty, append them to req.Tools
	for _, tool := range opts.Tools {
		t, err := toolFromTool(tool)
		if err != nil {
			return nil, fmt.Errorf("failed to convert llms tool to openai tool: %w", err)
		}
		req.Tools = append(req.Tools, t)
	}

	spew.Dump(req)

	result, err := a.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %w", err)
	}

	ts := time.Now()

	os.WriteFile(ts.String()+".req.json", []byte(spew.Sprint(req)), 0o644)
	os.WriteFile(ts.String()+".result.json", []byte(spew.Sprint(result)), 0o644)

	if len(result.Choices) == 0 {
		return nil, errors.New("no response")
	}

	choices := make([]*llms.ContentChoice, len(result.Choices))
	for i, c := range result.Choices {
		choices[i] = &llms.ContentChoice{
			Content:    c.Message.Content,
			StopReason: fmt.Sprint(c.FinishReason),
			GenerationInfo: map[string]any{
				"CompletionTokens": result.Usage.CompletionTokens,
				"PromptTokens":     result.Usage.PromptTokens,
				"TotalTokens":      result.Usage.TotalTokens,
			},
		}

		// Legacy function call handling
		if c.FinishReason == "function_call" {
			choices[i].FuncCall = &llms.FunctionCall{
				Name:      c.Message.FunctionCall.Name,
				Arguments: c.Message.FunctionCall.Arguments,
			}
		}
		for _, tool := range c.Message.ToolCalls {
			choices[i].ToolCalls = append(choices[i].ToolCalls, llms.ToolCall{
				ID:   tool.ID,
				Type: string(tool.Type),
				FunctionCall: &llms.FunctionCall{
					Name:      tool.Function.Name,
					Arguments: tool.Function.Arguments,
				},
			})
		}
		// populate legacy single-function call field for backwards compatibility
		if len(choices[i].ToolCalls) > 0 {
			choices[i].FuncCall = choices[i].ToolCalls[0].FunctionCall
		}
	}
	response := &llms.ContentResponse{Choices: choices}
	if a.CallbacksHandler != nil {
		a.CallbacksHandler.HandleLLMGenerateContentEnd(ctx, response)
	}
	return response, nil
}

// ExtractToolParts extracts the tool parts from a message.
func ExtractToolParts(msg *openai.ChatCompletionMessage) ([]openai.ChatMessagePart, []openai.ToolCall) {
	var content []openai.ChatMessagePart
	var toolCalls []openai.ToolCall
	for _, part := range msg.MultiContent {
		switch part.Type {
		case openai.ChatMessagePartTypeText:
			content = append(content, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeText,
				Text: part.Text,
			})
		case openai.ChatMessagePartTypeImageURL:
			content = append(content, part)
			// case llms.BinaryContent:
			// 	content = append(content, p)
			// TODO:
			// case llms.ToolCall:
			// 	toolCalls = append(toolCalls, p)
		}
	}
	return content, toolCalls
}

// toolFromTool converts an llms.Tool to a Tool.
func toolFromTool(t llms.Tool) (openai.Tool, error) {
	tool := openai.Tool{
		Type: openai.ToolType(t.Type),
	}
	switch t.Type {
	case string(openai.ToolTypeFunction):
		tool.Function = &openai.FunctionDefinition{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		}
	default:
		return openai.Tool{}, fmt.Errorf("tool type %v not supported", t.Type)
	}
	return tool, nil
}
