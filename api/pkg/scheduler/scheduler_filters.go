package scheduler

import (
	"fmt"
	"sort"

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
			var singleGPU *int
			var multiGPUs []int

			// For Ollama models, check if tensor parallelism would actually be used
			if work.Runtime() == types.RuntimeOllama {
				log.Info().
					Str("OLLAMA_TP_CHECK", "allocation_attempt").
					Str("runner_id", runnerID).
					Str("model", work.ModelName().String()).
					Uint64("model_memory_gb", work.model.Memory/(1024*1024*1024)).
					Msg("Ollama model - checking if tensor parallelism would be used")

				// First try single GPU allocation
				singleGPU, _, _ = s.controller.GetOptimalGPUAllocation(runnerID, work.model.Memory, work.Runtime())

				// If single GPU failed, check if Ollama would actually use tensor parallelism
				if singleGPU == nil {
					log.Info().
						Str("OLLAMA_TP_CHECK", "single_gpu_failed").
						Str("runner_id", runnerID).
						Str("model", work.ModelName().String()).
						Msg("Ollama single-GPU allocation failed, checking if tensor parallelism would be used")

					// Check if Ollama would actually allocate layers across multiple GPUs
					wouldUseTensorParallel, multiGPUs := s.wouldOllamaUseTensorParallel(runnerID, work.model.Memory)

					if wouldUseTensorParallel {
						log.Info().
							Str("OLLAMA_TP_CHECK", "would_use_tp").
							Str("runner_id", runnerID).
							Str("model", work.ModelName().String()).
							Ints("multi_gpus", multiGPUs).
							Msg("Ollama would use tensor parallelism - allowing multi-GPU allocation")

						// Allow multi-GPU allocation since Ollama would actually use TP
						runnerGPUCompatible[runnerID] = true
					} else {
						log.Info().
							Str("OLLAMA_TP_CHECK", "would_not_use_tp").
							Str("runner_id", runnerID).
							Str("model", work.ModelName().String()).
							Msg("Ollama would not use tensor parallelism - trying eviction")

						// Try with eviction since multi-GPU wouldn't help
						evictableMemoryPerGPU, err := s.calculateEvictableMemoryPerGPU(runnerID)
						if err == nil {
							singleGPU, _, _ = s.getOptimalGPUAllocationWithEviction(runnerID, work.model.Memory, work.Runtime(), evictableMemoryPerGPU)
						}
						runnerGPUCompatible[runnerID] = (singleGPU != nil)
					}
				} else {
					log.Info().
						Str("OLLAMA_TP_CHECK", "single_gpu_success").
						Str("runner_id", runnerID).
						Str("model", work.ModelName().String()).
						Interface("single_gpu", singleGPU).
						Msg("Ollama single-GPU allocation successful")
					runnerGPUCompatible[runnerID] = true
				}
			} else {
				// For VLLM models, use the original logic with multi-GPU support
				singleGPU, multiGPUs, _ = s.controller.GetOptimalGPUAllocation(runnerID, work.model.Memory, work.Runtime())

				log.Info().
					Str("ORANGE_EVICTION_START", "allocation_attempt").
					Str("runner_id", runnerID).
					Str("model", work.ModelName().String()).
					Str("runtime", string(work.Runtime())).
					Uint64("model_memory_gb", work.model.Memory/(1024*1024*1024)).
					Interface("standard_single_gpu", singleGPU).
					Ints("standard_multi_gpus", multiGPUs).
					Msg("ORANGE: VLLM initial GPU allocation attempt")

				// If allocation failed, try with eviction potential
				if singleGPU == nil && len(multiGPUs) == 0 {
					log.Info().
						Str("ORANGE_EVICTION_TRYING", "calculating_evictable").
						Str("runner_id", runnerID).
						Str("model", work.ModelName().String()).
						Uint64("model_memory_gb", work.model.Memory/(1024*1024*1024)).
						Msg("ORANGE: VLLM standard GPU allocation failed, trying with eviction potential")

					evictableMemoryPerGPU, err := s.calculateEvictableMemoryPerGPU(runnerID)
					log.Info().
						Str("ORANGE_EVICTION_CALCULATED", "evictable_memory").
						Str("runner_id", runnerID).
						Interface("evictable_memory_per_gpu_gb", func() map[int]uint64 {
							result := make(map[int]uint64)
							for gpu, mem := range evictableMemoryPerGPU {
								result[gpu] = mem / (1024 * 1024 * 1024)
							}
							return result
						}()).
						Err(err).
						Msg("ORANGE: Calculated evictable memory per GPU for VLLM")

					if err == nil {
						singleGPU, multiGPUs, _ = s.getOptimalGPUAllocationWithEviction(runnerID, work.model.Memory, work.Runtime(), evictableMemoryPerGPU)

						log.Info().
							Str("ORANGE_EVICTION_RESULT", "allocation_with_eviction").
							Str("runner_id", runnerID).
							Str("model", work.ModelName().String()).
							Interface("eviction_single_gpu", singleGPU).
							Ints("eviction_multi_gpus", multiGPUs).
							Interface("evictable_memory_per_gpu", evictableMemoryPerGPU).
							Msg("ORANGE: VLLM GPU allocation result with eviction potential")

						if singleGPU != nil || len(multiGPUs) > 0 {
							log.Info().
								Str("ORANGE_EVICTION_SUCCESS", "can_allocate").
								Str("runner_id", runnerID).
								Str("model", work.ModelName().String()).
								Interface("single_gpu", singleGPU).
								Ints("multi_gpus", multiGPUs).
								Interface("evictable_memory_per_gpu", evictableMemoryPerGPU).
								Msg("ORANGE: VLLM allocation successful with eviction potential - stale slots can be evicted")
						} else {
							log.Info().
								Str("ORANGE_EVICTION_FAILED", "cannot_allocate").
								Str("runner_id", runnerID).
								Str("model", work.ModelName().String()).
								Interface("evictable_memory_per_gpu", evictableMemoryPerGPU).
								Msg("ORANGE: VLLM allocation failed even with eviction potential")
						}
					} else {
						log.Info().
							Str("ORANGE_EVICTION_ERROR", "calc_failed").
							Str("runner_id", runnerID).
							Err(err).
							Msg("ORANGE: Failed to calculate evictable memory for VLLM")
					}
				}
				runnerGPUCompatible[runnerID] = (singleGPU != nil) || (len(multiGPUs) > 0)
			}

			log.Info().
				Str("ORANGE_FINAL_DECISION", "gpu_compatibility").
				Str("runner_id", runnerID).
				Str("model", work.ModelName().String()).
				Str("runtime", string(work.Runtime())).
				Interface("final_single_gpu", singleGPU).
				Ints("final_multi_gpus", multiGPUs).
				Bool("runner_gpu_compatible", runnerGPUCompatible[runnerID]).
				Msg("ORANGE: Final GPU compatibility decision")

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

// wouldOllamaUseTensorParallel simulates Ollama's layer allocation logic to determine
// if a model would actually be distributed across multiple GPUs, matching Ollama's
// createGPULayers and assignLayers functions.
//
// This prevents Helix from scheduling Ollama models on multiple GPUs when Ollama
// would not actually use tensor parallelism, matching Ollama's actual behavior.
func (s *Scheduler) wouldOllamaUseTensorParallel(runnerID string, modelMemoryRequirement uint64) (bool, []int) {
	status, err := s.controller.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("error getting runner status for Ollama TP check")
		return false, nil
	}

	if len(status.GPUs) < 2 {
		// Can't use tensor parallelism with less than 2 GPUs
		return false, nil
	}

	// Calculate allocated memory per GPU based on existing slots
	allocatedMemoryPerGPU, err := s.controller.calculateAllocatedMemoryPerGPU(runnerID)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("error calculating allocated memory per GPU for Ollama TP check")
		return false, nil
	}

	// Create a sorted list of GPUs by free memory (descending order, like Ollama does)
	type gpuInfo struct {
		index      int
		freeMemory uint64
	}

	var gpus []gpuInfo
	for _, gpu := range status.GPUs {
		allocatedMemory := allocatedMemoryPerGPU[gpu.Index]
		freeMemory := gpu.TotalMemory - allocatedMemory
		gpus = append(gpus, gpuInfo{
			index:      gpu.Index,
			freeMemory: freeMemory,
		})
	}

	// Sort by free memory descending (like Ollama's ByFreeMemory sort)
	sort.Slice(gpus, func(i, j int) bool {
		return gpus[i].freeMemory > gpus[j].freeMemory
	})

	// Simulate Ollama's greedy layer assignment algorithm
	// We'll use a simplified model: assume the model has layers that need to be distributed
	// and see if they would span multiple GPUs

	// Estimate layer count and size based on model memory
	// This is a simplified approximation of Ollama's layer calculation
	estimatedLayers := int(modelMemoryRequirement / (1024 * 1024 * 1024)) // Rough estimate: ~1GB per layer for large models
	if estimatedLayers < 10 {
		estimatedLayers = 10 // Minimum reasonable layer count
	}
	if estimatedLayers > 100 {
		estimatedLayers = 100 // Maximum reasonable layer count
	}

	layerSize := modelMemoryRequirement / uint64(estimatedLayers)

	// Simulate greedy assignment starting from the GPU with most free memory
	assignedGPUs := []int{}
	currentGPUIndex := 0
	layersAssigned := 0

	// Start with the first GPU if it has any free memory
	if len(gpus) > 0 && gpus[currentGPUIndex].freeMemory > 0 {
		remainingMemory := gpus[currentGPUIndex].freeMemory

		for layersAssigned < estimatedLayers && currentGPUIndex < len(gpus) {
			if remainingMemory >= layerSize {
				// Layer fits on current GPU
				remainingMemory -= layerSize
				layersAssigned++

				// Track which GPU we're using
				if len(assignedGPUs) == 0 || assignedGPUs[len(assignedGPUs)-1] != gpus[currentGPUIndex].index {
					assignedGPUs = append(assignedGPUs, gpus[currentGPUIndex].index)
				}
			} else {
				// Current GPU is full, move to next GPU
				currentGPUIndex++
				if currentGPUIndex < len(gpus) {
					remainingMemory = gpus[currentGPUIndex].freeMemory
					// Only add GPU if it has free memory
					if remainingMemory > 0 {
						assignedGPUs = append(assignedGPUs, gpus[currentGPUIndex].index)
					}
				}
			}
		}
	}

	// Ollama uses tensor parallelism if layers are assigned to multiple GPUs
	wouldUseTensorParallel := len(assignedGPUs) > 1

	log.Info().
		Str("runner_id", runnerID).
		Uint64("model_memory_gb", modelMemoryRequirement/(1024*1024*1024)).
		Int("estimated_layers", estimatedLayers).
		Uint64("layer_size_mb", layerSize/(1024*1024)).
		Int("layers_assigned", layersAssigned).
		Ints("assigned_gpus", assignedGPUs).
		Bool("would_use_tensor_parallel", wouldUseTensorParallel).
		Msg("Ollama tensor parallelism prediction")

	return wouldUseTensorParallel, assignedGPUs
}
