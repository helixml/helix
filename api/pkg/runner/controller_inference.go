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

func (r *Runner) warmupInference(ctx context.Context) error {
	instance, err := NewOllamaInferenceModelInstance(
		r.Ctx,
		&InferenceModelInstanceConfig{
			ResponseHandler: func(res *types.RunnerTaskResponse) error {
				return nil
			},
			GetNextRequest: func() (*types.RunnerLLMInferenceRequest, error) {
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
	// TODO: warmup models

	var (
		request *types.RunnerLLMInferenceRequest
		err     error
	)

	if request == nil {
		// ask the api server if it currently has any work based on the amount of
		// memory we could free if we killed stale sessions
		request, err = r.getNextGlobalLLMInferenceRequest(ctx)
		if err != nil {
			return err
		}
	}

	if request != nil {
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

		log.Debug().
			Msgf("🔵 runner start model instance")
		err = r.createInferenceModelInstance(ctx, request)
		if err != nil {
			return err
		}
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
			ResponseHandler: r.handleWorkerResponse,
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
