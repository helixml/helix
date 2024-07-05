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

	Start(ctx context.Context) error
	Stop() error
	Done() <-chan bool

	// TODO: remove all below
	QueueSession(session *types.Session, isInitialSession bool)
}
