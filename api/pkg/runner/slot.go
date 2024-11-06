package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type SlotFactory interface {
	NewSlot(ctx context.Context,
		slotID uuid.UUID,
		work *scheduler.Workload,
		inferenceResponseHandler func(res *types.RunnerLLMInferenceResponse) error,
		sessionResponseHandler func(res *types.RunnerTaskResponse) error,
		runnerOptions RunnerOptions,
	) (*Slot, error)
}

// Slot is the crazy mirror equivalent of scheduler.Slot
// You can think of it as the same thing as a Slot, but it's a bit fatter because it ecapsulates all
// the horrible logic involved with starting and destroying a ModelInstance.
// E.g. axolotl expects a session, whereas ollama expects an LLMInferenceRequest.
type Slot struct {
	ID              uuid.UUID           // Same as scheduler.Slot
	RunnerID        string              // Same as scheduler.Slot
	originalWork    *scheduler.Workload // The original work that was assigned to this slot
	modelInstance   ModelInstance
	llmWorkChan     chan *types.RunnerLLMInferenceRequest
	sessionWorkChan chan *types.Session
	currentWork     *scheduler.Workload
}

func (r *Slot) Stop() {
	err := r.modelInstance.Stop()
	if err != nil {
		log.Err(err).Msg("error stopping model instance")
	}
	if r.llmWorkChan != nil {
		close(r.llmWorkChan)
	}
	if r.sessionWorkChan != nil {
		close(r.sessionWorkChan)
	}
}

func (r *Slot) CurrentWorkload() *types.RunnerWorkload {
	if r.currentWork == nil {
		return &types.RunnerWorkload{}
	}
	return r.currentWork.ToRunnerWorkload()
}

func (r *Slot) OriginalWorkload() *types.RunnerWorkload {
	if r.originalWork == nil {
		return &types.RunnerWorkload{}
	}
	return r.originalWork.ToRunnerWorkload()
}

func (r *Slot) SetLLMInferenceRequest(work *scheduler.Workload) {
	r.currentWork = work
	r.llmWorkChan <- work.LLMInferenceRequest()
}

func (r *Slot) SetSessionRequest(work *scheduler.Workload) {
	r.currentWork = work
	r.sessionWorkChan <- work.Session()
}

func (r *Slot) IsScheduled() bool {
	return len(r.sessionWorkChan) > 0 || len(r.llmWorkChan) > 0
}

var _ SlotFactory = &runtimeFactory{}

type runtimeFactory struct{}

func (f *runtimeFactory) NewSlot(ctx context.Context,
	slotID uuid.UUID,
	work *scheduler.Workload,
	// TODO(PHIL): Merge these response handlers
	// TODO(PHIL): Also the slot doesn't know when the work has finished.
	inferenceResponseHandler func(res *types.RunnerLLMInferenceResponse) error,
	sessionResponseHandler func(res *types.RunnerTaskResponse) error,
	runnerOptions RunnerOptions,
) (*Slot, error) {
	slot := &Slot{
		ID:           slotID,
		RunnerID:     runnerOptions.ID,
		originalWork: work,
		currentWork:  work,
	}
	switch work.WorkloadType {
	case scheduler.WorkloadTypeLLMInferenceRequest:
		log.Debug().Str("workload_id", work.ID()).Msg("starting new ollama runtime")
		workCh := make(chan *types.RunnerLLMInferenceRequest, 1)
		ollama, err := NewOllamaInferenceModelInstance(
			ctx,
			&InferenceModelInstanceConfig{
				ResponseHandler: inferenceResponseHandler,
				GetNextRequest: func() (*types.RunnerLLMInferenceRequest, error) {
					return <-workCh, nil
				},
				RunnerOptions: runnerOptions,
			},
			work.LLMInferenceRequest(),
		)
		if err != nil {
			return nil, fmt.Errorf("error creating ollama runtime: %s", err.Error())
		}
		err = ollama.Start(ctx)
		if err != nil {
			return nil, fmt.Errorf("error starting ollama runtime: %s", err.Error())
		}
		slot.modelInstance = ollama
		slot.llmWorkChan = workCh
		return slot, nil
	case scheduler.WorkloadTypeSession:
		log.Debug().Str("workload_id", work.ID()).Msg("starting new session runtime")
		var (
			modelInstance ModelInstance
			err           error
		)
		workCh := make(chan *types.Session, 1)
		initialSession := work.Session()
		runtimeName := model.ModelName(initialSession.ModelName).InferenceRuntime()

		// if we are in mock mode - we need the axolotl model instance because
		// it understands how to do a mock runner
		if runnerOptions.MockRunner {
			if initialSession.Type == types.SessionTypeText {
				runtimeName = types.InferenceRuntimeAxolotl
				initialSession.ModelName = string(model.Model_Axolotl_Mistral7b)
			} else if initialSession.Type == types.SessionTypeImage {
				// I know - this looks odd, but "InferenceRuntimeAxolotl" should actually be called
				// "InferenceRuntimeDefault" - i.e. it's the original "run a python program" version
				// that does both axolotl and sdxl
				runtimeName = types.InferenceRuntimeAxolotl
				initialSession.ModelName = string(model.Model_Cog_SDXL)
			}
		}

		switch runtimeName {
		case types.InferenceRuntimeOllama:
			log.Debug().Str("workload_id", work.ID()).Msg("starting new ollama session runtime")
			modelInstance, err = NewOllamaModelInstance(
				ctx,
				&ModelInstanceConfig{
					InitialSession:  initialSession,
					ResponseHandler: sessionResponseHandler,
					GetNextSession: func() (*types.Session, error) {
						return <-workCh, nil
					},
					RunnerOptions: runnerOptions,
				},
			)
			if err != nil {
				return nil, err
			}
			err = modelInstance.Start(ctx)
			if err != nil {
				return nil, err
			}
			slot.modelInstance = modelInstance
			slot.sessionWorkChan = workCh
			return slot, nil
		default:
			// Defaulting to axolotl
			log.Debug().Str("workload_id", work.ID()).Msg("starting new axolotl session runtime")
			modelInstance, err = NewAxolotlModelInstance(
				ctx,
				&ModelInstanceConfig{
					InitialSession:    initialSession,
					InitialSessionURL: runnerOptions.InitialSessionURL,
					NextTaskURL:       runnerOptions.TaskURL,
					// this function will convert any files it sees locally into an upload
					// to the api server filestore - all files will be written to the filestore
					// under a session sub path - you can include tar files and they will untarred at the other end
					// into the filestore
					// TODO: support the tar feature above
					ResponseHandler: sessionResponseHandler,
					RunnerOptions:   runnerOptions,
					GetNextSession: func() (*types.Session, error) {
						return <-workCh, nil
					},
				},
			)
			if err != nil {
				return nil, err
			}

			go modelInstance.QueueSession(initialSession, true)

			err = modelInstance.Start(ctx)
			if err != nil {
				return nil, err
			}
			slot.modelInstance = modelInstance
			slot.sessionWorkChan = workCh
			return slot, nil
		}
	default:
		return nil, fmt.Errorf("unknown workload type: %s", work.WorkloadType)
	}
}
