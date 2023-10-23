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

type Model interface {
	// return the number of bytes of memory this model will require
	// this enables the runner to multiplex models onto one GPU
	GetMemoryUsage(ctx context.Context) (uint64, error)
}

type LanguageModel interface {
	// return the prompt we send into a model given the current session
	GetPrompt(ctx context.Context, session *types.Session) (string, error)
	// return a text stream that knows how to parse the output of the model
	GetTextStream(ctx context.Context) (*TextStream, error)
}

type ImageModel interface {
	// return the prompt we send into a model given the current session
	GetPrompt(ctx context.Context, session *types.Session) (string, error)
}
