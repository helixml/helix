package runner

import (
	"context"
	"os/exec"

	"github.com/lukemarsden/helix/api/pkg/model"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// wrap a model instance in the context of the session that started it
// and be able to stop the process after a certain time of inactivity
type ModelInstance struct {
	id string

	model  model.Model
	filter types.SessionFilter

	// the process running the python model will quit when we call this
	ctx        context.Context
	cancelFunc context.CancelFunc

	// the session currently running on this model
	currentSession *types.Session

	// the next session that will be run on this model
	nextSession *types.Session

	// basically the timestamp of when the last job was finished
	lastJobCompletedTimestamp int64
}

func NewModelInstance(
	ctx context.Context,
	modelName types.ModelName,
	mode types.SessionMode,
) (*ModelInstance, error) {
	modelInstance, err := model.GetModel(modelName)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	return &ModelInstance{
		id:    system.GenerateUUID(),
		model: modelInstance,
		filter: types.SessionFilter{
			ModelName: modelName,
			Mode:      mode,
		},
		ctx:        ctx,
		cancelFunc: cancel,
	}, nil
}

// run the model process
func (instance *ModelInstance) start() error {
	cmd, err := instance.model.GetCommand(instance.ctx, instance.filter.Mode)
	if err != nil {
		return err
	}

	go func(cmd *exec.Cmd) {
		if err := cmd.Start(); err != nil {
			log.Error().Msgf("Failed to start command: %v\n", err.Error())
			return
		}

		if err := cmd.Wait(); err != nil {
			log.Error().Msgf("Failed to wait for command: %v\n", err.Error())
		}
	}(cmd)
	return nil
}

func (model *ModelInstance) stop() error {
	model.cancelFunc()
	return nil
}
