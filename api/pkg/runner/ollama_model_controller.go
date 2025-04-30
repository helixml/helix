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
	for {
		err := r.reconcileHelixModels(ctx)
		if err != nil {
			log.Error().Err(err).Msg("error reconciling helix models")
		}

		select {
		case <-time.After(time.Second * 30):
			continue
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while reconciling helix models")
		}
	}
}

func (r *Runner) reconcileHelixModels(ctx context.Context) error {
	err := r.reconcileOllamaHelixModels(ctx)
	if err != nil {
		log.Error().Err(err).Msg("error reconciling ollama models")
	}

	return nil
}

func (r *Runner) reconcileOllamaHelixModels(ctx context.Context) error {
	models := r.server.listHelixModels()

	var ollamaModels []*types.Model

	// Filter ollama models
	for _, model := range models {
		if model.Runtime != types.RuntimeOllama {
			continue
		}
		ollamaModels = append(ollamaModels, model)
	}

	log.Info().Int("ollama_models_count", len(ollamaModels)).Msg("reconciling ollama models")

	// Create ollama runtime
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

	// List models from ollama
	currentModels, err := retry.DoWithData(func() ([]string, error) {
		return runningRuntime.ListModels(ctx)
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

	// Pull models
	pool := pool.New().WithMaxGoroutines(r.Options.MaxPullConcurrency)
	for _, model := range modelsToPull {
		pool.Go(func() {
			err = runningRuntime.PullModel(ctx, model.ID, func(progress PullProgress) error {
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
