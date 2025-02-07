package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Slot is the crazy mirror equivalent of scheduler.Slot
// You can think of it as the same thing as a Slot, but it's a bit fatter because it ecapsulates all
// the horrible logic involved with starting and destroying a ModelInstance.
// E.g. axolotl expects a session, whereas ollama expects an LLMInferenceRequest.
type Slot struct {
	ID       uuid.UUID // Same as scheduler.Slot
	RunnerID string    // Same as scheduler.Slot
	Runtime  Runtime
	Model    string // The model assigned to this slot
}

type PullProgress struct {
	Status    string
	Completed int64
	Total     int64
}

type Runtime interface {
	Start(ctx context.Context) error
	Stop() error
	PullModel(ctx context.Context, model string, progress func(PullProgress) error) error
	Warm(ctx context.Context, model string) error
	Version() string
	Runtime() types.Runtime
	URL() string
}

type CreateSlotParams struct {
	RunnerOptions *Options
	ID            uuid.UUID
	Runtime       types.Runtime
	Model         string
}

func CreateSlot(ctx context.Context, params CreateSlotParams) (*Slot, error) {
	var r Runtime
	var err error
	switch params.Runtime {
	case types.RuntimeOllama:
		r, err = NewOllamaRuntime(ctx, OllamaRuntimeParams{
			CacheDir: &params.RunnerOptions.CacheDir,
		}) // TODO(phil): Add params
		if err != nil {
			return nil, err
		}
	case types.RuntimeDiffusers:
		r, err = NewDiffusersRuntime(ctx, DiffusersRuntimeParams{
			CacheDir: &params.RunnerOptions.CacheDir,
		}) // TODO(phil): Add params
		if err != nil {
			return nil, err
		}
	case types.RuntimeAxolotl:
		r, err = NewAxolotlRuntime(ctx, AxolotlRuntimeParams{
			RunnerOptions: params.RunnerOptions,
		}) // TODO(phil): Add params
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown runtime: %s", params.Runtime)
	}

	// Start the runtime
	err = r.Start(ctx)
	if err != nil {
		return nil, err
	}

	// Create OpenAI Client
	openAIClient, err := CreateOpenaiClient(ctx, fmt.Sprintf("%s/v1", r.URL()))
	if err != nil {
		return nil, err
	}
	// Check that the model is available in this runtime
	models, err := openAIClient.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	found := false
	for _, m := range models.Models {
		if m.ID == params.Model {
			found = true
			break
		}
	}
	if !found {
		// Pull the model if it's not already available
		err = r.PullModel(ctx, params.Model, func(progress PullProgress) error {
			log.Info().Str("status", progress.Status).Int64("completed", progress.Completed).Int64("total", progress.Total).Msgf("pulling model %s", params.Model)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Warm up the model
	err = r.Warm(ctx, params.Model)
	if err != nil {
		return nil, err
	}

	return &Slot{
		ID:       params.ID,
		RunnerID: params.RunnerOptions.ID,
		Model:    params.Model,
		Runtime:  r,
	}, nil
}
