package model

import (
	"context"
	"os/exec"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type WorkerEventHandler func(res *types.RunnerTaskResponse)

type TextStreamType string

const (
	TextStreamTypeStdout TextStreamType = "stdout"
	TextStreamTypeStderr TextStreamType = "stderr"
)

type Model interface {
	// return the number of bytes of memory this model will require
	// this enables the runner to multiplex models onto one GPU
	GetMemoryRequirements(mode types.SessionMode) uint64

	// tells you if this model is text or image based
	GetType() types.SessionType

	// convert a session (which has an active mode i.e. inference or finetune) into a task
	// this primarily means constructing the prompt
	// and downloading files from the filestore
	// we don't need to fill in the SessionID and Session fields
	// the runner controller will do that for us
	GetTask(session *types.Session) (*types.RunnerTask, error)

	// the function we call to get the python process booted and
	// asking us for work
	// this relies on the axotl and sd-script repos existing
	// at the same level as the helix - and the weights downloaded
	// we are either booting for inference or fine-tuning
	GetCommand(ctx context.Context, sessionFilter types.SessionFilter, config types.RunnerProcessConfig) (*exec.Cmd, error)

	// return a text stream that knows how to parse the stdout of a running python process
	// this usually means it will split by newline and then check for codes
	// the python has included to infer meaning
	// but it's really up to the model to decide how to parse the output
	// the eventHandler is the function that is wired up to the runner controller
	// and will update the api with changes to the given session
	GetTextStreams(mode types.SessionMode, eventHandler WorkerEventHandler) (*TextStream, *TextStream, error)
}
