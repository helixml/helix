package runner

import (
	"context"
	"fmt"
	"net/url"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

// warmupInference downloads the model weights for the inference model,
// this function should be called when the runner is initialized
func (r *Runner) warmupInference(ctx context.Context) error {
	instance, err := NewOllamaInferenceModelInstance(
		r.Ctx,
		&InferenceModelInstanceConfig{
			ResponseHandler: func(res *types.RunnerLLMInferenceResponse) error {
				// No-op
				return nil
			},
			GetNextRequest: func() (*types.RunnerLLMInferenceRequest, error) {
				// No-op
				return nil, nil
			},
			RunnerOptions: r.Options,
		},
		&types.RunnerLLMInferenceRequest{
			Request: &openai.ChatCompletionRequest{
				Model: string(types.Model_Ollama_Llama3_8b),
			},
		},
	)
	if err != nil {
		return err
	}

	err = instance.Warmup(ctx)
	if err != nil {
		return fmt.Errorf("error warming up inference model instance: %s", err.Error())
	}

	return nil
}

func (r *Runner) pollInferenceRequests(ctx context.Context) error {
	// Query for the next global inference request
	request, err := r.getNextGlobalLLMInferenceRequest(ctx)
	if err != nil {
		return err
	}

	if request == nil {
		// Nothing to do
		return nil
	}

	modelName := types.ModelName(request.Request.Model)

	aiModel, err := model.GetModel(modelName)
	if err != nil {
		return fmt.Errorf("error getting model %s: %s", modelName, err.Error())
	}

	// if we need to kill any stale sessions, do it now
	// check for running model instances that have not seen a job in a while
	// and kill them if they are over the timeout AND the session requires it
	err = r.checkForStaleModelInstances(ctx, aiModel, types.SessionModeInference)
	if err != nil {
		return err
	}

	log.Info().
		Str("request_id", request.RequestID).
		Str("model_name", modelName.String()).
		Msgf("🔵 runner start model instance")

	err = r.createInferenceModelInstance(ctx, request)
	if err != nil {
		return fmt.Errorf("failed to create inference model instance: %w", err)
	}

	return nil
}

func (r *Runner) createInferenceModelInstance(ctx context.Context, request *types.RunnerLLMInferenceRequest) error {
	var (
		modelInstance ModelInstance
		err           error
	)

	log.Info().Msg("using LLM inference model instance")
	modelInstance, err = NewOllamaInferenceModelInstance(
		r.Ctx,
		&InferenceModelInstanceConfig{
			ResponseHandler: r.handleInferenceResponse,
			GetNextRequest: func() (*types.RunnerLLMInferenceRequest, error) {
				queryParams := url.Values{}

				queryParams.Add("model_name", string(modelInstance.Filter().ModelName))
				queryParams.Add("mode", string(modelInstance.Filter().Mode))

				nextRequest, err := r.getNextLLMInferenceRequest(ctx, queryParams)
				if err != nil {
					return nil, err
				}
				return nextRequest, nil
			},
			RunnerOptions: r.Options,
		},
		request,
	)
	if err != nil {
		return err
	}

	log.Info().
		Str("model_instance", modelInstance.Filter().ModelName.String()).
		Msgf("🔵 runner started inference model instance: %s", modelInstance.ID())

	r.activeModelInstances.Store(modelInstance.ID(), modelInstance)

	err = modelInstance.Start(ctx)
	if err != nil {
		return err
	}

	go func() {
		<-modelInstance.Done()
		log.Debug().
			Msgf("🔵 runner stop model instance: %s", modelInstance.ID())
		r.activeModelInstances.Delete(modelInstance.ID())
	}()

	return nil
}
