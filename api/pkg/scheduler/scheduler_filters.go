package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/helixml/helix/api/pkg/memory"
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

// getEffectiveMemoryRequirement gets the memory requirement for a model, preferring GGUF-based estimates for Ollama models
func (s *Scheduler) getEffectiveMemoryRequirement(ctx context.Context, work *Workload, runnerID string, numGPUs int) (uint64, bool) {
	// For Ollama models, try to get GGUF-based memory estimate
	if work.Runtime() == types.RuntimeOllama && s.memoryEstimationService != nil {
		log.Debug().
			Str("runner_id", runnerID).
			Str("model", work.ModelName().String()).
			Int("num_gpus", numGPUs).
			Msg("using standard GPU config for GGUF memory estimation")

		// Get memory estimate with model's context length and concurrency
		var numParallel int
		if work.model.Concurrency > 0 {
			numParallel = work.model.Concurrency
		} else if work.Runtime() == types.RuntimeVLLM {
			numParallel = types.DefaultVLLMParallelSequences
		} else if work.Runtime() == types.RuntimeOllama {
			numParallel = types.DefaultOllamaParallelSequences
		} else {
			numParallel = types.DefaultParallelSequences
		}

		opts := memory.EstimateOptions{
			NumCtx:      int(work.model.ContextLength),
			NumBatch:    types.DefaultBatchSize,
			NumParallel: numParallel,
			NumGPU:      types.AutoDetectLayers,
			KVCacheType: types.DefaultKVCacheType,
		}

		result, err := s.memoryEstimationService.EstimateModelMemory(ctx, work.model.ID, opts)
		if err != nil {
			log.Warn().
				Err(err).
				Str("runner_id", runnerID).
				Str("model", work.ModelName().String()).
				Msg("failed to get GGUF memory estimate - will skip scheduling this Ollama model until estimate is available")
			// Return 0 to indicate this model should be skipped
			return 0, false
		}

		// Select appropriate estimate based on GPU count
		var estimate *memory.MemoryEstimate
		if numGPUs <= 1 && result.SingleGPU != nil {
			estimate = result.SingleGPU
		} else if numGPUs > 1 && result.TensorParallel != nil {
			estimate = result.TensorParallel
		} else if result.SingleGPU != nil {
			estimate = result.SingleGPU
		} else {
			log.Debug().
				Str("runner_id", runnerID).
				Str("model", work.ModelName().String()).
				Int("num_gpus", numGPUs).
				Msg("no suitable GGUF estimate found - will skip scheduling this Ollama model")
			return 0, false
		}

		log.Info().
			Str("GGUF_MEMORY_ESTIMATE", "used").
			Str("runner_id", runnerID).
			Str("model", work.ModelName().String()).
			Int("num_gpus", numGPUs).
			Uint64("gguf_memory_gb", estimate.TotalSize/(1024*1024*1024)).
			Uint64("hardcoded_memory_gb", work.model.Memory/(1024*1024*1024)).
			Msg("using GGUF-based memory estimate instead of hardcoded value")

		return estimate.TotalSize, true
	}

	// For non-Ollama models or when GGUF estimation is not available, use hardcoded value
	return work.model.Memory, false
}

