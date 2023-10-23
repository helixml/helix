package runner

import (
	"context"

	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
)

// wraps the more generic SDXL or Mistral model types
// (which can handle both fine tuning and inference)
// in a "runner" wrapper that represents one of the two
// modes in a long running python process that will reach
// out to the runner parent (over http) to pull new work
// this struct is kept in memory by the runner controller
// and it represents a single long running instance of a model
// running in either inference or training mode
type ModelWrapper struct {
	id        string
	modelName types.ModelName
	mode      types.SessionMode
	model     model.Model

	// the session currently running on this model
	currentSession *types.Session

	// the next session that will be run on this model
	nextSession *types.Session
}

func NewModelWrapper(
	ctx context.Context,
	modelName types.ModelName,
	mode types.SessionMode,
) (*ModelWrapper, error) {
	modelInstance, err := model.GetModel(modelName)
	if err != nil {
		return nil, err
	}
	return &ModelWrapper{
		id:        system.GenerateUUID(),
		modelName: modelName,
		mode:      mode,
		model:     modelInstance,
	}, nil
}
