// Package agent - tool.go
// Defines the Tool interface and basic stubs for tool usage.
package agent

import (
	"context"

	openai "github.com/sashabaranov/go-openai"
)

type Tool interface {
	String() string
	Name() string
	StatusMessage() string // not using now - but we will - soon
	Description() string
	OpenAI() []openai.Tool
	Execute(ctx context.Context, meta Meta, args map[string]interface{}) (string, error)
}
