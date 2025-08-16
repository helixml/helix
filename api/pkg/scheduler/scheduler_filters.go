package scheduler

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// filterRunners - filter runners by available memory, models.
func (s *Scheduler) filterRunners(work *Workload, runnerIDs []string) ([]string, error) {
	filteredRunners, err := s.filterRunnersByMemory(work, runnerIDs)
	if err != nil {
		return nil, err
	}

	filteredRunners, err = s.filterRunnersByModel(work, filteredRunners)
	if err != nil {
		return nil, err
	}

	return filteredRunners, nil
}

func (s *Scheduler) filterRunnersByMemory(work *Workload, runnerIDs []string) ([]string, error) {
	log.Trace().
		Strs("runner_ids", runnerIDs).
		Str("model", work.ModelName().String()).
		Uint64("model_memory", work.model.Memory).
		Msg("starting runner memory filtering")

	if len(runnerIDs) == 0 {
		return nil, fmt.Errorf("no runners available")
	}

	var filteredRunners []string

	runnerMemory := make(map[string]uint64)
	runnerGPUCompatible := make(map[string]bool)
	for _, runnerID := range runnerIDs {
		runnerMemory[runnerID] = s.controller.TotalMemory(runnerID)

		log.Debug().
			Str("runner_id", runnerID).
			Uint64("total_memory", runnerMemory[runnerID]).
			Str("model", work.ModelName().String()).
			Uint64("model_memory", work.model.Memory).
			Msg("checking runner memory")

		// Check GPU compatibility based on runtime and memory requirements
		if work.Runtime() == types.RuntimeVLLM || work.Runtime() == types.RuntimeOllama {
			// First try standard allocation with current memory state
			singleGPU, multiGPUs, _ := s.controller.GetOptimalGPUAllocation(runnerID, work.model.Memory)

			// If allocation failed, try with eviction potential
			if singleGPU == nil && len(multiGPUs) == 0 {
				log.Debug().
					Str("runner_id", runnerID).
					Str("model", work.ModelName().String()).
					Uint64("model_memory_gb", work.model.Memory/(1024*1024*1024)).
					Msg("Standard GPU allocation failed, trying with eviction potential")

				evictableMemoryPerGPU, err := s.calculateEvictableMemoryPerGPU(runnerID)
				if err == nil {
					singleGPU, multiGPUs, _ = s.getOptimalGPUAllocationWithEviction(runnerID, work.model.Memory, evictableMemoryPerGPU)

					if singleGPU != nil || len(multiGPUs) > 0 {
						log.Info().
							Str("runner_id", runnerID).
							Str("model", work.ModelName().String()).
							Interface("single_gpu", singleGPU).
							Ints("multi_gpus", multiGPUs).
							Interface("evictable_memory_per_gpu", evictableMemoryPerGPU).
							Msg("GPU allocation successful with eviction potential - stale slots can be evicted")
					}
				}
			}

			// TEMPORARY: Disable multi-GPU for Ollama due to timeout issues
			if work.Runtime() == types.RuntimeOllama && singleGPU == nil && len(multiGPUs) > 0 {
				log.Info().
					Str("runner_id", runnerID).
					Str("model", work.ModelName().String()).
					Ints("rejected_multi_gpus", multiGPUs).
					Uint64("model_memory_gb", work.model.Memory/(1024*1024*1024)).
					Msg("TEMPORARY: Disabling multi-GPU allocation for Ollama model due to timeout issues - rejecting model that needs multi-GPU")
				// Force rejection for Ollama models that need multi-GPU
				runnerGPUCompatible[runnerID] = false
				multiGPUs = nil // Clear multi-GPU allocation
			} else {
				runnerGPUCompatible[runnerID] = (singleGPU != nil) || (len(multiGPUs) > 0)
			}

			log.Trace().
				Str("runner_id", runnerID).
				Str("runtime", string(work.Runtime())).
				Interface("single_gpu", singleGPU).
				Ints("multi_gpus", multiGPUs).
				Bool("gpu_compatible", runnerGPUCompatible[runnerID]).
				Msg("GPU allocation check with pending allocations")

			// Store the allocation decision for this workload-runner combination
			if runnerGPUCompatible[runnerID] {
				s.storeGPUAllocation(work, runnerID, singleGPU, multiGPUs)
			}
		} else {
			// For other runtimes, use traditional total memory check
			runnerGPUCompatible[runnerID] = runnerMemory[runnerID] >= work.model.Memory
		}

		// log.Info().
		//	Str("runner_id", runnerID).
		//	Str("runtime", string(work.Runtime())).
		//	Uint64("runner_memory", runnerMemory[runnerID]).
		//	Uint64("model_memory", work.model.Memory).
		//	Bool("memory_compatible", runnerGPUCompatible[runnerID]).
		//	Msg("SLOT_RECONCILE_DEBUG: Traditional memory check")
	}

	numRunnersWithNotEnoughTotalMemory := 0
	numRunnersWithGPUFragmentation := 0
	largestRunnerMemory := uint64(0)
	requiredMemory := work.model.Memory

	for runnerID, memory := range runnerMemory {
		// Check both total memory and GPU compatibility
		hasEnoughTotalMemory := memory >= requiredMemory
		hasGPUCompatibility := runnerGPUCompatible[runnerID]

		if hasEnoughTotalMemory && hasGPUCompatibility {
			filteredRunners = append(filteredRunners, runnerID)
		} else {
			if !hasEnoughTotalMemory {
				numRunnersWithNotEnoughTotalMemory++
			} else if !hasGPUCompatibility {
				numRunnersWithGPUFragmentation++
				withWorkContext(&log.Logger, work).Debug().
					Str("runner_id", runnerID).
					Uint64("total_memory", memory).
					Uint64("required_memory", requiredMemory).
					Msg("Runner has enough total memory but model cannot fit on available GPU(s)")
			}
		}
		if memory > largestRunnerMemory {
			largestRunnerMemory = memory
		}
	}

	withWorkContext(&log.Logger, work).Trace().
		Interface("filtered_runners", filteredRunners).
		Int("total_memory_failures", numRunnersWithNotEnoughTotalMemory).
		Int("gpu_fragmentation_failures", numRunnersWithGPUFragmentation).
		Msg("filtered runners with GPU-aware memory checking")

	if len(filteredRunners) == 0 {
		if numRunnersWithGPUFragmentation > 0 {
			return nil, fmt.Errorf("no runner can fit model on available GPU(s) - tried single and multi-GPU allocation (desired: %d, largest total: %d): %w", requiredMemory, largestRunnerMemory, ErrModelWontFit)
		}
		return nil, fmt.Errorf("no runner has enough GPU memory for this workload (desired: %d, largest: %d): %w", requiredMemory, largestRunnerMemory, ErrModelWontFit)
	}

	return filteredRunners, nil
}

func (s *Scheduler) filterRunnersByModel(work *Workload, runnerIDs []string) ([]string, error) {
	// Currently filtering only for ollama models (dynamic)
	// TODO: add support for pulling and reporting vllm, diffusers, etc.
	if work.model.Runtime != types.RuntimeOllama {
		return runnerIDs, nil
	}

	if len(runnerIDs) == 0 {
		return nil, fmt.Errorf("no runners available")
	}

	var filteredRunners []string

	for _, runnerID := range runnerIDs {
		status, err := s.controller.GetStatus(runnerID)
		if err != nil {
			return nil, fmt.Errorf("failed to get runner status: %w", err)
		}

		for _, modelStatus := range status.Models {
			if !modelStatus.DownloadInProgress && modelStatus.ModelID == work.model.ID {
				filteredRunners = append(filteredRunners, runnerID)
			}
		}
	}

	if len(filteredRunners) == 0 {
		return nil, fmt.Errorf("no runner has the model %s", work.model.ID)
	}

	return filteredRunners, nil
}
