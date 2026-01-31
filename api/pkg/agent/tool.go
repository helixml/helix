// Package agent - tool.go
// Defines the Tool interface and basic stubs for tool usage.
package agent

import (
	"context"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

//go:generate mockgen -source $GOFILE -destination tool_mocks.go -package $GOPACKAGE

type Tool interface {
	String() string
	Name() string
	Icon() string          // Either Material UI icon, emoji or SVG. Leave empty for default
	StatusMessage() string // not using now - but we will - soon
	Description() string
	OpenAI() []openai.Tool
	Execute(ctx context.Context, meta Meta, args map[string]interface{}) (string, error)
}

func getUniqueToolCalls(toolCalls []openai.ToolCall) []openai.ToolCall {
	seen := make(map[string]bool)

	uniqueToolCalls := []openai.ToolCall{}

	for _, toolCall := range toolCalls {
		// Create a unique key based on function name and arguments
		key := toolCall.Function.Name + ":" + toolCall.Function.Arguments
		if !seen[key] {
			seen[key] = true
			uniqueToolCalls = append(uniqueToolCalls, toolCall)
		} else {
			log.Warn().
				Str("tool_call", toolCall.Function.Name).
				Str("arguments", toolCall.Function.Arguments).
				Msg("Removing duplicate tool call")
		}
	}
	return uniqueToolCalls
}
