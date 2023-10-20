package model

import (
	"context"

	"github.com/lukemarsden/helix/api/pkg/types"
)

// allows you to write into a processing function that emit chunks
// this is how we parse the output of language models
type TextStreamProcessor struct {
	Output chan string
}

type LanguageModel interface {
	// return the prompt we send into a model given the current session
	GetPrompt(ctx context.Context, session *types.Session) (string, error)
	// display the following text to the user whilst we are waiting for output
	GetLoading(ctx context.Context) (string, error)
	// return a text stream that knows how to parse the output of the model
	GetTextStream(ctx context.Context) (*TextStream, error)
}
