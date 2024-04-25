package runner

import (
	"context"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
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
