package runner

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
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

	// Create a special reconciler slot ID for process tracking
	reconcilerSlotID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	// Register the reconciler's Ollama runtime with the process tracker to prevent orphan cleanup
	if r.server != nil {
		runningRuntime.SetProcessTracker(r.server.processTracker, reconcilerSlotID)
		log.Info().Msg("PROCESS_TRACKER: Set up process tracking for reconciler Ollama runtime")
	}

	// Start ollama runtime
	err = runningRuntime.Start(ctx)
	if err != nil {
		return fmt.Errorf("error starting ollama runtime: %w", err)
	}

	defer func() {
		// Clean up process tracker registration before stopping runtime
		if r.server != nil {
			r.server.processTracker.UnregisterSlot(reconcilerSlotID)
			log.Info().Msg("PROCESS_TRACKER: Cleaned up reconciler Ollama runtime registration")
		}
		runningRuntime.Stop() //nolint:errcheck
	}()

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

	err := r.reconcileOllamaHelixModels(ctx, ollamaRuntime)
	if err != nil {
		log.Error().Err(err).Msg("error reconciling ollama models")
	}

	return nil
}

func (r *Runner) reconcileOllamaHelixModels(ctx context.Context, runtime Runtime) error {
	log.Info().Msg("reconciling ollama models")

	models := r.server.listHelixModels()

	// Create a map of model ID to memory for quick lookup
	modelMemoryMap := make(map[string]uint64)
	for _, model := range models {
		modelMemoryMap[model.ID] = model.Memory
	}

	var ollamaModels []*types.Model

	// Filter ollama models
	for _, model := range models {
		if model.Runtime != types.RuntimeOllama {
			continue
		}
		if !model.AutoPull {
			log.Debug().Str("model_id", model.ID).Msg("model auto pull disabled")
			continue
		}
		// If model requires more memory than we have, skip it
		if model.Memory > r.Options.MemoryBytes {
			log.Debug().Str("model_id", model.ID).Msg("model memory limit exceeded")
			continue
		}

		ollamaModels = append(ollamaModels, model)
	}

	log.Debug().Any("models", models).Int("ollama_models_count", len(ollamaModels)).Msg("reconciling ollama models")

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
		memory := modelMemoryMap[currentModel]
		r.server.setHelixModelsStatus(&types.RunnerModelStatus{
			ModelID:            currentModel,
			Runtime:            types.RuntimeOllama,
			DownloadInProgress: false,
			DownloadPercent:    100,
			Memory:             memory,
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
			Memory:             model.Memory,
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
				Memory:             model.Memory,
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
				Memory:             model.Memory,
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
			Memory:             model.Memory,
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
