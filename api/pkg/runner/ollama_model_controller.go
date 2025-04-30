package runner

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/sourcegraph/conc/pool"

	"github.com/avast/retry-go/v4"
	"github.com/rs/zerolog/log"
)

func (r *Runner) startHelixModelReconciler(ctx context.Context) error {

	// Create ollama runtime, we will use it to pull/list models
	runtimeParams := OllamaRuntimeParams{
		CacheDir: &r.Options.CacheDir,
	}
	runningRuntime, err := NewOllamaRuntime(ctx, runtimeParams)
	if err != nil {
		return fmt.Errorf("error creating ollama runtime: %w", err)
	}
	// Start ollama runtime
	err = runningRuntime.Start(ctx)
	if err != nil {
		return fmt.Errorf("error starting ollama runtime: %w", err)
	}

	defer runningRuntime.Stop()

	for {
		err := r.reconcileHelixModels(ctx, runningRuntime)
		if err != nil {
			log.Error().Err(err).Msg("error reconciling helix models")
		}

		select {
		case <-time.After(time.Second * 5):
			continue
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while reconciling helix models")
		}
	}
}

func (r *Runner) reconcileHelixModels(ctx context.Context, ollamaRuntime Runtime) error {
	log.Info().Msg("reconciling helix models")

	err := r.reconcileOllamaHelixModels(ctx, ollamaRuntime)
	if err != nil {
		log.Error().Err(err).Msg("error reconciling ollama models")
	}

	return nil
}

func (r *Runner) reconcileOllamaHelixModels(ctx context.Context, runtime Runtime) error {
	models := r.server.listHelixModels()

	var ollamaModels []*types.Model

	// Filter ollama models
	for _, model := range models {
		if model.Runtime != types.RuntimeOllama {
			continue
		}
		if !model.Enabled {
			continue
		}
		// If model requires more memory than we have, skip it
		if model.Memory > r.Options.MemoryBytes {
			continue
		}

		ollamaModels = append(ollamaModels, model)
	}

	log.Info().Any("models", models).Int("ollama_models_count", len(ollamaModels)).Msg("reconciling ollama models")

	// List models from ollama
	currentModels, err := retry.DoWithData(func() ([]string, error) {
		return runtime.ListModels(ctx)
	}, retry.Attempts(3), retry.Delay(time.Second*5))
	if err != nil {
		return fmt.Errorf("error listing ollama models: %w", err)
	}

	var modelsToPull []*types.Model

	// Compare models
	for _, model := range ollamaModels {
		if !slices.Contains(currentModels, model.Name) {
			log.Info().Msgf("model to pull %s", model.ID)
			modelsToPull = append(modelsToPull, model)
		}
	}

	for _, currentModel := range currentModels {
		// Already exists, set the status to downloaded
		r.server.setHelixModelsStatus(&types.RunnerModelStatus{
			ModelID:            currentModel,
			Runtime:            types.RuntimeOllama,
			DownloadInProgress: false,
			DownloadPercent:    100,
		})
	}

	// Pull models, if any
	pool := pool.New().WithMaxGoroutines(r.Options.MaxPullConcurrency)

	for _, model := range modelsToPull {
		pool.Go(func() {
			r.server.setHelixModelsStatus(&types.RunnerModelStatus{
				ModelID:            model.ID,
				Runtime:            types.RuntimeOllama,
				DownloadInProgress: true,
				DownloadPercent:    0,
			})

			err = runtime.PullModel(ctx, model.ID, func(progress PullProgress) error {
				log.Info().Msgf("pulling model %s: %d/%d", model.ID, progress.Completed, progress.Total)

				r.server.setHelixModelsStatus(&types.RunnerModelStatus{
					ModelID:            model.ID,
					Runtime:            types.RuntimeOllama,
					DownloadInProgress: true,
					DownloadPercent:    int(progress.Completed * 100 / progress.Total),
				})

				return nil
			})
			if err != nil {
				log.Error().Err(err).Str("model_id", model.ID).Msg("error pulling model")
				r.server.setHelixModelsStatus(&types.RunnerModelStatus{
					ModelID:            model.ID,
					Runtime:            types.RuntimeOllama,
					DownloadInProgress: false,
					DownloadPercent:    100,
					Error:              err.Error(),
				})
				return
			}
			// Model pulled successfully, set the status to downloaded
			r.server.setHelixModelsStatus(&types.RunnerModelStatus{
				ModelID:            model.ID,
				Runtime:            types.RuntimeOllama,
				DownloadInProgress: false,
				DownloadPercent:    100,
			})
		})
	}
	pool.Wait()

	return nil
}
