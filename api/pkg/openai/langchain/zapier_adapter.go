package langchain

import (
	"context"
	"fmt"

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

	chatMsgs := make([]*openai.ChatCompletionMessage, 0, len(messages))

	for _, mc := range messages {
		msg := &openai.ChatCompletionMessage{
			Role: string(mc.Role),
			// Content: mc.,
		}

		for _, part := range mc.Parts {
			switch p := part.(type) {
			case llms.TextContent:
				msg.MultiContent = append(msg.MultiContent, openai.ChatMessagePart{
					Type: openai.ChatMessagePartTypeText,
					Text: p.Text,
				})
			}
		}

		switch mc.Role {
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

		newParts, toolCalls := ExtractToolParts(msg)
		msg.MultiContent = newParts
		msg.ToolCalls = toolCallsFromToolCalls(toolCalls)

		chatMsgs = append(chatMsgs, msg)
	}

	return nil, nil
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
