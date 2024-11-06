package runner

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
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
				Model: string(model.Model_Ollama_Llama3_8b),
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
