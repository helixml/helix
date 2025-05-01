package runner

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/avast/retry-go/v4"
	"github.com/rs/zerolog/log"
)

func (r *Runner) startHelixModelReconciler(ctx context.Context) error {
	log.Info().Msg("starting helix model reconciler")
	defer log.Info().Msg("helix model reconciler stopped")

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

	defer runningRuntime.Stop() //nolint:errcheck

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
		if !slices.Contains(currentModels, model.ID) {
			log.Info().Str("model_id", model.ID).Msg("model to pull")
			modelsToPull = append(modelsToPull, model)
		}
	}

	for _, currentModel := range currentModels {
		// Already exists, set the status to downloaded
		log.Debug().Str("model_id", currentModel).Msg("existing model found")
		r.server.setHelixModelsStatus(&types.RunnerModelStatus{
			ModelID:            currentModel,
			Runtime:            types.RuntimeOllama,
			DownloadInProgress: false,
			DownloadPercent:    100,
		})
	}

	if len(modelsToPull) > 0 {
		log.Info().Int("models_to_pull", len(modelsToPull)).Msg("pulling models")
	}

	for _, model := range modelsToPull {
		log.Info().Str("model_id", model.ID).Msg("starting model pull")

		r.server.setHelixModelsStatus(&types.RunnerModelStatus{
			ModelID:            model.ID,
			Runtime:            types.RuntimeOllama,
			DownloadInProgress: true,
			DownloadPercent:    0,
		})

		err = runtime.PullModel(ctx, model.ID, func(progress PullProgress) error {
			log.Info().
				Str("model_id", model.ID).
				Int("progress_total", int(progress.Total)).
				Int("progress_completed", int(progress.Completed)).
				Str("progress_status", progress.Status).
				Msg("pulling model")

			r.server.setHelixModelsStatus(&types.RunnerModelStatus{
				ModelID:            model.ID,
				Runtime:            types.RuntimeOllama,
				DownloadInProgress: true,
				DownloadPercent:    getPercent(progress.Completed, progress.Total),
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
			// Continue to the next model
			continue
		}
		// Model pulled successfully, set the status to downloaded
		r.server.setHelixModelsStatus(&types.RunnerModelStatus{
			ModelID:            model.ID,
			Runtime:            types.RuntimeOllama,
			DownloadInProgress: false,
			DownloadPercent:    100,
		})
	}

	return nil
}

func getPercent(completed, total int64) int {
	if total == 0 {
		return 0
	}
	return int(completed * 100 / total)
}
