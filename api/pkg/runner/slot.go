package runner

import (
	"context"
	"fmt"
	"strings"

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
	Runtime  Runtime   // TODO(Phil): This is dangerous because it can be nil
	Model    string    // The model assigned to this slot
	Active   bool      // True if the slot is active
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

// If there is an error at any point during creation, we call Stop to kill the runtime. Otherwise it
// can just sit there taking up GPU and doing nothing.
func CreateSlot(ctx context.Context, params CreateSlotParams) (s *Slot, err error) {
	s = &Slot{
		ID:       params.ID,
		RunnerID: params.RunnerOptions.ID,
		Model:    params.Model,
		Runtime:  nil,
		Active:   true,
	}
	// Need to be very careful to shutdown the runtime if there is an error!
	// Safest to do this in a defer so that it always checks.
	defer func() {
		if err != nil {
			if s.Runtime != nil {
				log.Warn().Str("model", params.Model).Str("runtime", string(params.Runtime)).Msg("error creating slot, stopping runtime")
				stopErr := s.Runtime.Stop()
				if stopErr != nil {
					log.Error().Err(stopErr).Str("model", params.Model).Str("runtime", string(params.Runtime)).Msg("error stopping runtime, possible memory leak")
				}
			}
		}
	}()

	switch params.Runtime {
	case types.RuntimeOllama:
		s.Runtime, err = NewOllamaRuntime(ctx, OllamaRuntimeParams{
			CacheDir: &params.RunnerOptions.CacheDir,
		}) // TODO(phil): Add params
		if err != nil {
			return
		}
	case types.RuntimeDiffusers:
		s.Runtime, err = NewDiffusersRuntime(ctx, DiffusersRuntimeParams{
			CacheDir: &params.RunnerOptions.CacheDir,
		}) // TODO(phil): Add params
		if err != nil {
			return
		}
	case types.RuntimeAxolotl:
		s.Runtime, err = NewAxolotlRuntime(ctx, AxolotlRuntimeParams{
			RunnerOptions: params.RunnerOptions,
		}) // TODO(phil): Add params
		if err != nil {
			return
		}
	default:
		err = fmt.Errorf("unknown runtime: %s", params.Runtime)
		return
	}

	// Start the runtime
	err = s.Runtime.Start(ctx)
	if err != nil {
		return
	}

	// Create OpenAI Client
	openAIClient, err := CreateOpenaiClient(ctx, fmt.Sprintf("%s/v1", s.Runtime.URL()))
	if err != nil {
		return
	}
	// Check that the model is available in this runtime
	models, err := openAIClient.ListModels(ctx)
	if err != nil {
		return
	}
	found := false
	modelList := make([]string, 0, len(models.Models))
	for _, m := range models.Models {
		modelList = append(modelList, m.ID)
		if m.ID == params.Model {
			found = true
			break
		}
	}
	if !found {
		// TODO(phil): I disabled model pulling for now because it's more work. But it is there if
		// we need it
		err = fmt.Errorf("model %s not found, available models: %s", params.Model, strings.Join(modelList, ", "))
		return
	}

	// Warm up the model
	err = s.Runtime.Warm(ctx, params.Model)
	if err != nil {
		return
	}
	s.Active = false
	return
}
