package model

import (
	"context"
	"fmt"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type Mistral7bInstruct01 struct {
}

func (l *Mistral7bInstruct01) GetMemoryRequirements(ctx context.Context) uint64 {
	return GB * 12
}

func (l *Mistral7bInstruct01) GetPrompt(ctx context.Context, session *types.Session) (string, error) {
	if len(session.Interactions) == 0 {
		return "", fmt.Errorf("session has no messages")
	}
	lastInteraction := session.Interactions[len(session.Interactions)-1]
	return fmt.Sprintf("[INST]%s[/INST]", lastInteraction.Message), nil
}

func (l *Mistral7bInstruct01) GetTextStream(ctx context.Context) (*TextStream, error) {
	return NewTextStream(
		splitOnSpace,
		"[/INST]",
		"</s>",
	), nil
}

// Compile-time interface check:
var _ LanguageModel = (*Mistral7bInstruct01)(nil)
var _ Model = (*Mistral7bInstruct01)(nil)
