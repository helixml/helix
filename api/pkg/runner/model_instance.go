package runner

import (
	"context"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
)

//go:generate mockgen -source $GOFILE -destination model_instance_mocks.go -package $GOPACKAGE
type ModelInstance interface {
	ID() string
	Filter() types.SessionFilter
	Stale() bool
	Model() model.Model
	GetState() (*types.ModelInstanceState, error)
	IsActive() bool
	Start(ctx context.Context) error
	Stop() error
	Done() <-chan bool

	// TODO: remove all below
	QueueSession(session *types.Session, isInitialSession bool)
}
