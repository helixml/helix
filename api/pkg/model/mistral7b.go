package model

import (
	"context"
	"fmt"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type Mistral7bInstruct01 struct {
}

func (l *Mistral7bInstruct01) GetPrompt(ctx context.Context, session *types.Session) (string, error) {
	var messages string
	for _, message := range session.Interactions.Messages {
		messages += message.Message + "\n"
	}
	return fmt.Sprintf("[INST]%s[/INST]", messages), nil
}

func (l *Mistral7bInstruct01) GetLoading(ctx context.Context) (string, error) {
	return "🤔... \n\n", nil
}

func (l *Mistral7bInstruct01) GetTextStream(ctx context.Context) (*TextStream, error) {
	stream := NewTextStream(
		splitOnSpace,
		"[/INST]",
		"</s>",
	)
	return stream, nil
}

// Compile-time interface check:
var _ LanguageModel = (*Mistral7bInstruct01)(nil)
