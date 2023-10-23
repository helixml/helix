package model

import (
	"context"
	"os/exec"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type Model interface {
	// return the number of bytes of memory this model will require
	// this enables the runner to multiplex models onto one GPU
	GetMemoryRequirements(mode types.SessionMode) uint64
	// tells you if this model is text or image based
	GetType() types.SessionType
	// return the prompt we send into a model given the current session
	// this is used for doing inference
	GetPrompt(ctx context.Context, session *types.Session) (string, error)
	// return a text stream that knows how to parse the output of the model
	// only language models doing inference on text will implement this
	GetTextStream(ctx context.Context) (*TextStream, error)
	// the function we call to get the python process booted and
	// asking us for work
	// this relies on the axotl and sd-script repos existing
	// at the same level as the helix - and the weights downloaded
	// we are either booting for inference or fine-tuning
	GetCommand(ctx context.Context, mode types.SessionMode) (*exec.Cmd, error)
}
