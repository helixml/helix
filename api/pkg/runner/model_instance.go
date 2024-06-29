package runner

import (
	"context"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
)

type ModelInstance interface {
	ID() string
	Filter() types.SessionFilter
	Stale() bool
	Model() model.Model
	GetState() (*types.ModelInstanceState, error)

	Start(session *types.Session) error

	NextSession() *types.Session
	SetNextSession(session *types.Session)

	QueueSession(session *types.Session, isInitialSession bool)
	GetQueuedSession() *types.Session

	AssignSessionTask(ctx context.Context, session *types.Session) (*types.RunnerTask, error)

	Stop() error

	Done() <-chan bool
}

type LLMModelInstance interface {
	ID() string
	Stale() bool
	Model() model.Model
	GetState() (*types.ModelInstanceState, error)

	Run(ctx context.Context) error
	Stop() error

	CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
	CreateChatCompletionStream(ctx context.Context, req openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error)
}
