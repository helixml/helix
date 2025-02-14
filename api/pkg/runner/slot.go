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
	ID              uuid.UUID // Same as scheduler.Slot
	RunnerID        string    // Same as scheduler.Slot
	Model           string    // The model assigned to this slot
	IntendedRuntime types.Runtime
	Active          bool // True if the slot is active
	Ready           bool // True if the slot is ready to be used
	runnerOptions   *Options
	runningRuntime  Runtime
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
	Status(ctx context.Context) string // To hold general status information like ollama ps output
	Runtime() types.Runtime
	URL() string
}

type CreateSlotParams struct {
	RunnerOptions *Options
	ID            uuid.UUID
	Runtime       types.Runtime
	Model         string
}

func NewEmptySlot(params CreateSlotParams) *Slot {
	return &Slot{
		ID:              params.ID,
		RunnerID:        params.RunnerOptions.ID,
		Model:           params.Model,
		IntendedRuntime: params.Runtime,
		Active:          false,
		Ready:           false,
		runnerOptions:   params.RunnerOptions,
		runningRuntime:  nil, // This is set during creation
	}
}

// If there is an error at any point during creation, we call Stop to kill the runtime. Otherwise it
// can just sit there taking up GPU and doing nothing.
func (s *Slot) Create(ctx context.Context) (err error) {
	// Need to be very careful to shutdown the runtime if there is an error!
	// Safest to do this in a defer so that it always checks.
	defer func() {
		if err != nil {
			if s.runningRuntime != nil {
				log.Warn().Str("model", s.Model).Interface("runtime", s.IntendedRuntime).Msg("error creating slot, stopping runtime")
				stopErr := s.runningRuntime.Stop()
				if stopErr != nil {
					log.Error().Err(stopErr).Str("model", s.Model).Interface("runtime", s.IntendedRuntime).Msg("error stopping runtime, possible memory leak")
				}
			}
		}
	}()

	switch s.IntendedRuntime {
	case types.RuntimeOllama:
		s.runningRuntime, err = NewOllamaRuntime(ctx, OllamaRuntimeParams{
			CacheDir: &s.runnerOptions.CacheDir,
		}) // TODO(phil): Add params
		if err != nil {
			return
		}
	case types.RuntimeDiffusers:
		s.runningRuntime, err = NewDiffusersRuntime(ctx, DiffusersRuntimeParams{
			CacheDir: &s.runnerOptions.CacheDir,
		}) // TODO(phil): Add params
		if err != nil {
			return
		}
	case types.RuntimeAxolotl:
		s.runningRuntime, err = NewAxolotlRuntime(ctx, AxolotlRuntimeParams{
			RunnerOptions: s.runnerOptions,
		}) // TODO(phil): Add params
		if err != nil {
			return
		}
	default:
		err = fmt.Errorf("unknown runtime: %s", s.IntendedRuntime)
		return
	}

	// Start the runtime
	err = s.runningRuntime.Start(ctx)
	if err != nil {
		return
	}

	// Create OpenAI Client
	openAIClient, err := CreateOpenaiClient(ctx, fmt.Sprintf("%s/v1", s.runningRuntime.URL()))
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
		if m.ID == s.Model {
			found = true
			break
		}
	}
	if !found {
		// TODO(phil): I disabled model pulling for now because it's more work. But it is there if
		// we need it
		err = fmt.Errorf("model %s not found, available models: %s", s.Model, strings.Join(modelList, ", "))
		return
	}

	s.Active = true
	// Warm up the model
	err = s.runningRuntime.Warm(ctx, s.Model)
	if err != nil {
		return
	}
	s.Active = false
	s.Ready = true
	return
}

func (s *Slot) Delete() error {
	if s.runningRuntime != nil {
		return s.runningRuntime.Stop()
	}
	return nil
}

func (s *Slot) Version() string {
	if s.runningRuntime != nil {
		return s.runningRuntime.Version()
	}
	return "unknown"
}

func (s *Slot) Runtime() types.Runtime {
	if s.runningRuntime != nil {
		return s.runningRuntime.Runtime()
	}
	return types.Runtime("unknown")
}

func (s *Slot) Status(ctx context.Context) string {
	if s.runningRuntime != nil {
		return s.runningRuntime.Status(ctx)
	}
	return "unknown"
}

func (s *Slot) URL() string {
	if s.runningRuntime != nil {
		return s.runningRuntime.URL()
	}
	return ""
}