func (s *Scheduler) filterRunnersByMemory(work *Workload, runnerIDs []string) ([]string, error) {
	ctx := s.ctx // Use scheduler's context
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
					Str("OLLAMA_GGUF_CHECK", "allocation_attempt").
					Str("runner_id", runnerID).
					Str("model", work.ModelName().String()).
					Uint64("hardcoded_memory_gb", work.model.Memory/(1024*1024*1024)).
					Msg("Ollama model - using GGUF-based memory estimation")

				// Get GGUF-based memory estimate for single GPU
				singleGPUMemory, usedGGUF := s.getEffectiveMemoryRequirement(ctx, work, runnerID, 1)
				if singleGPUMemory == 0 {
					// Skip this runner - no GGUF estimate available
					log.Info().
						Str("runner_id", runnerID).
						Str("model", work.ModelName().String()).
						Msg("Skipping runner - no GGUF memory estimate available for Ollama model")
					continue
				}
				effectiveMemoryRequirement := singleGPUMemory

				// Check if Ollama would use tensor parallelism for this model
				wouldUseTensorParallel, multiGPUs := s.wouldOllamaUseTensorParallel(runnerID, singleGPUMemory)
				if wouldUseTensorParallel && len(multiGPUs) > 1 {
					// Get GGUF-based memory estimate for multi-GPU
					multiGPUMemory, _ := s.getEffectiveMemoryRequirement(ctx, work, runnerID, len(multiGPUs))
					effectiveMemoryRequirement = multiGPUMemory

					log.Info().
						Str("OLLAMA_GGUF_TP", "gguf_estimates_used").
						Str("runner_id", runnerID).
						Str("model", work.ModelName().String()).
						Uint64("single_gpu_memory_gb", singleGPUMemory/(1024*1024*1024)).
						Uint64("multi_gpu_memory_gb", multiGPUMemory/(1024*1024*1024)).
						Uint64("hardcoded_memory_gb", work.model.Memory/(1024*1024*1024)).
						Int("tensor_parallel_gpus", len(multiGPUs)).
						Ints("assigned_gpus", multiGPUs).
						Bool("used_gguf", usedGGUF).
						Msg("Using GGUF-based memory estimates for tensor parallelism")
				}

				// First try single GPU allocation with effective memory requirement
				singleGPU, _, _ = s.controller.GetOptimalGPUAllocation(runnerID, effectiveMemoryRequirement, work.Runtime())

				// If single GPU failed, check if Ollama would actually use tensor parallelism
				if singleGPU == nil {
					log.Info().
						Str("OLLAMA_TP_CHECK", "single_gpu_failed").
						Str("runner_id", runnerID).
						Str("model", work.ModelName().String()).
						Msg("Ollama single-GPU allocation failed, checking if tensor parallelism would be used")

					if wouldUseTensorParallel {
						log.Info().
							Str("OLLAMA_TP_DEBUG", "tp_approved_multi_gpu").
							Str("runner_id", runnerID).
							Str("model", work.ModelName().String()).
							Uint64("effective_memory_gb", effectiveMemoryRequirement/(1024*1024*1024)).
							Ints("assigned_gpus", multiGPUs).
							Msg("OLLAMA_TP_DEBUG: TP prediction approved - allowing multi-GPU allocation")

						// Allow multi-GPU allocation since Ollama would actually use TP
						runnerGPUCompatible[runnerID] = true
					} else {
						log.Info().
							Str("OLLAMA_TP_DEBUG", "tp_rejected_trying_eviction").
							Str("runner_id", runnerID).
							Str("model", work.ModelName().String()).
							Uint64("effective_memory_gb", effectiveMemoryRequirement/(1024*1024*1024)).
							Msg("OLLAMA_TP_DEBUG: TP prediction rejected - attempting eviction fallback")

						// Try with eviction since multi-GPU wouldn't help
						evictableMemoryPerGPU, err := s.calculateEvictableMemoryPerGPU(runnerID)
						if err != nil {
							log.Error().
								Str("OLLAMA_TP_DEBUG", "eviction_calculation_failed").
								Str("runner_id", runnerID).
								Str("model", work.ModelName().String()).
								Err(err).
								Msg("OLLAMA_TP_DEBUG: Failed to calculate evictable memory")
						} else {
							log.Info().
								Str("OLLAMA_TP_DEBUG", "eviction_memory_calculated").
								Str("runner_id", runnerID).
								Str("model", work.ModelName().String()).
								Interface("evictable_memory_per_gpu_gb", func() map[int]float64 {
									result := make(map[int]float64)
									for gpuIndex, evictable := range evictableMemoryPerGPU {
										result[gpuIndex] = float64(evictable) / (1024 * 1024 * 1024)
									}
									return result
								}()).
								Msg("OLLAMA_TP_DEBUG: Calculated evictable memory per GPU")

							singleGPU, _, _ = s.getOptimalGPUAllocationWithEviction(runnerID, effectiveMemoryRequirement, work.Runtime(), evictableMemoryPerGPU)

							if singleGPU != nil {
								log.Info().
									Str("OLLAMA_TP_DEBUG", "eviction_allocation_success").
									Str("runner_id", runnerID).
									Str("model", work.ModelName().String()).
									Int("allocated_gpu", *singleGPU).
									Uint64("effective_memory_gb", effectiveMemoryRequirement/(1024*1024*1024)).
									Msg("OLLAMA_TP_DEBUG: Eviction-based allocation successful")
							} else {
								log.Info().
									Str("OLLAMA_TP_DEBUG", "eviction_allocation_failed").
									Str("runner_id", runnerID).
									Str("model", work.ModelName().String()).
									Uint64("effective_memory_gb", effectiveMemoryRequirement/(1024*1024*1024)).
									Interface("evictable_memory_per_gpu_gb", func() map[int]float64 {
										result := make(map[int]float64)
										for gpuIndex, evictable := range evictableMemoryPerGPU {
											result[gpuIndex] = float64(evictable) / (1024 * 1024 * 1024)
										}
										return result
									}()).
									Msg("OLLAMA_TP_DEBUG: Eviction-based allocation failed - insufficient memory even with eviction")
							}
						}
						runnerGPUCompatible[runnerID] = (singleGPU != nil)
					}
				} else {
					log.Info().
						Str("OLLAMA_TP_DEBUG", "single_gpu_success").
						Str("runner_id", runnerID).
						Str("model", work.ModelName().String()).
						Uint64("effective_memory_gb", effectiveMemoryRequirement/(1024*1024*1024)).
						Interface("allocated_gpu", singleGPU).
						Msg("OLLAMA_TP_DEBUG: Single GPU allocation successful - no TP needed")
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

	// For Ollama models, ONLY use GGUF-based memory estimate - never use store values
	var requiredMemory uint64
	if work.Runtime() == types.RuntimeOllama {
		if s.memoryEstimationService == nil {
			return nil, fmt.Errorf("memory estimation service required for Ollama model %s but not available: %w", work.ModelName().String(), ErrModelWontFit)
		}
		// Use a representative runner to get GGUF estimate for error message
		found := false
		for runnerID := range runnerMemory {
			if ggufMemory, _ := s.getEffectiveMemoryRequirement(ctx, work, runnerID, 1); ggufMemory > 0 {
				requiredMemory = ggufMemory
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("GGUF memory estimation failed for Ollama model %s: %w", work.ModelName().String(), ErrModelWontFit)
		}
	} else {
		// For non-Ollama models, use store values
		requiredMemory = work.model.Memory
	}

	evictionAttempts := make(map[string]string) // runnerID -> eviction status

	for runnerID, memory := range runnerMemory {
		// Check both total memory and GPU compatibility
		hasEnoughTotalMemory := memory >= requiredMemory
		hasGPUCompatibility := runnerGPUCompatible[runnerID]

		if hasEnoughTotalMemory && hasGPUCompatibility {
			filteredRunners = append(filteredRunners, runnerID)
		} else {
			if !hasEnoughTotalMemory {
				numRunnersWithNotEnoughTotalMemory++
				evictionAttempts[runnerID] = "SKIPPED - insufficient total memory"
			} else if !hasGPUCompatibility {
				numRunnersWithGPUFragmentation++
				// Check if eviction was attempted for this runner
				if work.Runtime() == types.RuntimeVLLM {
					evictionAttempts[runnerID] = "ATTEMPTED - failed to find suitable GPU allocation even with eviction"
				} else if work.Runtime() == types.RuntimeOllama {
					evictionAttempts[runnerID] = "ATTEMPTED - GGUF estimation found no valid single/multi-GPU allocation"
				} else {
					evictionAttempts[runnerID] = "ATTEMPTED - traditional memory check failed"
				}
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
		// Build detailed error message with runner status information
		var errorDetails strings.Builder
		errorDetails.WriteString(fmt.Sprintf("Model '%s' (%s runtime) cannot be scheduled", work.ModelName().String(), work.Runtime()))
		errorDetails.WriteString(fmt.Sprintf(" - requires %d GB memory\n", requiredMemory/(1024*1024*1024)))

		if numRunnersWithGPUFragmentation > 0 {
			errorDetails.WriteString("Scheduling attempted but failed:\n")
			errorDetails.WriteString("• Single-GPU allocation: FAILED\n")
			errorDetails.WriteString("• Multi-GPU allocation: FAILED\n")
			if work.Runtime() == types.RuntimeOllama {
				errorDetails.WriteString("• GGUF-based memory estimation: USED\n")
			}
			errorDetails.WriteString("• Eviction attempts: ATTEMPTED\n\n")

			errorDetails.WriteString("Runner status breakdown:\n")
			for runnerID, memory := range runnerMemory {
				hasEnoughTotalMemory := memory >= requiredMemory
				hasGPUCompatibility := runnerGPUCompatible[runnerID]
				evictionStatus := evictionAttempts[runnerID]

				if hasEnoughTotalMemory && !hasGPUCompatibility {
					// Get runner status for detailed GPU info
					if status, err := s.controller.GetStatus(runnerID); err == nil {
						totalMemGB := memory / (1024 * 1024 * 1024)
						freeMemGB := status.FreeMemory / (1024 * 1024 * 1024)
						allocatedMemGB := (status.TotalMemory - status.FreeMemory) / (1024 * 1024 * 1024)

						errorDetails.WriteString(fmt.Sprintf("• Runner %s: %d GPUs, %d GB total (%d GB free, %d GB allocated)\n",
							runnerID, len(status.GPUs), totalMemGB, freeMemGB, allocatedMemGB))

						if evictionStatus != "" {
							errorDetails.WriteString(fmt.Sprintf("  Eviction status: %s\n", evictionStatus))
						}

						// Show per-GPU details
						for i, gpu := range status.GPUs {
							gpuFreeGB := gpu.FreeMemory / (1024 * 1024 * 1024)
							gpuTotalGB := gpu.TotalMemory / (1024 * 1024 * 1024)
							gpuAllocatedGB := gpuTotalGB - gpuFreeGB
							errorDetails.WriteString(fmt.Sprintf("  - GPU %d: %d GB total (%d GB free, %d GB allocated)\n",
								i, gpuTotalGB, gpuFreeGB, gpuAllocatedGB))
						}
					}
				} else if !hasEnoughTotalMemory && evictionStatus != "" {
					memoryGB := memory / (1024 * 1024 * 1024)
					errorDetails.WriteString(fmt.Sprintf("• Runner %s: %d GB total memory - %s\n", runnerID, memoryGB, evictionStatus))
				}
			}

			return nil, fmt.Errorf("%s: %w", errorDetails.String(), ErrModelWontFit)
		}

		// Not enough total memory case
		errorDetails.WriteString("No runners have sufficient total memory:\n")
		for runnerID, memory := range runnerMemory {
			memoryGB := memory / (1024 * 1024 * 1024)
			evictionStatus := evictionAttempts[runnerID]
			if evictionStatus != "" {
				errorDetails.WriteString(fmt.Sprintf("• Runner %s: %d GB total memory - %s\n", runnerID, memoryGB, evictionStatus))
			} else {
				errorDetails.WriteString(fmt.Sprintf("• Runner %s: %d GB total memory\n", runnerID, memoryGB))
			}
		}
		errorDetails.WriteString(fmt.Sprintf("Largest runner has %d GB, but model requires %d GB",
			largestRunnerMemory/(1024*1024*1024), requiredMemory/(1024*1024*1024)))

		return nil, fmt.Errorf("%s: %w", errorDetails.String(), ErrModelWontFit)
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

	log.Info().
		Str("OLLAMA_TP_DEBUG", "gpu_memory_state").
		Str("runner_id", runnerID).
		Interface("gpu_total_memory_gb", func() map[int]float64 {
			result := make(map[int]float64)
			for _, gpu := range status.GPUs {
				result[gpu.Index] = float64(gpu.TotalMemory) / (1024 * 1024 * 1024)
			}
			return result
		}()).
		Interface("gpu_allocated_memory_gb", func() map[int]float64 {
			result := make(map[int]float64)
			for gpuIndex, allocated := range allocatedMemoryPerGPU {
				result[gpuIndex] = float64(allocated) / (1024 * 1024 * 1024)
			}
			return result
		}()).
		Msg("OLLAMA_TP_DEBUG: GPU memory state before TP calculation")

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

	// Apply Ollama's fixed overhead calculations based on actual Ollama source code analysis:
	// For multi-GPU (TP), each GPU needs: graph + gpu.MinimumMemory + layerBuffer + runtimeOverhead
	// For single-GPU: same as above, plus gpuZeroOverhead (projector weights) on GPU 0
	// Based on logs: graph=~16GB, but this varies by model size and context
	// Reasonable overhead: ~10GB per GPU for TP allocations (prevents CPU fallback scenarios)
	const fixedOverheadPerGPU = uint64(10 * 1024 * 1024 * 1024) // 10GB fixed overhead per GPU for TP

	// Sanity check: if model won't fit across all GPUs even with TP, reject immediately
	// Use fixed overhead calculation instead of percentage-based margins
	totalEffectiveMemory := uint64(0)
	for _, gpu := range status.GPUs {
		allocatedMemory := allocatedMemoryPerGPU[gpu.Index]
		freeMemory := gpu.TotalMemory - allocatedMemory
		if freeMemory > fixedOverheadPerGPU {
			effectiveMemory := freeMemory - fixedOverheadPerGPU
			totalEffectiveMemory += effectiveMemory
		}
		// If freeMemory <= fixedOverheadPerGPU, this GPU contributes 0 effective memory
	}

	if modelMemoryRequirement > totalEffectiveMemory {
		log.Info().
			Str("OLLAMA_TP_DEBUG", "insufficient_total_memory").
			Str("runner_id", runnerID).
			Uint64("model_memory_gb", modelMemoryRequirement/(1024*1024*1024)).
			Uint64("total_effective_memory_gb", totalEffectiveMemory/(1024*1024*1024)).
			Uint64("fixed_overhead_per_gpu_gb", fixedOverheadPerGPU/(1024*1024*1024)).
			Msg("OLLAMA_TP_DEBUG: Model too large for TP even across all GPUs with fixed 10GB overhead per GPU - rejecting to prevent CPU fallback")
		return false, nil
	}

	// Simulate greedy assignment starting from the GPU with most free memory
	assignedGPUs := []int{}
	currentGPUIndex := 0
	layersAssigned := 0

	// Start with the first GPU if it has any free memory after fixed overhead
	if len(gpus) > 0 && gpus[currentGPUIndex].freeMemory > fixedOverheadPerGPU {

		remainingMemory := gpus[currentGPUIndex].freeMemory - fixedOverheadPerGPU

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
					// Apply fixed overhead to next GPU too
					if gpus[currentGPUIndex].freeMemory > fixedOverheadPerGPU {
						remainingMemory = gpus[currentGPUIndex].freeMemory - fixedOverheadPerGPU
						assignedGPUs = append(assignedGPUs, gpus[currentGPUIndex].index)
					} else {
						remainingMemory = 0 // GPU doesn't have enough memory after overhead
					}
				}
			}
		}
	}

	// Ollama uses tensor parallelism if layers are assigned to multiple GPUs
	wouldUseTensorParallel := len(assignedGPUs) > 1

	log.Info().
		Str("OLLAMA_TP_DEBUG", "memory_calculation").
		Str("runner_id", runnerID).
		Uint64("model_memory_gb", modelMemoryRequirement/(1024*1024*1024)).
		Interface("gpu_free_memory_gb", func() []float64 {
			result := make([]float64, len(gpus))
			for i, gpu := range gpus {
				result[i] = float64(gpu.freeMemory) / (1024 * 1024 * 1024)
			}
			return result
		}()).
		Interface("gpu_effective_memory_gb", func() []float64 {
			result := make([]float64, len(gpus))
			for i, gpu := range gpus {
				if gpu.freeMemory > fixedOverheadPerGPU {
					effective := float64(gpu.freeMemory - fixedOverheadPerGPU)
					result[i] = effective / (1024 * 1024 * 1024)
				} else {
					result[i] = 0.0 // No effective memory after fixed overhead
				}
			}
			return result
		}()).
		Int("estimated_layers", estimatedLayers).
		Uint64("layer_size_mb", layerSize/(1024*1024)).
		Int("layers_assigned", layersAssigned).
		Ints("assigned_gpus", assignedGPUs).
		Bool("would_use_tensor_parallel", wouldUseTensorParallel).
		Uint64("fixed_overhead_per_gpu_gb", fixedOverheadPerGPU/(1024*1024*1024)).
		Msg("OLLAMA_TP_DEBUG: Tensor parallelism prediction with fixed overhead analysis")

	return wouldUseTensorParallel, assignedGPUs
}
